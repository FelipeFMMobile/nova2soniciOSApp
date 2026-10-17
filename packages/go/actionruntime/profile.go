package orchestrator

type Profile struct {
	Policy              string   `json:"policy"`
	Direct              bool     `json:"direct,omitempty"`
	ConfirmationPhrase  string   `json:"confirmation_phrase,omitempty"`
	AllowUIConfirmation bool     `json:"allow_ui_confirmation,omitempty"`
	DateTimeFields      []string `json:"datetime_fields,omitempty"`
	IntentPattern       string   `json:"intent_pattern,omitempty"`
}
