# mcp-identity

Identidade, OAuth/PKCE, sessões, revogação e CLIENTE_CODE. Aplicação Spring AI independente com Java 21, Maven Wrapper,
MCP Streamable HTTP e PostgreSQL próprio. Contratos em `contracts/v2/`.
Autenticação entre serviços e envelope v2 assinado são obrigatórios.

```sh
./mvnw -B verify
```

Configuração e desenvolvimento integrado: [guia Conta](../../docs/domains/account-development.md).
Migrações pertencem à aplicação em `src/main/resources/db/migration`.
