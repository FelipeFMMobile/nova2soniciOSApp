// Package integrations loads trusted host policy, never remote MCP annotations.
package integrations

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"stsmodel.local/actionruntime"
)

//go:embed */profile.json
var builtins embed.FS

type Profile = orchestrator.Profile
type Registry map[string]Profile

func decode(data []byte, r Registry) error {
	var doc struct {
		Tools map[string]Profile `json:"tools"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&doc); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing profile JSON")
	}
	if len(doc.Tools) == 0 {
		return errors.New("empty integration profile")
	}
	for name, p := range doc.Tools {
		if strings.TrimSpace(name) == "" {
			return errors.New("empty tool name")
		}
		if _, ok := r[name]; ok {
			return fmt.Errorf("duplicate host profile: %s", name)
		}
		switch p.Policy {
		case "read_only":
			if p.Direct || p.ConfirmationPhrase != "" || p.AllowUIConfirmation || p.IntentPattern != "" {
				return errors.New("invalid read policy")
			}
		case "explicit_intent":
			if p.Direct == (p.IntentPattern != "") || p.ConfirmationPhrase != "" || p.AllowUIConfirmation {
				return errors.New("intent policy needs exactly direct or intent_pattern")
			}
		case "confirm_later":
			if p.ConfirmationPhrase == "" || p.Direct || p.IntentPattern != "" {
				return errors.New("confirmation policy requires phrase")
			}
		default:
			return errors.New("invalid host policy")
		}
		if p.IntentPattern != "" {
			if _, err := regexp.Compile(p.IntentPattern); err != nil {
				return err
			}
		}
		r[name] = p
	}
	return nil
}

// Load merges embedded built-ins with additional trusted profiles. Built-ins
// cannot be overridden; new applications require only another profile file.
func Load() (Registry, error) {
	r := Registry{}
	files, _ := builtins.ReadDir(".")
	for _, f := range files {
		data, err := builtins.ReadFile(f.Name() + "/profile.json")
		if err != nil {
			return nil, err
		}
		if err = decode(data, r); err != nil {
			return nil, err
		}
	}
	if dir := os.Getenv("STS_INTEGRATIONS_DIR"); dir != "" {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "runtime.json" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return decode(data, r)
		})
		if err != nil {
			return nil, err
		}

	}
	return r, nil
}
func ValidatePolicy(name, policy string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	p, ok := r[name]
	if !ok {
		return errors.New("missing trusted integration profile")
	}
	if p.Policy != policy {
		return errors.New("configured policy differs from trusted integration profile")
	}
	return nil
}
