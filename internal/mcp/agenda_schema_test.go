package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgendaIntervalPropertiesIncludeCoherentRFC3339Examples(t *testing.T) {
	for _, tool := range AgendaTools() {
		if tool.Name == "agenda.cancel_event" {
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			var schema struct {
				Properties map[string]struct {
					Format      string   `json:"format"`
					Description string   `json:"description"`
					Examples    []string `json:"examples"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			times := make(map[string]time.Time)
			args := map[string]any{}
			for _, name := range []string{"start", "end"} {
				property := schema.Properties[name]
				if property.Format != "date-time" || property.Description == "" || len(property.Examples) != 1 {
					t.Fatalf("%s: missing date-time documentation or example", name)
				}
				parsed, err := time.Parse(time.RFC3339, property.Examples[0])
				if err != nil {
					t.Fatal(err)
				}
				times[name] = parsed
				args[name] = property.Examples[0]
			}
			if times["end"].Sub(times["start"]) != time.Hour {
				t.Fatal("examples must describe a one-hour interval")
			}
			switch tool.Name {
			case "agenda.create_event":
				args["title"] = "Exemplo"
			case "agenda.list_slots":
				args["duration_minutes"] = 60
			}
			compiled, err := Compile(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(compiled, encoded); err != nil {
				t.Fatal(err)
			}
		})
	}
}
