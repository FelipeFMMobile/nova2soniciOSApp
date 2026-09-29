# Agent Session v1alpha1

Contrato interno experimental entre `voice-gateway` e aplicações agênticas.
Cada conexão WebSocket representa exatamente uma sessão e usa somente frames
JSON de texto. O identificador de protocolo é `agent-session/v1alpha1`; não há
downgrade automático.

O gateway escolhe `agentId` e endpoint de um catálogo local confiável; esses
campos nunca vêm do cliente Voice v1. O upgrade envia bearer derivado (hex HMAC-SHA256 da chave compartilhada sobre
`protocolo + NUL + client-bearer + NUL + agentId`),
`X-STS-Agent-Protocol` e nonce aleatório. O agente prova sua identidade com
`X-STS-Agent-Proof`: hex HMAC-SHA256 da chave compartilhada sobre
`protocolo + NUL + agentId + NUL + nonce`. O nonce em `X-STS-Agent-Nonce`
contém 32 bytes aleatórios codificados em hexadecimal. O header de autorização
é `Authorization: Bearer <credencial derivada>`. `ws://` só é permitido em
loopback; rede não local exige uma versão futura com TLS/mTLS.

A primeira mensagem é `session.open`; o agente responde `session.accepted`
com o mesmo protocolo e `agentId`. Versão, agente, token ou forma incompatíveis
fecham antes do áudio. Comandos usam `session.command`; eventos usam
`session.event`.

Áudio PCM16 LE é base64, limitado a 128 KiB por mensagem. Cada frame carrega
sessão, turno e sequência. Frame órfão, sequência zero, PCM ímpar, excesso de
tamanho e campo desconhecido são rejeitados. Mensagens têm no máximo 256 KiB e
as filas de transporte têm oito posições.

Perda do agente produz erro recuperável no Voice v1 e encerra a sessão. O
gateway não repete áudio, commit, confirmação ou tool. Uma nova conexão pode
reutilizar `requestId`; agente e MCPs preservam idempotência e resultado
desconhecido. `agent.session.closed` encerra normalmente os dois lados.

`fixtures.json` contém vetores positivos e negativos. Os testes de `agentwire`
cobrem também autenticação, catálogo, limites e transporte.

A chave compartilhada nunca é enviada no upgrade. O bearer derivado não permite
produzir a prova do servidor. Esse mecanismo não cifra o tráfego nem protege
contra interceptação ativa com relay; o rollout permanece restrito a loopback
confiável. Exposição em rede exige TLS/mTLS e revisão do threat model.
