# Implementação — separação gateway/agente

Base: `v0.7.0`. Branch: `codex/agent-migration-v1`.

Este registro acompanha o plano em
[`agent-migration-plan.md`](agent-migration-plan.md). Estado planejado não é
tratado como implementado antes de cada gate passar.

## Fase 1 — caracterização

Estado: implementada na branch; aguardando integração.

Baseline adicionado:

- `contracts/voice/v1/fixtures.json` congela mensagens públicas válidas,
  rejeições e eventos representativos;
- teste do prompt atual impede mudança incidental antes de sua transferência;
- confirmação por voz/UI, renovação e barge-in continuam cobertos pelas suítes
  existentes;
- teste de mutação pendente impede segunda execução antes do resultado;
- teste de desconexão simula efeito durável cuja resposta foi perdida e prova
  replay com o mesmo `requestId`, sem duplicar o efeito;
- teste de logs usa conteúdo sentinela para impedir vazamento de transcrição,
  argumentos e resultado;
- `/v1/metrics` passa a expor p50/p95 de primeiro áudio por janela limitada,
  além da média existente.

O teste de desconexão usa um backend determinístico em memória para reproduzir
o limite entre efeito confirmado no executor e resposta perdida no transporte.
Os testes e2e existentes continuam cobrindo o ledger SQLite real entre sessões.

Validação executada e aprovada:

- testes direcionados sem cache dos contratos, actions, Nova e adaptador de voz;
- `make check`: build, testes, race detector, vet e fronteiras de todos os
  módulos;
- `git diff --check`, validação JSON das fixtures e check do harness;
- checks Docker/AWS permanecem opt-in, pois não houve mudança de deploy.

## Fase 2 — porta AgentSession in-process

Estado: não iniciada neste registro.

O próximo passo é introduzir a fábrica de sessão completa e adaptar primeiro o
caminho Nova, sem processo remoto nem mudança do Voice Protocol v1.
