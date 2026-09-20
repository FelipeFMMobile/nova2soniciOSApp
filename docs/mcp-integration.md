# Integração MCP

Notes e Agenda são servidores MCP stdio executados pelo LiteLLM. O gateway Go
descobre ferramentas, converte seus JSON Schemas para function tools do
Realtime e resolve as chamadas devolvidas pelo Nova.

O host valida schema e allowlist, exige intenção explícita para criações e uma
confirmação posterior para exclusões/cancelamentos. Uma mutação de resultado
desconhecido nunca é repetida automaticamente. A chave de idempotência deriva
do request/session ID, nome da ferramenta e argumentos canônicos.

Os SQLite de Notes e Agenda persistem dados funcionais e resultados de
operações atômicas. Eles não são o antigo banco de auditoria do gateway.

Configuração de múltiplos servidores: [mcp-servers.example.json](mcp-servers.example.json).
O caminho recomendado é `make dev`; detalhes de LiteLLM estão no
[runbook](nova-litellm-runbook.md).
