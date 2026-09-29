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

Estado: implementada na branch; aguardando integração.

Foi introduzida uma porta de sessão completa em `internal/ports`, com abertura,
comandos, stream de eventos e fechamento. Seus tipos não importam Voice v1:
áudio atravessa a fronteira como PCM decodificado e confirmação tem um comando
próprio. A abertura já carrega `sessionId`, `requestId`, agente padrão e provider
legado; seleção pública de agente continua fora de escopo.

O handler público agora possui somente WebSocket, autenticação já existente,
limites, heartbeat, validação de sessão e tradução Voice v1 ↔ AgentSession. A
fábrica padrão mantém Nova/fake no mesmo processo. Handshake, ping e fechamento
WebSocket saíram do loop Nova e ficaram no adaptador público.

Um teste substitui a fábrica in-process por uma sessão controlada e prova que
abertura, PCM, eventos, confirmação de UI e fechamento passam exclusivamente
pela porta. Toda a suíte anterior de Nova, MCP, confirmação, renovação,
barge-in, idempotência e privacidade passou sem mudança do contrato público.

O antigo handler fake foi mantido temporariamente sem registro e sem caminho
Nova, apenas como ponto de comparação durante esta fase. Ele deve ser removido
quando a extração da aplicação semântica consolidar o rollback pela própria
fábrica in-process.

Validação executada e aprovada:

- `make check`: build, testes, race detector, vet e fronteiras;
- teste sem cache do adaptador público e da porta;
- `git diff --check` e check do harness;
- nenhum teste Docker/AWS, pois processo, imagem e deploy não mudaram.

## Fase 3 — aplicação semântica independente

Estado: não iniciada neste registro.

O próximo passo é mover a implementação atrás da porta para
`personal-assistant-agent`, preservando a fábrica in-process como rollback e
sem introduzir ainda o transporte remoto.
