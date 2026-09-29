# Changelog

## v0.10.0 — Agent Session v1 e rollout local

- transporte remoto padrão com heartbeat e métricas; rollback in-process durante
  toda a minor `v0.10.x`; demo fake explicitamente in-process;
- supervisor inicia agente e gateway com credenciais separadas;
- testes espelhados, MCPs SQLite reais, CLI e Apple VoiceCore com Realtime simulado;
- gate de latência local p95 adicional ≤ 10 ms, aprovado pelo usuário;
- sem downgrade automático de Agent Session v1 para v1alpha1.

| Binários / contratos | Compatibilidade |
| --- | --- |
| Gateway + agente `v0.10.x` | Agent Session `v1` |
| CLI + Apple existentes | Voice `v1`, sem mudança de produção |
| Host Context / Notes / Agenda | `v1`, inalterados |
| Rollback | `STS_AGENT_MODE=inprocess` no conjunto `v0.10.x` |

Não misture gateway `v0.10.x` e agente `v0.9.x`. Para reverter binários, volte
os dois e o catálogo juntos. Evidência local e limitações em
[rollout](docs/domains/agent-rollout-validation.md).

## v0.9.0 — Agent Session remoto opt-in

- nova aplicação `personal-assistant-agent`, dona de prompt, Realtime, actions,
  confirmação, MCP, perfis e Host Context;
- contrato `agent-session/v1alpha1` e implementação Go em `agentwire`;
- transporte WebSocket interno autenticado e limitado, somente loopback;
- bearer derivado e prova HMAC do agente sem transmitir a chave compartilhada;
- catálogo confiável de agentes, sem seleção de endpoint pelo cliente;
- modo remoto opt-in; modo in-process permanece padrão e rollback;
- Voice Protocol v1, Host Context v1 e contratos MCP v1 permanecem compatíveis.

Matriz compatível desta release:

| Componente | Versões |
| --- | --- |
| Voice público | `v1` |
| Agent Session | `v1alpha1` |
| Host Context | `v1` |
| MCP Notes/Agenda | `v1` |

O gateway e o agente remoto devem usar os binários desta revisão `v0.9.0`;
CLI e App Apple que implementam Voice v1 permanecem compatíveis no contrato.
O smoke com serviços reais e a medição comparativa de latência pertencem à Fase 5.

## v0.8.0 — Porta de sessão e caracterização

- baseline de Voice v1, Nova, confirmação, idempotência e privacidade;
- porta `AgentSession` in-process e handler público desacoplado da semântica;
- métricas de primeiro áudio com média, p50 e p95.
