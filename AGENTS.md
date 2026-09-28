# Trabalho neste repositório

Para análise de domínios, desenho modular ou inclusão de novas aplicações,
comece por `harness/README.md` e execute o roteiro `harness/domain-analysis.md`
com a skill local indicada ali. Os perfis em `harness/agents/` são papéis de
trabalho reutilizáveis; sua presença não solicita criação automática de subagentes.

A proposta em `docs/domains/` distingue o estado atual do estado planejado.
Não trate diretórios futuros como existentes nem a migração como concluída.
O repositório deve comportar aplicações de diferentes linguagens. Domínios são
capacidades de negócio; Go, Spring, MCP e SQLite são escolhas técnicas.
Preserve contratos de voz/MCP, confirmação, idempotência e privacidade durante
refatorações. Cada aplicação deve possuir seus dados e suas regras.
