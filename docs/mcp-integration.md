# Integração MCP

Notes e Agenda são servidores MCP stdio executados pelo LiteLLM. O agente Go
descobre ferramentas, converte seus JSON Schemas para function tools do
Realtime e resolve as chamadas devolvidas pelo Nova.

O host valida schema e allowlist. Os perfis atuais permitem criação direta
de Notes/Agenda quando a ferramenta é chamada com argumentos válidos; exclusões
e cancelamentos exigem confirmação posterior. Uma mutação de resultado
desconhecido nunca é repetida automaticamente. A chave de idempotência deriva
do request/session ID, nome da ferramenta e argumentos canônicos.

Os SQLite de Notes e Agenda persistem dados funcionais e resultados de
operações atômicas. Eles não são o antigo banco de auditoria do gateway.

Configuração de múltiplos servidores: [mcp-servers.example.json](mcp-servers.example.json).
O caminho recomendado é `make dev`; detalhes de LiteLLM estão no
[runbook](nova-litellm-runbook.md).

## Aplicações e perfis

Os contratos publicados pertencem a `apps/mcp-notes/contracts/v1/` e
`apps/mcp-agenda/contracts/v1/`. O gateway mantém suas decisões confiáveis em
`apps/personal-assistant-agent/config/integrations/`: `profile.json` contém políticas e
frases, `runtime.json` descreve o processo e seu identificador no LiteLLM.

Para uma app nova sem alterar regras do gateway, adicione um perfil JSON no diretório
apontado por `STS_INTEGRATIONS_DIR` (também aceita subdiretórios). Os perfis
embutidos não podem ser sobrescritos. Configure o servidor em `STS_MCP_SERVERS`
ou `STS_LITELLM_MCP_SERVERS`; cada ferramenta precisa de perfil e política
coerentes. Metadados/annotations do MCP não concedem permissões.

No modo stdio, `cwd` é um caminho absoluto opcional e `env` mapeia nomes de
variáveis do processo para nomes de variáveis do ambiente do host. Nunca
coloque valores secretos no JSON; referências ausentes causam erro no load.
No LiteLLM, credenciais e runtime do servidor são configurados na plataforma;
esses campos stdio não são transmitidos à API REST.

O supervisor aceita `--mcps notes,agenda` ou nomes adicionais do catálogo.
Para um novo servidor usar esse supervisor, forneça `runtime.json` e
`profile.json` em seu diretório de integração e registre-o no LiteLLM. O
transporte HTTP remoto ainda requer validação na plataforma; a composição
atual mantém os MCPs stdio na imagem LiteLLM como etapa de compatibilidade.
