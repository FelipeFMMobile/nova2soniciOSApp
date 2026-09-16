# Etapa adicional: Notes + Agenda local antes de Apple

Implementação na branch `codex/multi-mcp-agenda`, a partir da main após Notes
v0.5.0. Esta etapa preserva Notes e não implementa macOS/iOS, contas externas,
Google Calendar, recursos/credenciais AWS ou IAM. Aceite e validação Nova real
continuam pendentes; merge, tag e push dependem do usuário.

Validação local em 16/09/2026: `make check` (build, todos os testes Go,
race detector e vet) e quatro testes Python da ponte passaram. Os dez cenários
com dois subprocessos MCP/SQLite e ponte mock passaram, com JSONL privado em
`artifacts/multi-mcp-local/` (ignorado pelo Git). Renovação, áudio e isolamento de
timeout também passaram nos testes Go. Nenhuma inferência AWS foi executada.

## Configuração explícita do host

```bash
make mcp-build agenda-build
export STS_PROVIDER=nova
export STS_MCP_TIMEOUT=10s
export STS_DEVELOPMENT_TOKEN=local-demo-token
unset STS_MCP_COMMAND STS_MCP_ARGS STS_MCP_ALLOWED_TOOLS
```

Copie `docs/mcp-servers.example.json` para um arquivo local, substituindo todos
os caminhos por caminhos absolutos dos binários e bancos nesta worktree. Exporte
seu conteúdo como `STS_MCP_SERVERS`, por exemplo:

```bash
export STS_MCP_SERVERS="$(cat /absolute/path/mcp-servers.local.json)"
export STS_MCP_EVIDENCE_PATH=/private/tmp/sts-multi-demo/private-tools.jsonl
```

Não há shell na execução MCP: `command` é absoluto e `args` é um array JSON.
Aliases são únicos, `[a-z][a-z0-9_]{0,23}`; cada ferramenta permitida tem uma
política explícita. Políticas fora da allowlist, ferramentas duplicadas e
combinações inseguras de políticas das tools conhecidas são rejeitadas.
Annotations do servidor não autorizam ferramentas. Os subprocessos são código
local confiável configurado pelo usuário, com ambiente/permissões do host.

| Ferramenta MCP | Política host | Nome anunciado (aliases do exemplo) |
| --- | --- | --- |
| notes.list | read_only | memo_notes_list |
| notes.create | explicit_intent (sem nova confirmação, compatível v0.5.0) | memo_notes_create |
| notes.delete | confirm_later | memo_notes_delete |
| agenda.list_slots | read_only | local_agenda_list_slots |
| agenda.list_events | read_only | local_agenda_list_events |
| agenda.create_event | explicit_intent | local_agenda_create_event |
| agenda.cancel_event | confirm_later | local_agenda_cancel_event |

Para compatibilidade, `STS_MCP_COMMAND/ARGS/ALLOWED_TOOLS` permanece disponível
quando `STS_MCP_SERVERS` não estiver definido. Os nomes Notes dessa configuração
continuam `notes_create/list/delete`, com as mesmas chaves de replay v0.5.0.
Definir simultaneamente as duas configurações produz erro.

O gateway inicia ambos os MCPs por sessão de voz, descobre a união de specs e
mantém o binding nome Nova → alias/nome MCP → subprocesso. Transformações que
colidem ou nomes fora de `[A-Za-z][A-Za-z0-9_]*`/64 bytes são rejeitados. O JSON
schema MCP é objeto; `inputSchema.json` no wire Bedrock é **string JSON
serializada**, inclusive na renovação. Nova escolhe tools usando `auto`; todas
as validações, chamadas e correlação de toolUseId ficam no Go. A ponte Python
permanece transporte. O terminal exibe nomes host como `memo.notes.list` e
`local.agenda.create_event`; use esses nomes em `-expect-tool`.

## Agenda fictícia e tempo

`cmd/mcp-agenda -db /absolute/path/agenda.sqlite` é stdio, não serviço TCP.
`-fixtures` semeia, de modo idempotente, `fixture-2030`, em **16/05/2030,
10–11h America/Sao_Paulo** (13–14h UTC). Fixtures são opt-in; não há eventos
criados automaticamente em bancos normais.

`start/end` usam RFC3339 com offset obrigatório. Por exemplo,
`2030-05-20T09:00:00-03:00` é armazenado como epoch UTC e retornado como
`2030-05-20T12:00:00Z`, com timezone `America/Sao_Paulo` no resultado.
Intervalos são semiabertos, início < fim, sem frações, no máximo 31 dias.
Agenda oferece dias úteis, 09–18h locais; eventos duram no máximo oito horas.
Slots aceitam duração 15–240 minutos em múltiplos de 15 e grid de 15 minutos
partindo do início solicitado. Até 100 slots/eventos por consulta; não há
paginação. Cancelados são retornados com status próprio e liberam o horário.

