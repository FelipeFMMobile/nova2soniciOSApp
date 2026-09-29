package voice_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"stsmodel.local/voicewire"
)

type fixtureEvent struct {
	Name string          `json:"name"`
	Type string          `json:"type"`
	JSON json.RawMessage `json:"json"`
}

type fixtures struct {
	ValidClientEvents   []fixtureEvent `json:"validClientEvents"`
	InvalidClientEvents []fixtureEvent `json:"invalidClientEvents"`
	ServerEvents        []fixtureEvent `json:"serverEvents"`
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	data, err := os.ReadFile("fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var value fixtures
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPublishedVoiceV1ClientFixtures(t *testing.T) {
	fixture := loadFixtures(t)
	for _, item := range fixture.ValidClientEvents {
		t.Run(item.Name, func(t *testing.T) {
			event, err := protocol.DecodeClient(item.JSON)
			if err != nil || event.Type != item.Type {
				t.Fatalf("published valid event changed: type=%q err=%v", event.Type, err)
			}
		})
	}
	for _, item := range fixture.InvalidClientEvents {
		t.Run(item.Name, func(t *testing.T) {
			if _, err := protocol.DecodeClient(item.JSON); err == nil {
				t.Fatal("published invalid event became valid")
			}
		})
	}
}

func TestPublishedVoiceV1ServerFixtures(t *testing.T) {
	for _, item := range loadFixtures(t).ServerEvents {
		t.Run(item.Name, func(t *testing.T) {
			var event protocol.Event
			if err := json.Unmarshal(item.JSON, &event); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			var want, got map[string]any
			if json.Unmarshal(item.JSON, &want) != nil || json.Unmarshal(encoded, &got) != nil {
				t.Fatal("invalid fixture JSON")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("published server event changed\nwant: %s\n got: %s", item.JSON, encoded)
			}
		})
	}
}
