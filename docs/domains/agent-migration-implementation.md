# Implementação — separação gateway/agente

Base: `v0.7.0`. Fases 1–3: `codex/agent-migration-v1`.
Continuação da Fase 4: `codex/agent-migration-phase4`.

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

## Fase 4 — Agent Session remoto v1alpha1

Estado: implementada na branch; remoto opt-in, in-process ainda padrão.

O contrato canônico foi publicado em
`apps/voice-gateway/contracts/agent-session/v1alpha1/`, com fixtures positivas
e negativas. `packages/go/agentwire` implementa framing JSON estrito, cliente,
servidor e conformidade. Áudio PCM16 permanece explicitamente vinculado a
sessão, turno e sequência; mensagens e áudio têm limites inferiores aos do
protocolo público e todas as filas são limitadas.

O gateway ganhou `STS_AGENT_MODE=remote` e um catálogo confiável. Cada entrada
fixa `agentId`, endpoint, versão e referência a token de ambiente. Bearer derivado e uma
prova HMAC de desafio autenticam os dois lados. O frontend
não envia agente nem endpoint. Agente ausente, versão incompatível, token
inválido, campo desconhecido e `ws://` fora de loopback falham fechados. Um
segundo agente, inclusive fake, é selecionado apenas mudando o catálogo e o
default, sem branch por nome no handler público.

O binário `personal-assistant-agent` agora serve o protocolo em loopback e
carrega sozinho prompt, LiteLLM e MCP. O gateway remoto rejeita na inicialização
qualquer configuração ou segredo de modelo/MCP. O bearer derivado permanece
restrito ao rollout local; bind de rede exigirá TLS/mTLS e threat model antes
de ser permitido.

Desconexão interna gera erro recuperável e encerra Voice v1 sem replay de
áudio, commit, confirmação ou tool. Os testes cobrem tradução completa pela
porta, roteamento entre dois agentes, autenticação, agente desconhecido,
limites, framing e o caminho gateway → agente remoto controlado.


### Retomada em 2026-09-29

As alterações não commitadas da sessão anterior foram recuperadas para a
worktree atual, preservando a cópia original. A revisão corrigiu a autenticação:
o bearer enviado agora é derivado da chave, que não atravessa o upgrade.
Um teste simula um processo indevido que tenta forjar a prova usando o bearer
recebido e verifica rejeição antes de enviar conteúdo da sessão.

Também foi corrigido o teste de barge-in da CLI: ele passa explicitamente o
caminho absoluto do prompt versionado, necessário desde a extração do agente,
sem depender da configuração do ambiente de quem executa a suíte.

A Fase 5 permanece pendente: testes espelhados com serviços reais, medição de
latência, validação Apple/CLI e promoção deliberada do protocolo para v1.

Validação desta retomada aprovada:

- `make check`: builds isolados, testes, race detector e `go vet` em todos os
  módulos; testes de rede executados com permissão para abrir portas loopback;
- check do harness, fronteiras de aplicações e JSON das fixtures;
- `git diff --check`.

Não foram executados smoke AWS/LiteLLM, builds Apple ou deploy nesta fase.


## Fase 5 — rollout local e Agent Session v1

Estado: implementada na branch `codex/agent-migration-phase5`, para `v0.10.0`.
O transporte remoto é o padrão; a demonstração fake do supervisor seleciona
explicitamente in-process. A compatibilidade pública permanece Voice v1.

A mesma suíte agora roda nos modos in-process, remoto e rollback: áudio contínuo,
barge-in, renovação, confirmação, falha/timeout, dois MCPs reais, replay durável,
desconexão após mutação e logs sem conteúdo. O teste entre processos executa os
binários do agente e gateway, a CLI existente e Apple VoiceCore. Realtime é
simulado localmente; AWS real continua opt-in, conforme o plano.

A revisão corrigiu a exigência indevida de segredo LiteLLM no gateway remoto,
métricas ausentes nesse modo e expiração de sessão silenciosa sem heartbeat.
O supervisor inicia agente e gateway com ambientes separados e uma chave
interna aleatória; `--agent-mode inprocess` exercita o rollback. Essa opção
permanece durante toda `v0.10.x`; sua remoção pertence à Fase 6.

