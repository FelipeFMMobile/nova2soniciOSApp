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

### Assistente de autoatendimento — um terminal

```bash
cd /Users/felipemenezes/Codes/AIproj/StsModel
make dev
```

O assistente pergunta:

1. Fake (sem AWS/microfone/MCPs) ou Nova 2 Sonic.
2. Perfil AWS existente, com `TerraformUser` como padrão para Nova.
3. MCPs: nenhum, Notes, Agenda ou ambos.
4. Se deseja dados fictícios na Agenda (padrão: não).
5. Mac/Simulator (loopback) ou iPhone físico (rede local, com confirmação).
6. Token local: digite um ou pressione Enter para gerar um token forte.

Após o resumo e sua confirmação, verifica ferramentas/portas, autenticação AWS
(Nova), compila os binários e inicia bridge + gateway supervisionados no mesmo
terminal. Se faltar o ambiente Python, oferece instalar com `make nova-install`.
Não cria perfis/recursos AWS nem altera IAM. Credenciais AWS não são salvas pelo
assistente; somente o perfil selecionado é utilizado. Conversas Nova têm custos
Bedrock; a checagem de prontidão não inicia inferência.

Ao ficar pronto, mostra URL, provider e token para preencher no app. No iPhone,
substitua `IP-DO-MAC` pelo IP do Mac na mesma rede. `ws://` não criptografa
voz/token: habilite rede local somente em Wi-Fi privado confiável. A bridge
permanece em `127.0.0.1:8091` mesmo nesse modo.

`Ctrl+C` encerra somente os processos iniciados pelo assistente, incluindo os
MCPs filhos. Se um serviço cair, ele encerra os demais e informa onde estão os
logs. Portas ocupadas são rejeitadas: não mata nem reaproveita processos
existentes. Bancos persistem em `data/`; logs privados em `logs/dev/`, ambos
ignorados pelo Git. Configurações `STS_*`/`NOVA_*` herdadas são descartadas para
não ativar MCPs ou captura de evidência sem sua escolha. Cada execução preserva
os aliases/políticas do exemplo MCP. Fixtures opt-in usam a reunião fictícia de
16/05/2030, 10–11h; nenhum banco é apagado.

Para conferir as escolhas sem autenticar, instalar, compilar ou iniciar:

```bash
make dev-plan
```

Testes do assistente: `make dev-test`. Para incluir startup/cleanup reais do
gateway fake, sem AWS: `STS_DEV_E2E=1 make dev-test`.
O teste de integração usa uma porta livre temporária para não interferir em um
gateway já rodando. Para iniciar outro ambiente em portas diferentes:

```bash
python3 scripts/dev.py --gateway-port 8082 --bridge-port 8092
```

Os comandos manuais abaixo continuam disponíveis para diagnóstico.

### Início rápido — perfil existente `TerraformUser`

Se as dependências e permissões IAM já estão configuradas, use diretamente os
dois blocos abaixo. Não é necessário criar outro perfil nem executar
`aws configure` novamente. Se faltar o ambiente Python, execute
`make nova-install` uma vez na raiz do projeto.

**Terminal 1 — ponte privada com a AWS:**

```bash
cd /Users/felipemenezes/Codes/AIproj/StsModel

export AWS_PROFILE=TerraformUser
export AWS_REGION=us-east-1
export NOVA_MODEL_ID=amazon.nova-2-sonic-v1:0
export NOVA_VOICE_ID=carolina

aws sts get-caller-identity --profile TerraformUser --region us-east-1
make nova-bridge
```

Confira a identidade retornada antes de iniciar a ponte. O comando STS verifica
autenticação, **não** a permissão de invocar o Nova. A ponte deve informar que
está ouvindo em `ws://127.0.0.1:8091`; deixe o processo rodando.

**Terminal 2 — gateway Go para o app:**

```bash
cd /Users/felipemenezes/Codes/AIproj/StsModel

export STS_PROVIDER=nova
export STS_NOVA_BRIDGE_URL=ws://127.0.0.1:8091
export STS_GATEWAY_ADDRESS=127.0.0.1:8080
export STS_DEVELOPMENT_TOKEN=local-demo-token

make gateway
```

O gateway deve informar que está ouvindo em `127.0.0.1:8080`, com provider
`nova`. No app macOS ou iOS Simulator, abra **Conexão local** e informe:

- URL: `ws://127.0.0.1:8080/v1/voice`
- Token: `local-demo-token` (token de desenvolvimento local, não uma chave AWS)
- Provider: **Nova 2 Sonic**

Toque **Iniciar conversa**, permita o microfone e fale em PT-BR. Esse início
rápido não configura ferramentas MCP: para Notes + Agenda, configure
`STS_MCP_SERVERS` no terminal 2 antes de `make gateway`, conforme a seção
**Consultar Notes e Agenda na mesma conversa** mais abaixo. Variáveis já exportadas
nesse terminal continuam valendo; use uma nova aba para um ambiente sem MCPs.

