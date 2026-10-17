# Conta: implementação e validação

Retomada em 2026-10-06 da sessão “Adicionar agente de login”. Base: `4706b73`.
A entrega implementa o agente Conta e mantém Agenda/notas independente de login.
A validação Cognito/SES real permanece pendente; o modo fake não comprova MFA.

## Contratos e limites

- `apps/account-agent`: conversação, prompt, políticas de confirmação e A2UI.
- `apps/mcp-identity`: transações OAuth/PKCE, vínculo Cognito, `CLIENTE_CODE`,
  credenciais opacas, renovação rotativa e revogação; PostgreSQL exclusivo.
- `apps/mcp-profile`: nome, idade, cidade, gênero, profissão, versão e ledger;
  PostgreSQL exclusivo; verifica a sessão por API de identidade antes do acesso.
- `apps/voice-gateway`: catálogo confiável, transporte v1/v2 e canal de controle
  `/v2/auth/*`; recebe credenciais do app, nunca do modelo.
- `packages/go/realtime`, `actionruntime`, `agentwire`, `mcpwire`, `voicewire`:
  bibliotecas técnicas. Não são domínios nem donos de bancos.
- Apple: seleção de agente, catálogo A2UI limitado, navegador do sistema,
  Keychain por origem e instalação, suspensão de captura e nova conversa após login.

Identidade e perfil publicam contratos MCP v2 em suas próprias pastas `contracts`.
Notes/Agenda continuam no envelope v1. Nenhum campo de identificação é argumento
editável do perfil; o contexto assinado fornece a identidade validada.

## Desenvolvimento local

Ao reiniciar, o supervisor encerra por SIGTERM os serviços reconhecidos que
ocupam as portas solicitadas: gateway (8080), Agenda/notas (8090) e Conta (8091).
Também reconhece o antigo `python -m nova_bridge.server` em checkouts do mesmo
repositório Git. Confere usuário, executável/comando e início do processo antes
de sinalizar, aguarda a liberação das portas e preserva bancos. Processos de
outros projetos bloqueiam a inicialização e são identificados pelo PID.

Requisitos: Go, Python 3, Docker Compose; Java 21 para builds Maven locais;
Xcode para builds Apple. Nenhuma alteração no Java global é necessária.

```sh
make dev
# opções 3 (fake) ou 4 (Nova/Cognito)
# ou:
python3 scripts/dev.py --account
python3 scripts/dev.py --account --dry-run
```

O supervisor configura `deploy/account/.env` com permissão 0600, inicia bancos
separados e cria fixtures Alice, Bruno e Carla no fake. Cognito usa de um a três
e-mails, faz preflight de MFA obrigatório por e-mail, pool Essentials/Plus, SES,
callback e app client público. Criação usa convite e nomes de fixture com namespace;
contas existentes sem registro local não são adotadas nem sobrescritas.
Não alternar fake/Cognito sobre os mesmos bancos: o supervisor recusa essa troca.

O app recebe o endereço e token local exibidos pelo supervisor. Selecionar Conta,
iniciar a conversa e usar Autenticar. No fake DEBUG/loopback a etapa de navegador
é simulada; no Cognito senha e código ficam no Managed Login. iPhone físico exige
certificado HTTPS/WSS confiável. O fake simula login; edição por voz exige Nova.

```sh
make test              # inclui builds/verificação Java; JAVA_HOME deve apontar a Java 21
make dev-test
make java-test
make apple-core-test apple-build-macos apple-build-ios
make vet race isolated
```

Os testes Go `TestLiveGuardrails` e `TestLiveIdentityProfile` são opt-in:
`STS_ACCOUNT_INTEGRATION=1`, URL/chave LiteLLM e segredo MCP v2 no ambiente;
`STS_TEST_IDENTITY_URL`, `STS_TEST_IDENTITY_TOKEN`, `STS_TEST_PROFILE_URL`,
`STS_TEST_PROFILE_TOKEN` apontam a um stack fake isolado. Nunca usar contas reais.
A suíte verifica bloqueios de entrada/saída, login, confirmação, versão, replay,
isolamento e revogação. Segredos não devem aparecer em comandos ou relatórios.

## Critérios restantes de aceite

- Executar login Cognito real com senha e MFA por e-mail em conta fixture;
  evidenciar segundo fator, cancelamento e indisponibilidade de SES/Cognito.
- Exercitar no dispositivo o retorno do navegador após suspensão, reinício,
  renovação, logout e captura de áudio interrompida durante autenticação.
- Validar interação por voz Nova com perfil, confirmação e interrupção.

Builds e testes fake não substituem esses ensaios. Nenhum recurso AWS foi
provisionado nesta retomada. Configurar os identificadores pelo supervisor local.

## Rollback

Iniciar `make dev` nas opções anteriores (1/2), sem Conta. Não remover volumes.
O catálogo deixa de publicar Conta; Voice v1 e Notes/Agenda permanecem disponíveis.
Preservar separadamente os bancos de identidade/perfil para uma retomada posterior.

