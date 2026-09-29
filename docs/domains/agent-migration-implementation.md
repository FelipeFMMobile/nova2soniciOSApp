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

Estado: implementada na branch; aguardando integração.

Foi criado `apps/personal-assistant-agent` com `go.mod`, manifesto, imagem,
binário e testes próprios. Prompt, Nova Realtime, actions, adapters MCP,
perfis de integração, confirmação, evidência privada e produção do Host
Context foram movidos para esse módulo. Notes e Agenda continuam donos de suas
regras, dados e ledgers.

A porta de sessão neutra foi promovida para `packages/go/agentwire`, sem
política de negócio. O gateway mantém somente Voice v1, autenticação, limites,
WebSocket, métricas de borda e tradução para `AgentSession`. Nova é carregada
por `personal-assistant-agent/inprocess`, dependência temporária e explícita de
rollback; o fake determinístico permanece como implementação local da mesma
porta. O handler legado não registrado foi removido.

O prompt canônico agora é `prompts/system.md`. Tanto o artefato independente
quanto a fábrica in-process recusam caminho relativo, arquivo ausente, vazio,
não regular ou maior que 64 KiB. O supervisor local injeta o caminho absoluto.

O modo remoto ainda não existe nesta fase. Portanto, credenciais LiteLLM/MCP
ainda entram no processo do gateway exclusivamente para alimentar o adapter
in-process. A condição de não haver esses segredos no gateway será exercitada
quando o modo remoto for implementado na Fase 4; não foi declarada como
atendida antecipadamente.

Validação executada e aprovada:

- builds e testes isolados do gateway e do agente com `GOWORK=off`;
- `make check`: build, testes, race detector, vet e fronteiras de todos os
  módulos, incluindo a exceção temporária restrita do adapter in-process;
- `git diff --check`, check do harness e verificação de fronteiras;
- Voice Protocol v1 e o comportamento de confirmação/idempotência permaneceram
  cobertos pelas mesmas suítes de integração.
