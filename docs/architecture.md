# Arquitetura

```text
App Apple / voice-client
  │ WebSocket /v1/voice · JSON + PCM16 16 kHz
  ▼
voice-gateway · borda Go
  ├─ autenticação, limites, heartbeat e métricas de transporte
  ├─ contracts/voice/v1 → contrato público
  └─ AgentSession → personal-assistant-agent
                       ├─ prompt, histórico, VAD/barge-in e renovação
                       ├─ Actions → autorização, confirmação e tools
                       ├─ config/integrations/ → aliases, allowlist e políticas
                       └─ Realtime/MCP → LiteLLM
                            ├─ perfil AWS somente leitura → Bedrock/Nova
                            ├─ PostgreSQL → chaves + Usage/Spend
                            └─ MCP stdio
                                 ├─ mcp-notes → contrato e SQLite próprios
                                 └─ mcp-agenda → contrato e SQLite próprios
```

O protocolo público termina no gateway. Ele traduz Voice v1 para a porta
`AgentSession`; o agente traduz a conversa para OpenAI Realtime, mas não
conhece SigV4, Bedrock ou credenciais AWS. LiteLLM é a única fronteira cloud e
não há fallback direto. `agent-session/v1` remoto é padrão e restrito a
loopback; o adapter in-process permanece como rollback durante `v0.10.x`.

Cada WebSocket usa seu `requestId` como session/trace ID e recebe as tags
`sts`, `voice` e `nova`. A chave `sts-go-gateway` autoriza somente
`nova-sonic`, `/v1/realtime` e os MCPs selecionados.

## Responsabilidades

- App: captura/reprodução, UI e descarte imediato de áudio interrompido.
- Voice gateway: autenticação local, WebSocket público, PCM, limites,
  heartbeat, roteamento e métricas de transporte.
- Personal assistant agent: prompt, histórico textual limitado, renovação,
  autorização, confirmação e integração de ferramentas.
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
