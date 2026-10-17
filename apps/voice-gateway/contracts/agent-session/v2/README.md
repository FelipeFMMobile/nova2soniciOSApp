# AgentSession v2

Coexiste com [v1](../v1/README.md); endpoint Conta `/v2/session`, discriminador
`protocol: agent-session/v2`. Cada entrada do catálogo fixa protocolo, destino,
ID do agente e credencial de serviço. Rejeitar versões divergentes.

Mantém mensagens `session.open`, `session.accepted`, `session.command`,
`session.event`, filas e limites v1. O principal acrescenta `authenticated`,
`clientCode`, `sessionRef`, `deviceId`; é preenchido pelo gateway após validação
interna. IDs não são credenciais. Eventos `ui.a2ui` transportam somente o catálogo
restrito de Conta. Tokens OAuth/plataforma não integram o protocolo semântico.

Implementação e verificações: `packages/go/agentwire`; consumidor:
`apps/account-agent/internal/account/session.go`.

## Exemplo de transporte autenticado

Frames separados entre gateway e Conta. O gateway usa a credencial de serviço
no handshake WebSocket, fora do JSON. O principal fictício abaixo só pode ser
construído após a introspecção no serviço de identidade.

```json
{"protocol":"agent-session/v2","kind":"session.open","agentId":"account","start":{"sessionId":"session-profile-1","requestId":"request-profile-1","agentId":"account","provider":"nova","principal":{"subject":"user-example","clientCode":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sessionRef":"session-reference-example","deviceId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","authenticated":true}}}
{"protocol":"agent-session/v2","kind":"session.accepted","agentId":"account"}
{"protocol":"agent-session/v2","kind":"session.event","event":{"type":"agent.session.ready","provider":"nova","state":"idle"}}
{"protocol":"agent-session/v2","kind":"session.event","event":{"type":"tool.confirmation","turnId":"turn-profile-1","tool":{"operationId":"update-1","name":"profile.update_me","arguments":{"expected_version":1,"changes":{"city":"Campinas"}}}}}
{"protocol":"agent-session/v2","kind":"session.command","command":{"type":"input.confirmation","sessionId":"session-profile-1","operationId":"update-1","approved":false}}
{"protocol":"agent-session/v2","kind":"session.event","event":{"type":"tool.result","turnId":"turn-profile-1","tool":{"operationId":"update-1","name":"profile.update_me","result":{"content":[{"type":"text","text":"{\"status\":\"cancelled\"}"}],"isError":false}}}}
{"protocol":"agent-session/v2","kind":"session.command","command":{"type":"input.close","sessionId":"session-profile-1"}}
{"protocol":"agent-session/v2","kind":"session.event","event":{"type":"agent.session.closed","state":"closed"}}
```

O trecho de conversa omite áudio/transcrição. Antes do login, `authenticated`
fica falso e `clientCode`/`sessionRef` são omitidos; `deviceId` continua obrigatório
para Conta. Login abre uma sessão nova. `ui.a2ui` aparece dentro de `event.ui`
com o mesmo catálogo e mensagens do [exemplo Voice v2](../../voice/v2/README.md).
Eventos não incluem códigos OAuth, tokens ou envelopes MCP assinados.
