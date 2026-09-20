# Notes + Agenda

Ao selecionar **Notes + Agenda** em `make dev`, o LiteLLM inicia ambos os MCPs
e a chave `sts-go-gateway` recebe somente os grants correspondentes. O Nova vê
a união tipada das ferramentas; o Go mantém roteamento, políticas,
confirmações, limites, timeouts e correlação dos resultados.

Cada servidor usa seu próprio SQLite e ledger de idempotência. Uma falha ou
timeout de Agenda não autoriza retry automático de mutação e não altera o
banco de Notes.

Use fixtures somente pelo prompt opt-in do supervisor. O formato de datas da
Agenda é RFC3339 com offset explícito e padrão `America/Sao_Paulo`.