Criação verifica disponibilidade e conflito na mesma transação que grava evento
e resultado de replay. Não aceita sobreposição com eventos ativos; eventos
adjacentes podem coexistir. Cancelamento exige evento ativo existente.

O filtro de intenção é conservador e específico desta POC PT-BR: o ASR final
de USER deve começar com `agende`, `marque`, `crie um evento/agendamento`,
`quero agendar/marcar` (opcional `por favor`, ou prefixo `consulte/busque/leia … e`), conter data completa numérica
`AAAA-MM-DD` ou `DD/MM/AAAA` e horário numérico `09:00`/`9h`. Negação,
condicional, incerteza, citações e termos relativos como amanhã/depois bloqueiam
criação com `clarification_required`. O modelo deve esclarecer informações
ambíguas; o Go não transforma linguagem livre em uma data presumida. Isso
**não é um parser geral de intenção**: datas por extenso, pedidos compostos
fora dessa gramática e variantes do ASR podem exigir reformulação. O teste Nova
real precisa avaliar essa limitação antes do aceite. O schema exige título e
intervalo; validação semântica final de horários/disponibilidade fica no Go.

Para cancelar, o Go congela alias, ferramenta, argumentos/alvo, turno e validade
60s na confirmação pendente. Somente ASR final USER em turno posterior contendo
exatamente **“Confirmo cancelar agendamento”** aprova Agenda. `sim`, argumentos
`confirmed`, saída do modelo, citações, troca de alvo e booleano do cliente não
aprovam. Recusa limpa a pendência. Notes conserva “Confirmo excluir” e sua
confirmação de terminal aceita em v0.5.0. Nenhuma pendência é sucesso.

## Retry, falhas e limites

Uma mutação de cada alias/tool por `requestId`: chaves incluem requestId e nome
host completo, evitando colisões Notes/Agenda. Outra ação deliberada da mesma
tool precisa de novo requestId. Argumentos reformulados no mesmo requestId são
conflito; sucesso é replay do resultado durável, não segundo efeito. Não use
novo requestId para retry após resultado desconhecido. Mudança de alias muda o
namespace: mantenha aliases estáveis para retry. Os ledgers são específicos dos
servidores Go desta POC; MCPs genéricos podem ignorar os metadados host.

Não há restart nem reexecução automática de mutações. Durante a sessão,
chamadas concorrentes/repetidas da mesma mutação ficam bloqueadas; resultado de
erro também é preservado para não reexecutar. Reconnect explícito com mesmo
requestId pode consultar o ledger durável. Em queda/timeout, resultado é erro
controlado, mesmo se o efeito já tiver ocorrido: SQLite é a autoridade do replay.
Timeout invalida somente o cliente MCP afetado. Tools do outro servidor seguem
operantes. Falha no startup de um MCP anuncia placeholders indisponíveis para
suas tools (schemas conhecidos Notes/Agenda preservados); nenhuma chamada a um
placeholder executa subprocesso ou anuncia sucesso. Não há redescoberta dinâmica.

Até oito servidores, 32 tools na união, 128 chamadas por sessão, quatro jobs
ativos, 16 KiB argumentos, 256 KiB mensagem MCP e 64 KiB resultado de texto.
Protocolo stdio MCP **2025-11-25**: initialize/initialized, tools/list/call e
ping, sem HTTP/resources/sampling/paginação. Deadline de inicialização e chamada
é limitado; teardown cancela processos, fecha pipes, aguarda leitores/jobs e
recolhe subprocessos. Audio de entrada continua pelo gateway durante tools.
Cancelar a resposta não desfaz efeito já commitado; interrupções não repetem
mutações. O mesmo runtime/bindings/specs permanece durante renovação Nova;
rotação com jobs ativos respeita o fluxo de proteção preexistente.

## Teste local e evidência privada (sem AWS)

```bash
make check
PYTHONPATH=services/nova-bridge services/nova-bridge/.venv/bin/python \
  -m unittest discover -s tests/python -v
mkdir -p artifacts/multi-mcp-local
chmod 700 artifacts/multi-mcp-local
STS_TEST_EVIDENCE_DIR="$PWD/artifacts/multi-mcp-local" \
  GOCACHE=/private/tmp/sts-go-cache go test ./internal/gateway \
  -run TestTwoRealMCPsMockNovaScenarios -count=1 -v
```

Os helpers são subprocessos reais do binário Go de testes, executando exatamente
os servidores MCP Notes/Agenda com bancos SQLite privados. A ponte Nova é mock;
os scripts escolhem tools explicitamente. Testes cobrem notas/agenda isoladas,
dados da nota usados na criação, consulta Agenda registrada em Notes, conversa
sem toolUse, ambiguidades, aprovação/recusa/troca de alvo, schema inválido,
retry em sessão e após reconnect, conflitos/UTC/fixtures, colisão de nomes,
falta de processo/timeout com servidor saudável, áudio durante Agenda lenta,
renovação com as sete specs e regressões Notes.

