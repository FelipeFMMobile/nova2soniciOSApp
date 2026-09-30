# Fechamento do plano de separação gateway/agente

Data: 2026-09-30. Branch: `codex/agent-migration-phase6`.

## Resultado

Os critérios de desenvolvimento das seis fases foram fechados. O gateway
autentica a borda, atribui um principal confiável, escolhe agente e endpoint por
configuração e encaminha Voice v1 para Agent Session. O agente possui conversa,
Nova, ferramentas, confirmação, perfis MCP e produção de Host Context. Notes e
Agenda continuam donos de regras, dados e ledgers.

`principal.subject` passou a ser obrigatório em `session.open`. Ele é obtido de
`STS_PRINCIPAL_ID` no gateway e nunca de Voice v1; fora de desenvolvimento a
configuração é obrigatória. Fixtures provam rejeição de principal ausente e de
tentativa do cliente público de fornecê-lo.

O contrato canônico Host Context v1 e seus vetores agora pertencem a
`personal-assistant-agent`, o produtor. `mcpwire` permanece biblioteca técnica e
valida a compatibilidade contra essa fonte única.

## Evidências executadas

- `make check`: build, testes, race detector, vet, fronteiras e builds isolados.
- `STS_DEV_E2E=1 python3 -m unittest discover -s tests/scripts -v`: 24 testes,
  incluindo início e encerramento dos processos fake.
- `make agent-clients`: gateway/agente em processos separados, CLI e VoiceCore.
- `make agent-latency`: delta p95 de 0,1110 ms com uma sessão e 0,3976 ms com
  oito sessões, abaixo do gate de 10 ms.
- imagens independentes `sts-voice-gateway:local` e
  `sts-personal-assistant-agent:local` construídas; a CI repete ambas.
- builds completos macOS e iOS Simulator aprovados.
- harness, JSON e `git diff --check` aprovados.

O gate automatizado `make litellm-privacy-audit` identificou quatro registros
históricos de 2026-09-19 com argumentos MCP, anteriores ao patch de redação.
`make litellm-redact-history` removeu somente `arguments`/`result`, preservando
os 30 registros e seus dados de custo/atribuição. A repetição do audit passou
sobre todos os 30 registros, sem imprimir conteúdo.

## Limites deliberados

Inferência AWS real e teste em iPhone físico continuam opt-in porque geram custo
ou dependem de hardware. Eles não são necessários para a separação local já
caracterizada. Exposição do Agent Session fora de loopback continua proibida;
uma evolução para rede exige TLS/mTLS e threat model próprios.
