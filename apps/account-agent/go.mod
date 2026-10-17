module stsmodel.local/account-agent

go 1.27.0

replace stsmodel.local/agentwire => ../../packages/go/agentwire

replace stsmodel.local/mcpwire => ../../packages/go/mcpwire

replace stsmodel.local/voicewire => ../../packages/go/voicewire

replace stsmodel.local/realtime => ../../packages/go/realtime

require (
	stsmodel.local/actionruntime v0.0.0
	stsmodel.local/agentwire v0.0.0
	stsmodel.local/mcpwire v0.0.0
	stsmodel.local/realtime v0.0.0
	stsmodel.local/voicewire v0.0.0
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace stsmodel.local/actionruntime => ../../packages/go/actionruntime
