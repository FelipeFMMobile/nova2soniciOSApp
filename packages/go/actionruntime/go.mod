module stsmodel.local/actionruntime

go 1.27.0

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	stsmodel.local/agentwire v0.0.0
	stsmodel.local/mcpwire v0.0.0
	stsmodel.local/voicewire v0.0.0
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace stsmodel.local/agentwire => ../agentwire

replace stsmodel.local/mcpwire => ../mcpwire

replace stsmodel.local/voicewire => ../voicewire