O gate de acréscimo de p95 foi fixado pelo usuário em **10 ms** para o salto
local. Metodologia, valores, validações e limites estão no
[registro de rollout](agent-rollout-validation.md).

A promoção não é uma declaração de validação cloud ou de readiness para rede
remota: não foram executados inferência Bedrock real, auditoria de Usage/Spend
em banco ativo, teste em iPhone físico ou deploy. Nenhuma atualização do código
de produção Apple/CLI foi necessária para a compatibilidade demonstrada.


## Fase 6 — gateway exclusivamente remoto

Estado: implementada na branch `codex/agent-migration-phase6`, para `v0.11.0`.
O usuário autorizou o avanço após relatar boa execução local de `v0.10.0`.
A release `v0.10.x` e sua opção in-process não foram alteradas: a tag e branch
anteriores continuam disponíveis. A remoção acontece na nova minor, como previsto.

O gateway perdeu a dependência da aplicação agêntica, a conversão de configuração
semântica, a fábrica Nova e o runtime fake. Nova e fake pertencem agora ao agente;
o pacote `session` é a composição interna dessa aplicação, não um modo de execução
no gateway. A borda mantém somente autenticação pública, catálogo, transporte,
limites, tradução Voice v1/Agent Session v1 e métricas. Configurações semânticas
antigas no ambiente da borda são rejeitadas, não silenciosamente ignoradas.

Os cenários Nova/MCP foram movidos para `tests/voice` e continuam exercitando
confirmação, idempotência, renovação, desconexão e privacidade via gateway real.
Os testes de borda também usam o fake remoto. As regras de turnos e o gerador de
áudio fake, com seus testes existentes, foram movidos para a aplicação agente.

`make dev` inicia agente e gateway para ambos os ambientes. Fake continua sem
Docker/AWS. `--agent-mode inprocess` falha antes de iniciar processos. O rollback
agora consiste em restaurar o conjunto `v0.10.x`, incluindo configuração; não há
fallback nem migração de dados de negócio nesta remoção.

`make isolated` copia cada aplicação e apenas as bibliotecas técnicas para um
diretório temporário, compila com `GOWORK=off` e verifica que o binário do gateway
não depende de agente/MCP. A imagem do gateway não copia mais o agente. A CI
executa esse gate, além das verificações de fronteira sem a exceção temporária.

Validação local: `make check` (build/test/race/vet/isolated), 24 testes Python com
`STS_DEV_E2E=1`, incluindo início/encerramento do fake remoto; integração Apple/CLI
com modelo Realtime simulado; harness, diff e diagrama atualizado. Inferência AWS,
Usage/Spend ativo, builds Docker e iPhone físico não foram repetidos nesta fase.
O relato do usuário não é apresentado como evidência automatizada desses gates.

### Consolidação final

O gateway agora constrói um principal confiável depois da autenticação e envia
`principal.subject` na abertura de Agent Session. Voice v1 não expõe esse campo;
em desenvolvimento o sujeito padrão é `local-user`, enquanto outros ambientes
devem definir `STS_PRINCIPAL_ID`. A validação do protocolo rejeita principal
ausente ou inválido antes de entregar a sessão ao agente.

A fonte canônica do Host Context v1 e seus vetores foi movida para
`apps/personal-assistant-agent/contracts/host-context/v1/`, junto do produtor.
`mcpwire` continua sendo apenas a implementação técnica e valida os mesmos
vetores a partir da nova localização. Documentação de onboarding, MCP e Voice v1
foi alinhada às fronteiras finais.

A CI agora também constrói separadamente as imagens do gateway e do agente;
`make images` oferece o mesmo gate local. Isso valida os contextos e `COPY`
dos Dockerfiles sem tornar Docker requisito da suíte unitária `make check`.

O fechamento executado em 2026-09-30 está documentado no
[registro final](run-2026-09-30-agent-migration-closure.md).
