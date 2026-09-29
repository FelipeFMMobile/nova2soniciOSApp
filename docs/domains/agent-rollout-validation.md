# Validação do rollout — Fase 5

Data: 2026-09-29. Base: `v0.9.0` (`1d089db`).
Entrega: `v0.10.0`, branch `codex/agent-migration-phase5`.

## Decisão

Agent Session foi promovido para `v1`; remoto é o padrão de configuração.
`make dev` inicia Nova com agente e gateway separados e credencial interna
aleatória por execução. A demo fake seleciona in-process explicitamente.
`python3 scripts/dev.py --agent-mode inprocess` mantém o rollback em toda a
minor `v0.10.x`. A remoção do legado não foi executada.

O catálogo e o agente exigem `agent-session/v1` em `/v1/session`. Não há downgrade
para alpha nem fallback automático quando o agente remoto está indisponível.
Gateway e agente devem ser atualizados juntos. Voice v1, Host Context v1 e MCP
Notes/Agenda v1 permanecem compatíveis.

## Evidências executadas

- `make check`: builds isolados, testes, race detector, vet e fronteiras.
- `TestAgentRolloutParity`: 14 cenários, cada um executado em in-process,
  remoto e novamente in-process. Cada execução possui sessão própria, sem
  espelhamento de mutação real. Cobertura inclui áudio contínuo, barge-in,
  histórico/renovação, descoberta, confirmação posterior, timeout, microfone
  durante tool, desconexão após efeito durável, logs privados e dois MCPs.
- `make agent-clients`: binários reais gateway/agente em processos separados,
  CLI sem alteração de produção, VoiceSocket/VoiceInputPump/ConversationState
  Apple existentes, saída PCM, fechamento e reconexão, nos dois modos e rollback.
- `make apple-core-test`: 25 testes, zero falhas; o teste de rede opt-in foi
  pulado nessa chamada e executado separadamente em `make agent-clients`.
- `STS_DEV_E2E=1 python3 -m unittest discover -s tests/scripts -v`: 25 testes,
  incluindo processo fake real e encerramento do grupo de processos.
- Testes de separação de ambientes provam que modelo/MCP não chegam ao gateway
  remoto e que o token público não chega ao agente.
- Logs dos processos são verificados contra texto sentinela e credenciais de
  teste. Os MCPs usam bancos SQLite temporários reais nos cenários duráveis.
- Fixtures v1 cobrem todos os tipos de comando/evento; alpha é rejeitado.
- Harness, JSON, `git diff --check` e diagrama regenerado/verificado.

O modelo Realtime desses testes é um servidor local determinístico. Os testes
validam o runtime real do agente e os protocolos, sem afirmar validação Bedrock.
No teste Apple foi corrigida apenas a expectativa de teste: um close frame após
`session.stopped` é fechamento normal. O código de produção Apple/CLI não mudou.

## Medição de latência

Gate aprovado pelo usuário: acréscimo de p95 **≤ 10 ms** no salto local.
Comando: `make agent-latency`, sem race detector, host macOS arm64/Go 1.27.1.

20 ms de PCM por mensagem (640 bytes de entrada, 960 de saída), 100 amostras de
warmup e 1.000 amostras medidas por sessão. Tempo entre `Send` e primeiro evento
de áudio; handshake, modelo e playback não entram. Sessões exercitadas em carga
contínua, sem pacing. O servidor de eco roda no mesmo processo de testes, mas
atravessa WebSocket TCP loopback real; o teste separado de binários valida o
funcionamento entre processos, sem atribuir a ele estes números de latência.

| Sessões | Amostras | In-process p50 | In-process p95 | Remoto p50 | Remoto p95 | Acréscimo p95 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 1.000 | 0,0002 ms | 0,0006 ms | 0,0776 ms | 0,1159 ms | **0,1153 ms** |
| 8 | 8.000 | 0,0005 ms | 0,0045 ms | 0,1614 ms | 0,4605 ms | **0,4560 ms** |

Gate local aprovado. Resultados dependem da carga e do host; não constituem SLA
cloud nem medição de qualidade de voz. O teste opt-in falha se o delta exceder
10 ms; não adiciona um limiar temporal instável à suíte unitária comum.

## Correções identificadas durante o gate

- Removida exigência indevida de configuração LiteLLM no gateway Nova remoto.
- Heartbeat interno preserva sessão silenciosa enquanto pong chega no prazo.
- Eventos remotos atualizam contadores de conclusão, interrupção, erro e métricas
  de primeiro áudio do agente; o benchmark acima mede o salto separadamente.
- Supervisor inicia agente antes do gateway, espera saúde, separa ambientes e
  encerra os filhos; fake não exige Docker.
- CI inclui os módulos `agentwire` e `personal-assistant-agent`.

## Limites e operação

Não executados: inferência AWS real, auditoria de banco Usage/Spend ativo,
interface em iPhone físico, builds completos do app Apple, imagens Docker ou
deploy. Esses passos continuam opt-in; a ausência de conteúdo no LiteLLM ativo
não é inferida dos testes do gateway/agente. A configuração local LiteLLM não
estava presente nesta worktree. O rollout segue restrito a loopback confiável;
exposição em rede exige TLS/mTLS e revisão específica.

Para rollback operacional, encerre a sessão corrente e reinicie pelo supervisor
com `--agent-mode inprocess`. Para rollback de binários, volte gateway, agente e
catálogo juntos a `v0.9.0`. Não execute os dois modos para a mesma mutação. A Fase 6
só deve remover o caminho legado após a janela minor prometida.
