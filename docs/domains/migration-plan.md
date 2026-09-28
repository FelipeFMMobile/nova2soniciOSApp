# Plano de migração para aplicações modulares

Estado: migração implementada na branch `codex/modular-domain-migration`.
Fases 1–4 executadas; fase 5 entrega catálogo e imagens por app, mantendo stdio
na composição LiteLLM. Transporte MCP remoto segue fora do rollout atual.
Por decisão do usuário, a fase 6 prepara o repositório para Spring; a primeira
app Spring será criada depois, quando houver domínio definido.

Veja [registro de implementação](migration-review.md) para mudanças,
validações e limitações. O mapa de movimentação abaixo preserva as origens
históricas para revisão.

## Decisão

Adotar monorepo poliglota com aplicação como unidade de build/deploy e domínio
como fronteira semântica. O gateway Go terá módulos de conversação e execução
de ações no mesmo processo. Notes e Agenda continuam apps MCP independentes,
com modelos e persistência próprios. Novos MCPs Spring são apps pares, com seu
próprio build e ciclo de vida. Não reescrever as apps existentes em Java.

Manter os nomes públicos `notes.*`, `agenda.*`, aliases memo/local e protocolo
de voz na migração inicial. O nome da pasta não determina nome público de tool.
Adotar `apps/` como local canônico de aplicações (já existe para Apple),
evitar dividir aplicações entre `apps/` e `services/`. A pasta local
`services/nova-bridge` não tem arquivos versionados no inventário; não movê-la
ou apagá-la como parte dessa proposta.

## Estrutura implementada e extensão futura

```text
AGENTS.md
harness/                         # skill, perfis e roteiro já entregues
apps/
  apple/                         # localização atual preservada
  voice-gateway/
    go.mod
    cmd/gateway/
    contracts/
      voice/v1/                 # protocolo publicado pelo gateway
      host-context/v1/          # envelope assinado e vetores de teste
    config/integrations/
      notes/                    # aliases, allowlist, políticas e frases
      agenda/
    internal/
      conversation/              # sessão, turno, interrupção
      actions/                   # autorização, execução, resultado
      adapters/                  # WS, Realtime, cliente MCP
      bootstrap/                 # composição e configuração
      ports/                     # interfaces de inferência usadas pelo núcleo
  voice-client/
    go.mod
    cmd/voice-client/
  mcp-notes/
    go.mod
    cmd/mcp-notes/
    contracts/v1/               # schemas, resultados, erros e exemplos de Notes
    internal/notes/              # casos de uso e modelo
    internal/adapters/          # MCP e SQLite de Notes
    migrations/
  mcp-agenda/
    go.mod
    cmd/mcp-agenda/
    contracts/v1/               # schemas, resultados, erros e exemplos de Agenda
    internal/scheduling/         # intervalos, conflitos, disponibilidade
    internal/adapters/          # MCP e SQLite de Agenda
    migrations/
  <nova-app>/                    # exemplo futuro, domínio ainda indefinido
    pom.xml + mvnw + .mvn/       # se Spring/Maven for escolhido
    src/main/java/.../<modulo>/  # agrupado por capacidade
    src/test/
    contracts/v1/               # contratos publicados pela nova app MCP
packages/go/                    # somente bibliotecas técnicas justificadas
  mcpwire/go.mod
  voicewire/go.mod
  audio/go.mod                  # utilitários de áudio do cliente
  testkit/go.mod                # somente testes; clientes de processos reais
deploy/                         # composição de ambientes e plataforma
scripts/                        # build/dev agregadores por aplicação
tests/                          # integração e testes do tooling do repositório
docs/domains/
go.work                         # conveniência local para módulos Go
```

Os módulos internos podem começar com poucos arquivos; não exigir camadas
vazias nem uma interface por struct. Separar domínio/aplicação/adaptadores
quando existir dependência a inverter. `packages/go` não contém entidades,
políticas de negócio nem repositories. Java não depende dessas bibliotecas.

## Regras de dependência e dados

