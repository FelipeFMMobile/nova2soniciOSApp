# Voice v2 — Conta

Coexiste com [Voice v1](../v1/README.md) em `/v2/voice`. Todos os eventos têm
`version: 2`. Não converter mensagens v2 silenciosamente para v1.

`session.start` seleciona `agentId: account` do catálogo confiável do gateway.
O transporte de áudio, limites, turnos, interrupção e confirmação segue v1.
`ui.a2ui` acrescenta `ui` com mensagens A2UI v0.9.1 de catálogo restrito
`urn:sts:a2ui:account:v1`; somente Text, Column e Button para autenticação.
A ação `account.authenticate` carrega apenas `transaction_id` opaco.

Headers: `Authorization` autentica o cliente técnico; `X-STS-Device` vincula
instalação; `X-STS-Account` carrega acesso opaco quando autenticado. Nenhuma
credencial aparece na URL ou em eventos do modelo. HTTP somente em loopback;
fora dele TLS é obrigatório. O gateway valida a sessão em identidade e abre
AgentSession v2 com principal confiável; o modelo não escolhe esse principal.

O app usa POST `/v2/auth/begin`, `/prepare`, `/complete`, `/refresh`, `/logout`
com `Authorization`, `X-STS-Device` e JSON. Campos aceitos respectivamente:
nenhum; `transaction_id`; `transaction_id,state,code`; `refresh_token`;
`refresh_token`. Campos extras, JSON múltiplo e corpo acima do limite são recusados.
As respostas têm `Cache-Control: no-store`. Esse canal de controle não é MCP.

Implementação: `packages/go/voicewire`, `internal/adapters/voice/account.go`;
consumidor Swift: `VoiceCore/VoiceEvent.swift`, `Shared/AccountSession.swift`.

## Exemplo de sessão

Mensagens abaixo são frames WebSocket separados. IDs e dados são fictícios.
O app conecta a `/v2/voice` com os headers técnicos e de dispositivo acima.

```json
{"version":2,"type":"session.start","requestId":"request-login-1","provider":"fake","agentId":"account"}
{"version":2,"type":"session.ready","sessionId":"session-login-1","provider":"fake","state":"idle"}
{"version":2,"type":"ui.a2ui","sessionId":"session-login-1","ui":[{"version":"v0.9.1","deleteSurface":{"surfaceId":"account-login"}},{"version":"v0.9.1","createSurface":{"surfaceId":"account-login","catalogId":"urn:sts:a2ui:account:v1"}},{"version":"v0.9.1","updateComponents":{"surfaceId":"account-login","components":[{"id":"root","component":"Column","children":["instructions","login"]},{"id":"instructions","component":"Text","text":"Entre com sua senha e o código recebido por e-mail na tela segura."},{"id":"login","component":"Button","child":"label","action":{"event":{"name":"account.authenticate","context":{"transaction_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}},{"id":"label","component":"Text","text":"Autenticar"}]}}]}
{"version":2,"type":"session.stop","sessionId":"session-login-1"}
{"version":2,"type":"session.stopped","sessionId":"session-login-1","state":"closed"}
```

O botão suspende a conversa. O app usa `/v2/auth/prepare` e `/complete` no canal
HTTP autenticado; código, state, verifier e credenciais não aparecem nestes
frames. Após autenticar, reconecta com `X-STS-Account` e um novo requestId.
No modo fake há atalho DEBUG local; produção usa o navegador do sistema.

```json
{"version":2,"type":"session.start","requestId":"request-profile-1","provider":"nova","agentId":"account"}
{"version":2,"type":"session.ready","sessionId":"session-profile-1","provider":"nova","state":"idle"}
{"version":2,"type":"tool.confirmation","sessionId":"session-profile-1","turnId":"turn-profile-1","tool":{"operationId":"update-1","name":"profile.update_me","arguments":{"expected_version":1,"changes":{"city":"Campinas"}}}}
{"version":2,"type":"tool.confirm","sessionId":"session-profile-1","tool":{"operationId":"update-1","approved":false}}
{"version":2,"type":"tool.result","sessionId":"session-profile-1","turnId":"turn-profile-1","tool":{"operationId":"update-1","name":"profile.update_me","result":{"content":[{"type":"text","text":"{\"status\":\"cancelled\"}"}],"isError":false}}}
{"version":2,"type":"session.stop","sessionId":"session-profile-1"}
{"version":2,"type":"session.stopped","sessionId":"session-profile-1","state":"closed"}
```

O trecho omite áudio/transcrições anteriores à solicitação da ferramenta, que
seguem v1. `approved:true` executa somente os argumentos pendentes exatos;
`approved:false` publica `cancelled`, remove os botões e não grava. Por voz,
a frase exata `confirmo alterar meus dados` só autoriza em turno posterior.
Argumentos alterados exigem nova confirmação. Cancelar não equivale a sucesso.
