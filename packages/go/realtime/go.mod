module stsmodel.local/realtime

go 1.27.0

require (
	github.com/gorilla/websocket v1.5.3
	stsmodel.local/agentwire v0.0.0
	stsmodel.local/voicewire v0.0.0
)

replace stsmodel.local/agentwire => ../agentwire

replace stsmodel.local/voicewire => ../voicewire