`STS_MCP_EVIDENCE_PATH` é opt-in, absoluto, arquivo 0600; captura sessão/ID,
servidor, tool, argumentos/resultados reais e rejeitados. Bancos e JSONL/WAV
ficam fora do Git. Logs normais guardam metadados/status, sem payloads, áudio,
transcrições ou credenciais. O terminal/event capture intencionalmente contém
conteúdo da demo; use só fixtures e diretórios privados. O audit SQLite normal
continua sem conteúdo. Evidência local não comprova seleção do modelo.

## Roteiro Nova real PT-BR pelo terminal — somente após autorização

Não execute esta seção antes de autorização específica na task. Use a ponte e
perfil AWS já existentes; não crie credenciais/recursos e não altere IAM.
Após autorização: terminal 1 `make nova-bridge`, terminal 2 `make gateway` com a
configuração acima; terminal 3, WAV mono PCM16 16kHz até 30s:

```bash
GOCACHE=/private/tmp/sts-go-cache go run ./cmd/voice-client -provider nova \
  -wav /absolute/path/agende.wav -request-id agenda-demo-2030-1 \
  -expect-tool local.agenda.create_event \
  -events /private/tmp/sts-multi-demo/create-events.jsonl \
  -output /private/tmp/sts-multi-demo/create-response.wav
```

Fala sugerida: “Agende Consulta Aurora em vinte de maio de dois mil e trinta,
às nove horas, por uma hora.” **Verifique o ASR**: se o modelo/ASR produzir data
por extenso, o filtro conservador pode exigir reformulação numérica. Não trate
rejeição como sucesso nem force tool choice para esconder falha de seleção.

1. Só Notes: criar/consultar valor fictício único em conversa nova; somente
   `memo.notes.*` deve executar. Revalidar confirmação e replay v0.5.0.
2. Só Agenda: consultar slots, pedir criação com data/título/duração explícitos,
   consultar estado em sessão nova; conflito deve falhar sem segundo efeito.
3. Nota → Agenda: armazenar horário ISO e título na nota; pedir leitura e criação
   usando esses dados, dentro da gramática de intenção; correlacionar os dados
   da nota com argumentos Agenda e SQLite. Registrar limitações de fala composta.
4. Agenda → Notes: consultar e registrar resultado; conferir conteúdo persistido.
5. “Talvez marque algo amanhã”: esclarecimento, sem criação; conversa comum:
   nenhum toolUse. Conferir chamadas reais, não apenas texto falado.
6. Cancelar por ID consultado: primeira resposta pede confirmação, sem efeito;
   `-followup-wav /absolute/path/confirm.wav` com “Confirmo cancelar agendamento”
   deve cancelar. Repetir com “Não confirmo”: permanece ativo. Conferir alvo/turnos.
7. Repetir criação com mesmo requestId e WAV: mesmo ID/resultado, nenhum duplicado.
8. Derrubar/atrasar Agenda: erro de Agenda, Notes ainda disponível, nenhum anúncio
   de sucesso falso. Testar áudio contínuo e renovar a sessão com sete specs.

Critérios de aceite: todos os gates locais passam; Nova real seleciona tools
adequadas com `auto`, usa dados efetivos e esclarece ambiguidades; correlação
ASR final → servidor/tool/args → resultado/SQLite → resposta falada consistente;
cancelamento aprovado/recusado e retry sem duplicação; servidor saudável e
áudio continuam sob falha; limitações ASR/gramática são avaliadas e aceitas.
Mocks **não comprovam** seleção, linguagem/ASR, resposta falada ou latência do
Nova real. Um resultado `created/cancelled` deve preceder qualquer anúncio de
conclusão; erros e outcome desconhecido nunca satisfazem o gate de sucesso.

Custo local de inferência: zero. Roteiro AWS proposto: 10–12 sessões curtas
mais um soak de renovação, com novas tentativas somente dentro do orçamento
que o usuário autorizar. Bedrock cobra áudio de entrada/saída e tokens de texto
(incluindo schemas, tool calls/results e histórico reanunciado). Erros,
repetições e renovação podem gerar cobrança; não há garantia de teste gratuito.
Custo = áudio_in × tarifa_in + áudio_out × tarifa_out + texto_in × tarifa_in +
texto_out × tarifa_out, usando unidades/tarifas da região/modelo contratados.
Confirmar tabela vigente antes do teste e registrar usage/custo observado; não
há estimativa fixa confiável a partir dos mocks. Notes/Agenda/SQLite são locais
e não acrescentam serviço de agenda pago. Sugestão para aprovação futura:
primeiro lote curto com teto operacional US$5, interrompendo e reportando se a
estimativa/uso ultrapassar o teto; o teto não é um limite automático AWS.

Referências consultadas em 16/09/2026:
[AWS tool configuration](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-tool-configuration.html),
[AWS Nova pricing](https://aws.amazon.com/nova/pricing/),
[Bedrock pricing](https://aws.amazon.com/bedrock/pricing/).
