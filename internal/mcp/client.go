package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	cmd       *exec.Cmd
	in        io.WriteCloser
	out       io.ReadCloser
	responses chan rpcMessage
	done      chan struct{}
	closing   chan struct{}
	gate      chan struct{}
	once      sync.Once
	next      uint64
}

func Start(ctx context.Context, command string, args []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	// Discard untrusted subprocess stderr; tools may include sensitive payloads.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, errors.New("MCP process failed to start")
	}
	c := &Client{cmd: cmd, in: in, out: out, responses: make(chan rpcMessage, 16), done: make(chan struct{}), closing: make(chan struct{}), gate: make(chan struct{}, 1)}
	go c.read()
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var initialized struct {
		Version      string `json:"protocolVersion"`
		Capabilities struct {
			Tools *json.RawMessage `json:"tools"`
		} `json:"capabilities"`
	}
	err = c.request(initCtx, "initialize", map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "sts-gateway", "version": "0.5.0"}}, &initialized)
	if err != nil || initialized.Version != ProtocolVersion || initialized.Capabilities.Tools == nil {
		c.Close()
		return nil, errors.New("MCP initialization failed")
	}
	if err = json.NewEncoder(in).Encode(map[string]string{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		c.Close()
		return nil, errors.New("MCP initialization failed")
	}
	return c, nil
}
func (c *Client) read() {
	defer close(c.done)
	defer close(c.responses)
	scanner := bufio.NewScanner(c.out)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	for scanner.Scan() {
		var message rpcMessage
		if json.Unmarshal(scanner.Bytes(), &message) != nil || message.JSONRPC != "2.0" {
			return
		}
		if len(message.ID) == 0 {
			continue
		}
		select {
		case c.responses <- message:
		case <-c.closing:
			return
		}
	}
}

func (c *Client) Close() {
	c.once.Do(func() {
		close(c.closing)
		c.in.Close()
		_ = c.cmd.Process.Kill()
		c.out.Close()
		<-c.done
		_ = c.cmd.Wait()
	})
}

func (c *Client) request(ctx context.Context, method string, params any, target any) error {
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.gate }()
	// A timed-out stream cannot safely be reused: close it to release blocked
	// writes, discard late responses and leave the durable server ledger intact.
	stop := context.AfterFunc(ctx, c.Close)
	defer stop()
	c.next++
	id := json.RawMessage(strconv.FormatUint(c.next, 10))
	data, _ := json.Marshal(params)
	if err := json.NewEncoder(c.in).Encode(rpcMessage{JSONRPC: "2.0", ID: id, Method: method, Params: data}); err != nil {
		return errors.New("MCP disconnected")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case response, ok := <-c.responses:
			if !ok {
				return errors.New("MCP disconnected")
			}
			if string(response.ID) != string(id) {
				return errors.New("MCP response identity mismatch")
			}
			if response.Error != nil {
				return errors.New("MCP rejected request")
			}
			if len(response.Result) > 65536 {
				return errors.New("MCP result exceeds limit")
			}
			return json.Unmarshal(response.Result, target)
		}
	}
}

func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var page struct {
		Tools      []Tool `json:"tools"`
		NextCursor string `json:"nextCursor"`
	}
	if err := c.request(ctx, "tools/list", map[string]any{}, &page); err != nil {
		return nil, err
	}
	if len(page.Tools) > 32 || page.NextCursor != "" {
		return nil, errors.New("MCP tool list exceeds supported limits")
	}
	return page.Tools, nil
}
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) (Result, error) {
	var result Result
	err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args, "_meta": map[string]any{"sts/idempotencyKey": key, "sts/confirmed": confirmed}}, &result)
	return result, err
}
