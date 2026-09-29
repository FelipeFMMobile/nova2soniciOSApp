package v1alpha1

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"stsmodel.local/agentwire"
)

type fixture struct {
	Name      string          `json:"name"`
	Phase     string          `json:"phase"`
	SessionID string          `json:"sessionId"`
	AgentID   string          `json:"agentId"`
	Message   json.RawMessage `json:"message"`
}

func validate(item fixture) error {
	message, err := agentwire.Decode(item.Message)
	if err != nil {
		return err
	}
	switch item.Phase {
	case "open":
		return message.ValidateOpen()
	case "accepted":
		return message.ValidateAccepted(item.AgentID)
	case "command":
		return message.ValidateCommand(item.SessionID)
	case "event":
		return message.ValidateEvent()
	case "decode":
		return nil
	default:
		return errors.New("unknown fixture phase")
	}
}

func TestFixtures(t *testing.T) {
	data, err := os.ReadFile("fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Valid   []fixture `json:"valid"`
		Invalid []fixture `json:"invalid"`
	}
	if json.Unmarshal(data, &document) != nil || len(document.Valid) == 0 || len(document.Invalid) == 0 {
		t.Fatal("invalid Agent Session fixture document")
	}
	for _, item := range document.Valid {
		t.Run("valid/"+item.Name, func(t *testing.T) {
			if err := validate(item); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, item := range document.Invalid {
		t.Run("invalid/"+item.Name, func(t *testing.T) {
			if err := validate(item); err == nil {
				t.Fatal("invalid fixture accepted")
			}
		})
	}
}