- Bootstrap conecta módulos; núcleo de negócio não importa adaptadores.
- `conversation` solicita execução pela API de `actions`; este recebe portas
  neutras, não entidades de Notes/Agenda. Não criar ciclo de imports.
- Cada app importa apenas sua implementação e bibliotecas técnicas aprovadas.
  Proibir imports de outra `apps/.../internal`, acesso SQL cruzado e import de
  modelos de outro contexto. Go `internal` ajuda, mas CI também deve verificar
  dependências públicas e ciclos; em Java, testes de arquitetura equivalentes.
- Notes e Agenda possuem bancos e migrations separados. Nenhuma transação
  atravessa os dois contextos. A transação local inclui efeito e ledger.
- Cada MCP define suas operações e valida suas regras de negócio e o contexto
  de autorização recebido. O gateway define quais operações pode solicitar e
  as condições de autorização na conversa.
- Perfis em `apps/voice-gateway/config/integrations/` pertencem ao gateway:
  aliases, ferramentas permitidas, políticas e frases de confirmação. São
  configuração confiável, revisada e validada no carregamento. Política
  ausente/inválida deve negar a chamada; metadados do modelo ou do MCP não
  concedem permissões. Segredos são referenciados por variáveis de ambiente,
  nunca incluídos nos perfis versionados.
- Cada aplicação MCP mantém uma fonte canônica de schemas, resultados, erros
  e exemplos em seu `contracts/v1/`. O gateway publica voz e envelope assinado
  em seus próprios `contracts/voice/v1/` e `contracts/host-context/v1/`.
  Não haverá pastas `contracts/` ou `integrations/` na raiz.
- Consumidores validam compatibilidade com versões publicadas pelos fornecedores,
  sem importar modelos internos. DTOs podem ser implementados/gerados em cada
  linguagem e traduzidos. Bibliotecas técnicas implementam os protocolos,
  mas não são donas dos contratos de negócio.
- Builds Go isolados devem passar com `GOWORK=off`. Bibliotecas comuns precisam
  de versões resolvíveis para distribuição fora do checkout. Nesta migração,
  módulos `stsmodel.local` usam `replace` apenas para bibliotecas técnicas
  irmãs, fixadas pelo mesmo commit do monorepo. Não há `replace` para outra
  app. Builds independentes precisam da app e de `packages/go`; publicar
  bibliotecas em um registry fica para quando houver consumo fora do repo.

## Fases, dependências e aceite

| Fase | Mudança concreta | Verificação e critério de aceite | Reversão |
| --- | --- | --- | --- |
| 0 — entregue | Harness local, análise e plano; links no README | Checksum, geração do contexto e revisão dos arquivos | Reverter só os novos artefatos e links |
| 1 — caracterizar contratos | Capturar contratos e exemplos junto de cada app fornecedora, voz/envelope junto do gateway; documentar política real de criação | Testes existentes + fixtures de Unicode, datas, confirmação, replay e resultado desconhecido; baseline sem AWS | Reverter fixtures/docs, nenhum dado alterado |
| 2 — separar persistência | Extrair Notes e Scheduling ainda no módulo Go atual; OpenAgenda deixa de chamar Open de Notes | Notes não cria tabelas de Agenda e vice-versa em banco novo; replay após reinício, atomicidade e conflitos preservados | Manter bancos compatíveis; voltar binários anteriores, sem remover tabelas |
| 3 — separar MCP e regras do host | Adaptadores por app; biblioteca técnica neutra; API de actions e perfis em `apps/voice-gateway/config/integrations/`, com segredos via ambiente | Biblioteca MCP não importa domínios; novo servidor fake cadastrado por configuração sem branch por nome no núcleo; perfis ausentes/inválidos e metadados remotos não autorizam chamadas; confirmação/envelope preservados | Perfis anteriores e adaptadores de compatibilidade temporários |
| 4 — mover aplicações | Criar módulos Go por app e bibliotecas; manter contratos/testes nos donos e atualizar caminhos de build/test | Build isolado por módulo, CLI e gateway; make/check agregado; testes do supervisor; imagem LiteLLM compila no layout novo | Reverter movimentação/configuração como unidade, preservar dados |
| 5 — desacoplar entrega | Catálogo declarativo de apps, comandos/cwd/env e empacotamento por app; validar transporte remoto antes de separar containers MCP | Gateway descobre app sem importar código; timeout, servidor indisponível e isolamento de falha cobertos | Manter composição stdio conhecida até validar alternativa |
| 6 — preparação para Spring | Documentar onboarding poliglota e fornecer contrato/vetores; implementação da primeira app adiada pelo usuário | Nenhuma dependência de modelos Go; futuro MCP deve passar seus contratos e vetores do envelope | Não há app Spring a reverter nesta entrega |

