# Estado da implementação

Implementado:

- protocolo público `/v1/voice` e apps Apple;
- gateway Go com fake determinístico e adaptador LiteLLM Realtime;
- Nova 2 Sonic no Bedrock exclusivamente via LiteLLM;
- streaming contínuo, VAD/barge-in, timeout, cancelamento e renovação;
- MCP Notes/Agenda com schemas tipados, políticas, confirmação e
  idempotência funcional em SQLite;
- chave virtual limitada, PostgreSQL e Usage/Spend metadata-only;
- supervisor local sem transporte Python nem credenciais AWS no Go.

Antes de promover um ambiente, execute `make check`, testes Apple e integração
local. O smoke AWS é opt-in e somente é aceito quando produz áudio, libera a
sessão, registra custo positivo e não persiste conteúdo. Consulte
[nova-litellm-runbook.md](nova-litellm-runbook.md).