## Smoke local reproduzível

```sh
make account-smoke
make account-ui-test  # inclui os testes no simulador iOS
# Para outro simulador:
make account-ui-test APPLE_SIMULATOR_ID=ID-DO-SIMULADOR
```

`scripts/account_smoke.py` cria um projeto Docker com nome aleatório, portas livres,
bancos e segredos sintéticos exclusivos. Executa os testes reais Go → LiteLLM →
Spring e AgentSession v2. Com `--apple-ui`, inicia os dois agentes e o gateway e
executa XCTest: login fake, reinício/restauração pelo Keychain, logout e troca de
agente. Os testes usam o app compilado, não uma tela simulada.

O smoke também exercita o handler local de confirmação por botão e a política de
voz com transcrições simuladas, incluindo cancelamento, expiração e troca de
argumentos. Executa regressão MCP v1 de Notes/Agenda com chave própria. Após as
chamadas, procura canários de perfil/nota e segredos configurados nos logs dos
containers, logs locais e linhas de spend, sem imprimir os conteúdos auditados.
Informa a quantidade de linhas de spend; zero linhas não comprova privacidade
de Realtime. ASR Nova e edição completa de perfil no app ainda exigem ensaio real.

O runner remove somente seus containers e volumes ao terminar e apaga o arquivo
.env temporário. Mantém logs e resultados XCTest em
`/private/tmp/sts-account-smoke/run-*`, com diretório privado. Não herda credenciais
AWS nem configurações STS/Nova existentes. O stack de desenvolvimento é preservado.
O ensaio de Conta em XCTest fica opt-in; `make apple-ui-test` sem esse stack
continua executando o teste anterior de Agenda/notas.

A etapa fake DEBUG não abre o navegador do Cognito e não comprova MFA nem o retorno
real de ASWebAuthenticationSession após suspensão; esses critérios continuam pendentes.


## Correção do retorno Cognito no macOS — 2026-10-17

Após login/MFA, o usuário observou EXC_BREAKPOINT em
`_dispatch_assert_queue_fail`. O callback ASWebAuthenticationSession era criado
no contexto MainActor de AccountSession, apesar de poder ser invocado em outra
fila. Esse padrão é compatível com o diagnóstico de isolamento em runtime do
Swift 6; o screenshot não contém o backtrace completo para confirmar todos os
frames.

Extraído WebAuthenticationCallback com função Sendable que apenas conclui a
continuation. A tarefa aguardando o resultado retoma no MainActor para atualizar
Keychain/UI e completar o login. Testes simulam sucesso e cancelamento em fila de
fundo, verificando o retorno ao MainActor. VoiceCore: 30 testes aprovados e um
opt-in ignorado; build macOS aprovado. Ainda requer repetir o fluxo real de
login/MFA no app recompilado para confirmar a resolução observada pelo usuário.