Ordem: 1 → 2 → 3 → 4 → 5 → 6. Fases 2–3 removem dependências antes de quebrar
imports fisicamente. Na fase 1, os diretórios de contratos das apps podem ser
criados antes de mover seus executáveis; isso não antecipa a migração do runtime.
Cada fase deve caber em PR revisável com comportamento
caracterizado; não combinar mudança de linguagem, transporte e banco no mesmo PR.

## Mapa de movimentação

| Origem atual | Destino/ação planejada |
| --- | --- |
| `cmd/gateway`, `internal/gateway`, `internal/session` | `apps/voice-gateway`: separar coordenação de domínio e handlers WS |
| `internal/orchestrator` | módulo actions + perfis Notes/Agenda em `apps/voice-gateway/config/integrations/`; validação de domínio no servidor |
| `internal/provider`, `internal/config` | portas/adaptadores e bootstrap do gateway; revisar consumidores antes de mover |
| `cmd/mcp-notes`, `storage/notes.go`, `mcp.Serve`, `NotesTools` | `apps/mcp-notes`, migrando testes junto |
| `cmd/mcp-agenda`, `storage/agenda.go`, `mcp/agenda.go` | `apps/mcp-agenda`, com conexão/migrations próprias |
| `internal/mcp` restante | transporte/envelope neutro em mcpwire; cliente/routing e políticas do host no gateway |
| `internal/protocol` | contrato de voz em `apps/voice-gateway/contracts/voice/v1/` + implementação técnica voicewire |
| `internal/audio`, `cmd/voice-client` | cliente CLI ou biblioteca audio se houver uso compartilhado |
| `apps/apple` | permanece; consome contrato de voz |
| `tests/e2e` | integração na raiz; testes unitários e de contrato seguem app/módulo; configuração/autorização ficam no gateway |

Atualizar simultaneamente `Makefile`, `scripts/dev.py`, exemplos de comandos,
`deploy/litellm/Dockerfile`, contexto/COPY e `.dockerignore`, fixtures e paths
em testes, documentação e scripts e2e. `go test ./...` na raiz deixa de cobrir
módulos aninhados: o agregador precisa enumerá-los explicitamente e falhar se
qualquer build/test falhar. Criar CI por app com seus testes de contrato e um
job de integração entre aplicações na raiz;
o inventário atual não contém workflow versionado em `.github/`.

## Persistência e rollout

Antes de mudar schema ou localização, registrar caminhos reais sem divulgar
conteúdo e fazer backup consistente (SQLite backup API ou writers parados,
incluindo cuidados com WAL). Preservar IDs, estados, timestamps e chaves de
idempotência. Os defaults são `data/sts.sqlite` e `data/agenda.sqlite`, mas
configuração pode apontar ambos para um mesmo arquivo: detectar isso antes
de migrar. Se compartilhado, separar com ferramenta de migração explícita e
classificar o ledger pelo nome da operação; não fazer cópia parcial silenciosa.

Na primeira extração, aceitar tabelas legadas extras sem apagá-las. Verificar
contagens e replay de operações numa cópia, parar writers, migrar e trocar
configuração. Rollback de binário só é seguro com schema compatível; depois
de novas escritas, não restaurar backup antigo automaticamente (perderia dados).
Criar migrações expansivas primeiro e adiar remoções até encerrar a janela de
compatibilidade. Ledger e efeito permanecem na mesma transação.

