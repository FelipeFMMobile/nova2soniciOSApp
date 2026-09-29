module stsmodel.local/integration

go 1.27.0

require (
	github.com/gorilla/websocket v1.5.3
	stsmodel.local/mcpwire v0.0.0
)

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace stsmodel.local/mcpwire => ./packages/go/mcpwire
