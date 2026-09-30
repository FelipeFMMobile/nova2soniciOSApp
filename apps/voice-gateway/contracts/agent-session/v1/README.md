# Agent Session v1

Contrato interno estável entre `voice-gateway` e aplicações agênticas.
Cada conexão WebSocket representa exatamente uma sessão e usa somente frames
JSON de texto. O identificador de protocolo é `agent-session/v1`; não há
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

`session.open.start.principal.subject` é obrigatório e contém uma identidade
opaca autenticada. O gateway a constrói a partir de configuração ou claims
confiáveis depois de autenticar o cliente; Voice v1 não oferece campo para o
frontend escolher ou substituir essa identidade. O agente rejeita principal
ausente ou inválido antes de iniciar a sessão.

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

## Ciclo de vida e concorrência

`Open` só retorna após autenticação, abertura e aceite da versão e identidade.
Uma conexão transporta uma sessão; `Events` é ordenado e fechado pelo leitor,
unca pelo consumidor. `Send` serializa escritas; `Close` é idempotente, interrompe
I/O e aguarda o leitor. Filas cheias aplicam backpressure; não há descarte de PCM
nem replay automático. Cada escrita possui deadline de no máximo cinco segundos
no cliente; o servidor aplica seus limites configurados e no máximo 32 sessões.

O servidor envia ping a cada um terço de seu idle timeout; pong renova esse prazo.
Uma sessão silenciosa continua válida enquanto o transporte responder. Fechamento
normal produz `agent.session.closed` antes do close frame. Desconexão inesperada
produz `agent_disconnected`; mensagem inválida produz `agent_protocol_error`, e
ambas encerram o stream. A reconexão cria uma sessão nova; não retoma áudio.

O agente valida sequência crescente contígua a partir de 1, turnos não reutilizados
e associação de commit/cancelamento. Voice v1 limita cada entrada a 32.000 bytes;
o envelope interno aceita até 128 KiB, dentro do limite total de 256 KiB. O turno
semântico e as políticas de confirmação permanecem responsabilidade do agente.

`agent-session/v1alpha1` não é negociado por estes binários. Para rollback restaure o conjunto `v0.10.x` (que suporta `inprocess`), ou reverta gateway e agente juntos para
`v0.9.0`; misturar versões do transporte falha antes de aceitar áudio.
