# voice-gateway

Aplicação Go com módulo, build e ciclo de vida próprios. A partir deste diretório:

```sh
GOWORK=off go build ./...
```

O build requer somente esta app e as bibliotecas técnicas em `packages/go`.
Os `replace` locais dessas bibliotecas permitem builds reproduzíveis a partir
do checkout, sem depender de um registry para `stsmodel.local`. Não há `replace`
para outra aplicação. `go.work` é apenas conveniência na raiz.

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