Referência sobre callbacks Objective-C e isolamento inferido:
[Swift SE-0463](https://github.com/swiftlang/swift-evolution/blob/main/proposals/0463-sendable-completion-handlers.md).


## Experiência de entrada no modal — 2026-10-17

A sessão anônima começa com “Sou agente STS para logar sua conta, deseja
continuar?”. O agente solicita essa fala à Nova Sonic pelo LiteLLM; áudio e transcrição
da apresentação vêm do provedor, pelo mesmo canal usado na conversa.
Sessões autenticadas recebem a confirmação falada de conta conectada antes de
aguardar solicitações de consulta/edição de perfil.

O supervisor aguarda consentimento por transcrição finalizada ou por confirmação
no app. Uma recusa encerra a sessão sem chamar identity.begin_login. Respostas
ambíguas mantêm o convite. O modelo não cria transações por conta própria nessa
etapa. O fake usa o mesmo convite, respondido por botões, sem simular ASR real.
A confirmação usa tool.confirmation/tool.confirm com operação reservada
account-login-consent, vinculada à sessão pendente, sem novo tipo de evento.

Após consentimento, a A2UI validada abre a sheet “Entre com seus dados” e
interrompe a conversa/microfone. Autenticar prepara a transação e mostra uma
WKWebView com WKWebsiteDataStore.nonPersistent no mesmo modal. O delegate aceita
somente HTTPS na origem preparada pelo backend e intercepta o callback exato
stsvoice://oauth/callback; o backend continua validando state, PKCE, nonce e MFA.
Não há bridge JavaScript nem leitura de campos de senha/MFA, logs de URLs ou
credenciais, ou abertura de navegador externo. A sessão web temporária é
eliminada ao fechar. Este fluxo usa somente o IdP COGNITO já exigido no preflight.

Cancelar fecha a WebView, resolve a continuação pendente e invalida a tentativa;
uma resposta HTTP tardia não autentica o app após cancelamento. Falhas mantêm o
modal para tentar novamente. Sucesso fecha o modal e conecta uma nova sessão
com o token do usuário. O antigo fluxo ASWebAuthenticationSession foi substituído.

O guia da AWS descreve as páginas interativas de senha e MFA do
[Managed Login](https://docs.aws.amazon.com/cognito/latest/developerguide/managed-login-endpoints.html).
A restrição das navegações utiliza o
[delegate WebKit](https://developer.apple.com/documentation/WebKit/WKNavigationDelegate).
O login real na WebView ainda requer ensaio com o usuário; os testes locais
não demonstram envio de MFA nem a compatibilidade de toda página Cognito.


Validação desta alteração: builds macOS/iOS Simulator aprovados; VoiceCore com
31 testes aprovados e um opt-in ignorado; Go account-agent com race aprovado,
incluindo transcrições sim/não em Realtime simulado. `make account-ui-test`
aprovado no projeto isolado sts-account-smoke-9354a83e, com dois XCTest sem
falhas, chamadas Go → LiteLLM → Spring e regressão Agenda/notas aprovadas.
O teste de interface cobre recusa sem autenticação, novo início, aceite, modal,
login fake, restart/Keychain, logout e troca de agentes. Auditoria local aprovada;
81 registros de spend do teste foram inspecionados. Isso não representa auditoria
de conteúdo de inferência Realtime real. Evidências em
`/private/tmp/sts-account-smoke/run-nsl47slf`; recursos isolados removidos pelo runner.


## Correção: apresentação pela Nova Sonic — 2026-10-17

A primeira implementação usava AVSpeechSynthesizer, produzindo uma voz diferente.
Essa síntese local foi removida do app compartilhado. O agente Conta envia texto
interativo e response.create ao LiteLLM somente na abertura anônima. A voz é a
mesma carolina já configurada no canal Nova. Renovações técnicas não enviam outro
pedido de saudação; sessões autenticadas recebem contexto explícito para não
repetir o convite.

O mapper aceita a fala inicial sem exigir uma fala anterior do usuário. O
primeiro registro da captura não a cancela. O bootstrap response.create da imagem
LiteLLM fixada usa internamente uma fala ready; seus eventos de entrada anteriores
à resposta inicial são excluídos da transcrição e da decisão de consentimento.
Fala e interrupção do usuário seguem pelo fluxo normal após o início da resposta.
Texto e áudio da saudação publicada são exclusivamente os retornados pela Nova.
O fake continua explicitamente simulado e usa o convite por texto/botões.

Validação real: TestLiveNovaGreeting recebeu áudio e transcrição sem microfone;
TestLiveNovaOpeningWithSilentMicrophone recebeu 45 quadros Nova no runtime Conta,
com captura silenciosa concorrente, zero login sem consentimento e nenhum
bootstrap apresentado como fala do usuário. Cada ensaio usou chave temporária
revogada no finally, sem áudio gravado ou dados pessoais. Testes locais cobrem
áudio antes de fala do usuário, VAD/barge-in, descarte de áudio obsoleto e sim/não.
Builds macOS e iOS Simulator aprovados. Estes ensaios validam a saudação; o MFA
real na WebView continua uma homologação separada.

Texto interativo é descrito nos
[eventos de entrada Nova](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-input-events.html).
A necessidade de response.create foi confirmada no adaptador LiteLLM instalado
na imagem fixada e no ensaio real: somente texto não produziu áudio nesse runtime.


## Confirmação após login ou restauração — 2026-10-17

Ao abrir a sessão com Principal.Authenticated confirmado pelo gateway, o agente
solicita à Nova Sonic esta frase: “Conta conectada com sucesso, agora você pode
consultar e editar informações da sua conta”. O retorno bem-sucedido do login e
a restauração pelo refresh salvo no Keychain chegam por esse mesmo caminho.
A confirmação é emitida na nova sessão autenticada, sem consultar ou editar o
perfil automaticamente. Renovações técnicas internas não repetem a abertura.

O áudio e a transcrição vêm da Nova Sonic; não há síntese local no Apple client.
O fake emite a mesma frase como transcrição para testar a experiência. O contexto
anônimo mantém a saudação e o consentimento prévios; falhas de autenticação não
produzem confirmação de sucesso.

Go com race aprovado, incluindo sessões anônimas e autenticadas. Ensaio real
TestLiveNovaConnectedWithSilentMicrophone aprovado com 72 quadros Nova, nenhum
login adicional ou transcrição do bootstrap técnico. A chave temporária do
ensaio foi revogada ao final. XCTest foi ampliado para verificar a frase após
login fake e após reiniciar o app/restaurar a sessão.

Validação integrada concluída: `make account-ui-test` aprovado, incluindo as
asserções da frase após login e após restart/Keychain, regressões MCP reais locais
e auditoria do ambiente fake. Evidência: `/private/tmp/sts-account-smoke/run-3ju4mb_f`.
