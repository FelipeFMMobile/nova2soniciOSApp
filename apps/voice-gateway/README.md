# voice-gateway

Aplicação Go com módulo, build e ciclo de vida próprios. A partir deste diretório:

```sh
GOWORK=off go build ./...
```

O build requer somente esta app e as bibliotecas técnicas em `packages/go`.
Os `replace` locais apontam apenas para bibliotecas técnicas. `go.work` é
conveniência; `make isolated` prova o build sem o código do agente e verifica
que o binário não depende de MCP ou de outras aplicações. `testkit` e sua
dependência MCP são usados somente por testes, não pelo binário de borda.

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

Desde `v0.11.0`, somente transporte remoto é suportado. Para rollback, restaure
o conjunto `v0.10.x`; `STS_AGENT_MODE=inprocess` é rejeitado nesta versão.
No modo remoto, `STS_AGENT_CATALOG` fornece endpoints confiáveis e referências
a tokens internos; `agentId` e endpoint nunca são aceitos do cliente Voice v1.
O gateway rejeita configuração ou segredos LiteLLM/MCP nesse modo.
