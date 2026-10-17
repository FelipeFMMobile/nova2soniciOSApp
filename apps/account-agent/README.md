# Agente Conta

Agente conversacional Go independente: autenticação guiada e edição confirmada
de perfil, com prompt em `prompts/system.md`. Consome os MCPs de identidade e
perfil pelo LiteLLM `/mcp`, usando contexto assinado v2. Só fica pronto após
canários de guardrail de entrada e saída. Não grava evidência bruta de ferramentas.

Desenvolvimento, configurações e limites: [guia Conta](../../docs/domains/account-development.md).

```sh
GOWORK=off go test ./...
GOWORK=off go build ./cmd/account-agent
```

Expõe AgentSession v2 em `/v2/session`. O gateway fornece identidade validada;
o modelo nunca recebe tokens OAuth, senha, OTP ou credenciais de sessão.
