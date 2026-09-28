# Arquitetura

```text
App Apple / voice-client
  │ WebSocket /v1/voice · JSON + PCM16 16 kHz
  ▼
voice-gateway · monólito modular Go
  ├─ Conversation → sessão, VAD/barge-in, renovação e timeouts
  ├─ Actions → autorização, confirmação e execução de tools
  ├─ contracts/ → voz e envelope assinado
  ├─ config/integrations/ → aliases, allowlist e políticas
  └─ portas Realtime/MCP → LiteLLM
                            ├─ perfil AWS somente leitura → Bedrock/Nova
                            ├─ PostgreSQL → chaves + Usage/Spend
                            └─ MCP stdio
                                 ├─ mcp-notes → contrato e SQLite próprios
                                 └─ mcp-agenda → contrato e SQLite próprios
```

O protocolo público do app termina no Go. O adaptador Go traduz entre os
eventos internos e OpenAI Realtime, mas não conhece SigV4, Bedrock ou
credenciais AWS. LiteLLM é a única fronteira cloud e não há fallback direto.

Cada WebSocket usa seu `requestId` como session/trace ID e recebe as tags
`sts`, `voice` e `nova`. A chave `sts-go-gateway` autoriza somente
`nova-sonic`, `/v1/realtime` e os MCPs selecionados.

## Responsabilidades

- App: captura/reprodução, UI e descarte imediato de áudio interrompido.
- Voice gateway: autenticação local, estado, PCM, histórico textual limitado,
  renovação, autorização e confirmação de ferramentas.
- Apps MCP: regras de negócio, contratos públicos, persistência e ledger
  idempotente de suas próprias operações.
- LiteLLM: conexão Bedrock Realtime, credenciais AWS, grants e spend logs.
- PostgreSQL: uso/custo e estado operacional do LiteLLM, sem conteúdo.
- SQLite Notes/Agenda: dados de negócio e replay seguro de mutações, sem schema
  ou ledger compartilhado entre as aplicações.

O SQLite de auditoria do gateway foi removido. Instalações existentes não são
migradas nem apagadas; suas tabelas antigas ficam órfãs.

## Contrato de áudio e falhas

Entrada é PCM16 LE mono 16 kHz e saída é PCM16 mono 24 kHz. Áudio e eventos
são transmitidos continuamente. Silêncio, fechamento anormal ou erro do
LiteLLM encerram a sessão com erro controlado. Mutações com resultado
desconhecido não são repetidas automaticamente.

Consulte o [runbook Nova/LiteLLM](nova-litellm-runbook.md).
