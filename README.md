# STS Model POC

Proof of concept for a Brazilian Portuguese speech-to-speech assistant. The
Apple client streams microphone audio to a local Go gateway. The gateway owns
session state, interruption, permissions, and MCP tool execution, while Amazon
Nova 2 Sonic provides managed bidirectional inference through AWS Bedrock.

## Prerequisites

- macOS 26 with Xcode 27
- Go 1.27 or newer
- Python 3.12 for the small Bedrock transport bridge
- An AWS account with Nova 2 Sonic access in `us-east-1`

## Configurar o ambiente para conectar à AWS

Execute os comandos na raiz do repositório. A conexão com o Bedrock é feita
somente pela ponte Python no Mac; o gateway Go e o futuro app Apple não precisam
receber as credenciais AWS. Para voz bidirecional, use credenciais AWS padrão
com SigV4: **uma API key do Bedrock não funciona para essa operação**.
[Compatibilidade da API AWS](https://docs.aws.amazon.com/bedrock/latest/userguide/models-api-compatibility.html).

### 1. Instalar as ferramentas

Instale a [AWS CLI v2 para macOS](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html),
Go 1.27+ e Python 3.12. Confira as versões e instale as dependências da ponte:

```bash
aws --version
go version
python3.12 --version
make nova-install
```

### 2. Autorizar o usuário ou a role

Um administrador deve conceder à identidade usada pelo perfil a política
mínima em [deploy/aws/iam-policy.json](deploy/aws/iam-policy.json). Ela permite
`bedrock:InvokeModelWithBidirectionalStream` apenas para:

```text
arn:aws:bedrock:us-east-1::foundation-model/amazon.nova-2-sonic-v1:0
```

No IAM, aplique essa política ao usuário/role; para IAM Identity Center, ela
deve fazer parte das permissões da role atribuída à conta. `AmazonBedrockFullAccess`
também pode conceder acesso, mas é mais ampla que o necessário para esta POC.
Restrições da organização, permissions boundaries ou um deny explícito ainda
podem bloquear a chamada. O projeto não cria usuários nem altera políticas IAM.

### 3. Configurar um perfil AWS

**Opção recomendada — IAM Identity Center (SSO), se disponível na sua conta:**

```bash
aws configure sso --profile sts-poc
aws sso login --profile sts-poc
```

No assistente, informe a URL de acesso SSO e a região do Identity Center
fornecidas pelo administrador; selecione a conta e a role autorizadas. Configure
a região padrão dos serviços como `us-east-1` e a saída como `json`. A região
do Identity Center pode ser diferente da região do Bedrock. Repita `aws sso login`
quando a sessão expirar.
[Guia oficial de configuração SSO](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html).

**Alternativa — perfil de usuário IAM, quando SSO não estiver disponível:**

```bash
aws configure --profile sts-poc
```

Informe a Access Key ID e a Secret Access Key **somente no prompt local da CLI**,
a região `us-east-1` e a saída `json`. Não use chaves do usuário root. A CLI
armazena o perfil fora do projeto, em `~/.aws/config` e `~/.aws/credentials`;
nunca copie esses arquivos ou chaves para Git, `.env`, código ou configurações
Xcode. Prefira credenciais temporárias quando possível. Não envie chaves pelo chat.

**Se já houver um perfil configurado**, reutilize-o sem sobrescrever credenciais:

```bash
aws configure list-profiles
```

`sts-poc` é apenas o nome sugerido: ele só existe depois da configuração. Se
usar `default`, substitua `sts-poc` por `default` nos comandos abaixo.

### 4. Selecionar o perfil e verificar a identidade

No terminal que iniciará a ponte:

```bash
export AWS_PROFILE=sts-poc
export AWS_REGION=us-east-1
export NOVA_MODEL_ID=amazon.nova-2-sonic-v1:0
export NOVA_VOICE_ID=carolina
aws sts get-caller-identity --profile "$AWS_PROFILE" --region "$AWS_REGION"
```

Confira se a conta e o usuário/role retornados são os esperados. Esse comando
valida a autenticação, **não** a permissão de invocar o Nova. Variáveis exportadas
valem apenas naquele terminal; `.env.example` é uma referência e não é carregado
automaticamente. Evite variáveis AWS de credenciais conflitantes com o perfil.

### 5. Iniciar a ponte e testar áudio real

No mesmo terminal do passo anterior:

```bash
make nova-bridge
```

Deixe-o aberto. Em outro terminal, envie um WAV PT-BR mono PCM16 de 16 kHz:

```bash
make nova-smoke WAV=/absolute/path/sample-pt-br.wav
```

O smoke test usa a ponte já autenticada, recebe áudio falado e salva
`/private/tmp/sts-nova-response.wav`. **Ele faz uma chamada real à AWS e gera
cobrança do Bedrock.** Para testar também o gateway Go, siga a seção Stage 3
abaixo; para ferramentas MCP, use `make mcp-gateway`. O `STS_DEVELOPMENT_TOKEN`
é escolhido por você para proteger a conexão local e **não é uma credencial AWS**.
Encerre a ponte e o gateway com Ctrl+C ao terminar.

Problemas comuns:

- Perfil inexistente: confira `aws configure list-profiles` e o `AWS_PROFILE`.
- Credenciais ausentes/expiradas: configure o perfil ou renove o login SSO;
  depois reinicie a ponte para abrir uma nova sessão autenticada.
- `AccessDenied` ou falha de startup: confirme a identidade ativa, a política
  mínima, a região e as restrições da conta/organização. A ponte expõe erros
  sanitizados; eles podem não incluir a causa detalhada do Bedrock.
- Não há áudio: confira o formato do WAV, a conexão de rede com o endpoint
  Bedrock e a disponibilidade do modelo na região escolhida.

Veja também [AWS setup](deploy/aws/README.md) e o
[guia oficial do Nova 2 Sonic](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-getting-started.html).

## Local commands

```bash
make check
export STS_PROVIDER=fake
export STS_DEVELOPMENT_TOKEN=local-demo-token
make gateway
```

In another terminal, export the same token, then run:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make voice-demo
make voice-cancel
```

The normal demo saves `/private/tmp/sts-fake-response.wav`. It is a 440 Hz
tone, not speech: the fake provider does not perform ASR or call AWS. The
cancellation demo discards its output. Use `go run ./cmd/voice-client -help`
for an alternate URL, input WAV, or output path.

Configuration comes from process environment variables. `.env.example` is a
reference template; `.env` is **not automatically loaded**. Development may run
without a token only on loopback. Non-loopback bindings and production require
a token. Runtime data and secrets are intentionally excluded from Git.

See [Voice protocol](docs/voice-protocol.md) for endpoints, event examples,
limits, latency definitions, and the Stage 2 validation results.

## Real Nova 2 Sonic through the gateway (Stage 3)

Use three terminals. The local token is your choice and is unrelated to AWS
credentials. The Python bridge uses the AWS profile already authorized for
Bedrock; set `AWS_PROFILE` only if you need a named profile.

Terminal 1 — private Python/Bedrock bridge:

```bash
make nova-install
export AWS_PROFILE=sts-poc # or the existing profile you configured above
export AWS_REGION=us-east-1
make nova-bridge
```

Terminal 2 — public Go gateway:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
export STS_PROVIDER=nova
make gateway
```

Terminal 3 — send an actual PT-BR WAV (mono PCM16, 16 kHz):

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make nova-demo WAV=/absolute/path/input-pt-br.wav
make nova-barge-demo WAV=/absolute/path/input-pt-br.wav BARGE_WAV=/absolute/path/interruption-pt-br.wav
```

The first command saves the spoken response. The second injects the second
recording during the first response, requires native Nova interruption, clears
old output, and saves only the new response. These commands incur Bedrock
usage charges. Stop the Go gateway and Python bridge with Ctrl+C when finished.
Recordings are ignored by Git. See [Nova integration](docs/nova-integration.md)
for session behavior, limitations, and the live validation report.

## Nova with MCP tools (before the Apple apps)

Keep the Nova bridge running, then replace `make gateway` with:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make mcp-gateway
```

Go discovers the local notes server's tools and presents them to Nova. The
model requests tools; Go executes MCP calls and returns their actual results
for spoken responses. Notes create/list/delete, durable retry, and subsequent
voice confirmation before deletion have passed real AWS validation.

The terminal supports `-expect-tool notes.list`, `-request-id` for safe retry,
`-events /private/tmp/events.jsonl` for private evidence, and `-followup-wav`
for a second voice turn. No SwiftUI app is needed for this demonstration.
See [MCP setup, safety and live results](docs/mcp-integration.md).

## Repository layout

- `apps/apple`: shared SwiftUI application for iOS and macOS.
- `cmd/gateway`: public HTTP/WebSocket gateway.
- `cmd/voice-client`: terminal demonstration and WAV capture.
- `cmd/mcp-notes`: local MCP notes server.
- `internal`: protocol, provider, orchestration, audio, and persistence code.
- `services/nova-bridge`: local adapter for the AWS bidirectional SDK.
- `deploy/aws`: Bedrock IAM policy and setup runbook.
- `tests/e2e`: black-box scenarios and performance harnesses.

See [Architecture](docs/architecture.md) for responsibilities and trust
boundaries.

The additional pre-Apple stage supports two simultaneous stdio MCPs, Notes and
a fictitious local SQLite Agenda. See [configuration, local tests and PT-BR AWS
acceptance script](docs/multi-mcp-agenda.md). An authorized real Nova demo selected both MCPs and returned the stored code
and correct local event time after a timezone presentation fix. Other live
acceptance scenarios remain pending; mock tests establish host routing only.