No iPhone físico, `127.0.0.1` não aponta ao Mac. Veja o
[guia do app](docs/apple-app.md#iphone-físico--etapa-5) para rede local,
assinatura e requisitos de versão do iOS.

**Encerrar:** primeiro encerre a conversa no app; depois pressione `Ctrl+C` nos
dois terminais. Subir os processos apenas os deixa ouvindo localmente; a sessão
Bedrock é aberta ao iniciar a conversa Nova e gera custos de uso. Mantenha a
ponte privada em loopback, sem expor sua porta à rede.

### Por que gateway + bridge? Por que dois terminais?

O fluxo desta implementação é:

```text
App SwiftUI → gateway Go (:8080) → bridge Python (:8091) → AWS Bedrock / Nova
                     ↕
             MCP Notes + Agenda / SQLite
```

- **Gateway Go:** protocolo do app, sessões, validação, permissões, execução
  das ferramentas MCP e persistência. Não recebe as chaves AWS do app.
- **Bridge Python:** adaptador de transporte. Usa o SDK
  `aws_sdk_bedrock_runtime` para abrir `InvokeModelWithBidirectionalStream`,
  autenticar com o perfil AWS e transportar áudio/eventos/resultados das tools.
  Não executa ferramentas nem contém as regras de negócio do orquestrador.

A ponte é uma escolha desta POC para isolar o transporte bidirecional da AWS;
não é um serviço adicional hospedado na AWS, nem uma exigência de que o Nova
seja acessado em dois processos. Ela pode ser substituída por um transporte Go
no futuro, após validar suporte/compatibilidade e manter os testes do protocolo.

São **dois processos locais**, não uma obrigação de usar duas janelas de
terminal. Os comandos ficam em primeiro plano para facilitar ver logs e parar
cada serviço com `Ctrl+C`; por isso usamos duas abas. Poderíamos iniciar ambos
com um supervisor/script, mas ainda seriam dois processos. Com o app não há
terceiro terminal obrigatório: ele substitui o cliente de voz de linha de comando.

### Configuração inicial — somente se ainda necessária

Execute os comandos na raiz do repositório. A conexão com o Bedrock é feita
somente pela ponte Python no Mac; o gateway Go e o app Apple não precisam
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
abaixo; para apenas Notes, use `make mcp-gateway`; para Notes + Agenda, use a
configuração `STS_MCP_SERVERS` da seção de dois MCPs. O `STS_DEVELOPMENT_TOKEN`
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

For the Apple app, use the two-terminal quick start above. A third terminal is
needed only for the WAV command-line tests below. The local token is your choice and is unrelated to AWS
credentials. The Python bridge uses the AWS profile already authorized for
Bedrock; set `AWS_PROFILE` only if you need a named profile.

Terminal 1 — private Python/Bedrock bridge:

```bash
make nova-install
export AWS_PROFILE=TerraformUser # or your existing authorized profile
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

## Consultar Notes e Agenda na mesma conversa

Esta demo usa dois MCPs reais, dados fictícios e seleção automática do Nova.
A chamada conjunta foi validada: Notes retornou o código **9274** e Agenda uma
reunião em **16/05/2030, 10–11h America/Sao_Paulo**. A preparação por voz e a
consulta fazem chamadas reais ao Bedrock e geram cobrança de áudio/texto.

**Terminal 1:** reutilize seu perfil AWS e inicie a ponte, conforme a configuração
acima (`make nova-bridge`, porta padrão 8091).

**Terminal 2:** encerre o gateway anterior e prepare bancos isolados da demo:

```bash
make mcp-build agenda-build
mkdir -p artifacts/readme-multi-mcp
chmod 700 artifacts/readme-multi-mcp
python3 - <<'CONFIG'
import json
from pathlib import Path
root = Path.cwd()
demo = root / "artifacts/readme-multi-mcp"
servers = json.loads((root / "docs/mcp-servers.example.json").read_text())
for server in servers:
    kind = "notes" if server["alias"] == "memo" else "agenda"
    server["command"] = str(root / f"bin/mcp-{kind}")
    server["args"] = ["-db", str(demo / f"{kind}.sqlite")]
    if kind == "agenda":
        server["args"].append("-fixtures")
path = demo / "servers.json"
path.write_text(json.dumps(servers))
path.chmod(0o600)
CONFIG
unset STS_MCP_COMMAND STS_MCP_ARGS STS_MCP_ALLOWED_TOOLS
export STS_PROVIDER=nova
export STS_GATEWAY_ADDRESS=127.0.0.1:8080
export STS_NOVA_BRIDGE_URL=ws://127.0.0.1:8091
export STS_DEVELOPMENT_TOKEN=local-demo-token
export STS_MCP_SERVERS="$(cat artifacts/readme-multi-mcp/servers.json)"
export STS_DATABASE_PATH="$PWD/artifacts/readme-multi-mcp/audit.sqlite"
export STS_MCP_EVIDENCE_PATH="$PWD/artifacts/readme-multi-mcp/private-tools.jsonl"
make gateway
```

**Terminal 3:** crie a nota fictícia por voz e consulte os dois MCPs. `say` gera
áudio local; `afconvert` produz WAV mono PCM16/16kHz para o cliente:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
say -v Luciana -r 165 -o artifacts/readme-multi-mcp/create.aiff \
  'Crie uma nota com o título Aurora e o conteúdo: o código fictício de validação é nove dois sete quatro.'
afconvert -f WAVE -d LEI16@16000 -c 1 \
  artifacts/readme-multi-mcp/create.aiff artifacts/readme-multi-mcp/create.wav
GOCACHE=/private/tmp/sts-go-cache go run ./cmd/voice-client -provider nova \
  -wav "$PWD/artifacts/readme-multi-mcp/create.wav" \
  -request-id readme-aurora-fixture-1 -expect-tool memo.notes.create \
  -events "$PWD/artifacts/readme-multi-mcp/create-events.jsonl" \
  -output "$PWD/artifacts/readme-multi-mcp/create-response.wav"

say -v Luciana -r 165 -o artifacts/readme-multi-mcp/query.aiff \
  'Consulte minhas notas e me diga o código da nota Aurora. Consulte também minha agenda local no dia dezesseis de maio de dois mil e trinta e me diga quais eventos estão agendados nesse dia.'
afconvert -f WAVE -d LEI16@16000 -c 1 \
  artifacts/readme-multi-mcp/query.aiff artifacts/readme-multi-mcp/query.wav
GOCACHE=/private/tmp/sts-go-cache go run ./cmd/voice-client -provider nova \
  -wav "$PWD/artifacts/readme-multi-mcp/query.wav" \
  -request-id readme-two-mcp-query-1 \
  -expect-tool memo.notes.list,local.agenda.list_events \
  -events "$PWD/artifacts/readme-multi-mcp/query-events.jsonl" \
  -output "$PWD/artifacts/readme-multi-mcp/query-response.wav"
afplay artifacts/readme-multi-mcp/query-response.wav
```

O terminal deve mostrar resultados `ok` de `memo.notes.list` e
`local.agenda.list_events`, seguidos da resposta com o código e o evento.
`-expect-tool` verifica resultados depois da chamada; **não força a seleção** do
modelo. Na demo validada, Agenda foi consultada duas vezes; consultas não têm
efeitos. Confira `start_local/end_local` (10–11h), pois `start/end` são UTC
(13–14h). Não há garantia de reprodução idêntica da fala do modelo.

Os bancos/WAV/JSONL ficam privados e ignorados pelo Git. Reutilize o requestId
da preparação para retry sem duplicar a nota; outra criação deliberada usa novo
requestId. Encerre ponte e gateway com Ctrl+C após a demo.

Testes locais, sem novas chamadas AWS: `make check` e `make nova-test`.
Para verificar as **capturas da demo já executada nesta task**, se disponíveis:

```bash
python3 tests/e2e/multi_mcp_voice_evidence.py artifacts/multi-mcp-live
```

Esse verificador usa os arquivos originais da task, não os novos arquivos do
roteiro acima. Veja [configuração, políticas, limites e relatório real](docs/multi-mcp-agenda.md).
Outros cenários AWS permanecem fora desta validação; mocks comprovam roteamento,
não seleção do modelo.

## App iOS e macOS

Abra `apps/apple/NovaVoice.xcodeproj` no Xcode 27, scheme `NovaVoice`, e escolha
My Mac ou iPhone Simulator iOS 27. Em **Conexão local**, informe URL/token do
gateway e provider correspondente. O botão inicia/encerra a conversa contínua.
Há transcrições e resultados das ferramentas Notes/Agenda, sem credenciais AWS
no app. Comece pelo modo fake, que não usa microfone nem AWS.

Veja [como executar e testar o app](docs/apple-app.md) para configuração Xcode,
Nova + dois MCPs, rede local no iPhone e limites de idempotência. SDK 27 com
deployment macOS 26/iOS 27; validação física e aceite de áudio ainda pendentes.

## Repository layout

- `apps/apple`: shared SwiftUI application for iOS and macOS.
- `cmd/gateway`: public HTTP/WebSocket gateway.
- `cmd/voice-client`: terminal demonstration and WAV capture.
- `cmd/mcp-notes`: local MCP notes server.
- `cmd/mcp-agenda`: fictitious local SQLite agenda MCP server.
- `internal`: protocol, provider, orchestration, audio, and persistence code.
- `services/nova-bridge`: local adapter for the AWS bidirectional SDK.
- `deploy/aws`: Bedrock IAM policy and setup runbook.
- `tests/e2e`: black-box scenarios and performance harnesses.

See [Architecture](docs/architecture.md) for responsibilities and trust
boundaries.