## Contrato para novos MCPs, incluindo Spring

A integração atual oferece stdio local e chamadas via gateway LiteLLM. Um
servidor Spring HTTP remoto não é automaticamente compatível com essa
composição: provar descoberta/chamada, autenticação e transporte na versão
usada de LiteLLM antes de adotar. Stdio Java é alternativa, mas exige runtime
Java no ambiente que lança o processo e não desacopla deploy sozinho.

A aplicação MCP mantém seus contratos e testes de conformidade. O gateway
mantém os perfis de consumo e publica a especificação e os vetores do envelope
em `apps/voice-gateway/contracts/host-context/v1/`; implementações Go/Java
validam-se contra esses vetores, sem duplicar a fonte canônica.

O contrato precisa cobrir:

1. Handshake/versão MCP suportada, tools/list e tools/call, limites e schemas.
2. Nome estável de tool, alias, resultados, erros e versão compatível. Mudanças
   incompatíveis exigem versão nova com janela de coexistência.
3. Contexto do host: idempotency key e confirmação em `_meta` no modo local,
   ou envelope assinado no fluxo LiteLLM. Modo `_meta` não autentica origem e
   não pode ser promovido a contrato remoto confiável sem proteção equivalente.
4. Interoperabilidade do envelope em `packages/go/mcpwire/envelope.go`: SHA-256,
   HMAC, ordem/serialização dos campos assinados, escaping JSON, representação
   numérica, Unicode, audience, tool, TTL e rejeição de campos extras. Não
   assumir que Jackson produz os mesmos bytes que Go; publicar vetores positivos
   e negativos e versão de canonicalização antes do primeiro cliente Java.
5. Confirmação vinculada ao alvo e turno posterior, ledger durável com replay,
   conflito de chave/argumentos, timeout sem repetição automática de mutação
   incerta e ausência de sucesso antes do resultado confirmado.
6. Configuração explícita de permissões no host e validação de negócio no app;
   segredo/configuração por ambiente; logs sem conteúdo de áudio ou payloads.

Na futura app Spring, agrupar código por capacidade e definir APIs públicas entre módulos.
Avaliar Spring Modulith/ArchUnit na fase de implementação, escolhendo versões
compatíveis nessa ocasião; a proposta não presume dependência já instalada.
Não criar agora uma app vazia de domínio fictício nem exigir Spring para todos.

## Validação por fase

Testes de contrato ficam na aplicação fornecedora; testes de configuração e
autorização ficam no gateway. Na raiz ficam os testes de integração entre aplicações e do tooling
do repositório. Testes internos do gateway também usam peers reais via testkit,
sem importar modelos/implementações de outras apps. Validar cadastro de um MCP fake por configuração sem alterar
o núcleo do gateway, rejeição de políticas ausentes/inválidas e impossibilidade
de metadados externos concederem permissão. Preservar as políticas atuais de
Notes/Agenda e as garantias de confirmação e idempotência.

Baseline/fases Go: `make check` (build, test, race, vet) e `make dev-test`.
Após mover módulos, manter esses nomes como agregadores e validar também cada
app com `GOWORK=off`. Se voz mudar, `make apple-core-test` e builds macOS/iOS.
Se Docker/build ou configuração mudar, reconstruir composição em ambiente de
teste e executar testes e2e opt-in com serviços locais. Smoke AWS tem custo e
não é requisito para esta entrega documental. Não declarar esses checks
executados apenas por constarem no plano.

## Hipóteses a validar durante implementação

Conversação é o diferencial principal; Agenda usa regras fictícias locais;
Notes é genérico. O primeiro domínio Spring ainda não foi definido. Transporte
remoto será validado antes de trocar a composição stdio; bibliotecas Go
são distribuídas junto do checkout nesta entrega. Essas incertezas não impedem separar regras e persistência
nas fases iniciais. Nenhuma delas exige criar microserviços adicionais agora.
