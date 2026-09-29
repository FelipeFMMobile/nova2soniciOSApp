module stsmodel.local/voice-gateway

go 1.27.0

require (
	github.com/gorilla/websocket v1.5.3
	stsmodel.local/agentwire v0.0.0
	stsmodel.local/mcpwire v0.0.0
	stsmodel.local/personal-assistant-agent v0.0.0
	stsmodel.local/testkit v0.0.0
	stsmodel.local/voicewire v0.0.0
)

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace stsmodel.local/mcpwire => ../../packages/go/mcpwire

replace stsmodel.local/agentwire => ../../packages/go/agentwire

replace stsmodel.local/personal-assistant-agent => ../personal-assistant-agent

replace stsmodel.local/voicewire => ../../packages/go/voicewire

replace stsmodel.local/testkit => ../../packages/go/testkit
