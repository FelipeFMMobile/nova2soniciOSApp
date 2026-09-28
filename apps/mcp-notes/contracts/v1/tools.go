package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	mcp "stsmodel.local/mcpwire"
)

//go:embed tools.json
var files embed.FS

func Tools() []mcp.Tool {
	data, _ := files.ReadFile("tools.json")
	var tools []mcp.Tool
	if err := json.Unmarshal(data, &tools); err != nil {
		panic(err)
	}
	for i := range tools {
		var compact bytes.Buffer
		if err := json.Compact(&compact, tools[i].InputSchema); err != nil {
			panic(err)
		}
		tools[i].InputSchema = compact.Bytes()
	}
	return tools
}
