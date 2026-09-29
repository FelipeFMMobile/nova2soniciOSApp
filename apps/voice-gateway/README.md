# voice-gateway

Aplicação Go com módulo, build e ciclo de vida próprios. A partir deste diretório:

```sh
GOWORK=off go build ./...
```

O build requer somente esta app e as bibliotecas técnicas em `packages/go`.
Os `replace` locais permitem builds reproduzíveis a partir do checkout, sem
depender de um registry para `stsmodel.local`. Durante a janela de rollback há
um `replace` explícito para `personal-assistant-agent/inprocess`; ele será
removido somente após a janela completa de rollback `v0.10.x`. `go.work` é apenas
conveniência na raiz.

`app.json` declara comandos de build/test/run em arrays, relativos ao diretório
da app. O agregador `python3 scripts/modules.py build` na raiz produz os
binários em `bin/`; execute-o antes dos testes que lançam processos reais.
`GOWORK=off go test ./...` executa a suíte da app após esse preparo.

Imagem própria, com contexto na raiz do repositório:

```sh
docker build -f apps/voice-gateway/Dockerfile -t sts-voice-gateway:local .
```

A composição LiteLLM mantém stdio como compatibilidade. Esta imagem independente
não implica que transporte MCP HTTP remoto esteja configurado ou validado.

O modo remoto é padrão desde `v0.10.0`; `STS_AGENT_MODE=inprocess` ativa o rollback.
No modo remoto, `STS_AGENT_CATALOG` fornece endpoints confiáveis e referências
a tokens internos; `agentId` e endpoint nunca são aceitos do cliente Voice v1.
O gateway rejeita configuração ou segredos LiteLLM/MCP nesse modo.
