# Nova Voice — SwiftUI para Mac e iPhone

Implementação compartilhada em `apps/apple/NovaVoice.xcodeproj`, scheme
`NovaVoice`, branch `stage/04-macos-app`. SDK Xcode 27; deployment macOS 26
para executar no Mac atual e iOS 27 conforme o plano. A base iPhone está
incluída nesta etapa; validação física, assinatura e revisão de rotas de áudio
continuam na etapa 5, depois do aceite/merge desta branch.

## Executar primeiro sem AWS

Na raiz do repositório:

```bash
STS_PROVIDER=fake STS_DEVELOPMENT_TOKEN=local-demo-token make gateway
open apps/apple/NovaVoice.xcodeproj
```

No Xcode escolha `NovaVoice` → `My Mac` ou um iPhone Simulator iOS 27 e Run.
No app abra **Conexão local**, informe `ws://127.0.0.1:8080/v1/voice`, token
`local-demo-token` e provider **Fake · teste local**. Toque **Iniciar conversa**:
o gateway responde com transcrições explicitamente simuladas e um tom, não voz.
Esse modo não solicita microfone, não transcreve fala e não conecta à AWS.
Encerre no botão e pare o gateway com Ctrl+C.

O provider escolhido no app deve ser o mesmo selecionado no gateway; a POC
não troca o provider do backend dinamicamente.

## Nova + Notes + Agenda

1. Configure o perfil AWS e a ponte conforme o README. Somente a ponte recebe
   credenciais AWS. No terminal 1 execute `AWS_PROFILE=SEU_PERFIL make nova-bridge`.
2. No terminal 2 configure os dois MCPs conforme
   [Notes + Agenda](multi-mcp-agenda.md#configuração-explícita-do-host), incluindo
   `STS_PROVIDER=nova`, `STS_MCP_SERVERS` com caminhos absolutos válidos e
   `STS_DEVELOPMENT_TOKEN`. Execute `make gateway` nesse terminal.
3. No app use a URL/token desse gateway e provider **Nova 2 Sonic**. Inicie,
   aceite a permissão de microfone e fale em PT-BR. A sessão AWS é aberta apenas
   depois da permissão; há cobranças normais do Bedrock ao iniciar Nova.
4. Peça uma consulta de notas/agenda ou uma ação com dados explícitos. O app
   mostra ferramentas em execução, retorno real e confirmação pendente; não
   considera `tool.started` um sucesso. Para remoção diga a frase exata
   solicitada, por exemplo **Confirmo excluir** ou **Confirmo cancelar agendamento**.
5. Fale durante a resposta para testar barge-in. Encerre a conversa e pare
   gateway/ponte com Ctrl+C.

O cliente captura continuamente usando AVAudioEngine com processamento de voz,
converte para PCM16 mono 16 kHz e envia quadros de 32 ms. Nova decide o fim da
fala; não há push-to-talk por turno. O botão inicia/encerra a conversa inteira.
Saída PCM16 mono 24 kHz é reproduzida incrementalmente. O grafo de saída do
Voice Processor usa exatamente o formato
de entrada do microfone, não os 24 kHz do transporte. O mixer converte o áudio do
player para esse formato. A conexão mixer → saída é fixada antes de ligar o
player e refeita a cada início para não herdar formatos de outra sessão. No modo
fake, o processamento de voz é desabilitado e a saída usa o formato do dispositivo.
Até ~256 ms são
agendados no player; o buffer adicional é limitado a 30 s porque Nova pode
gerar mais rápido que a reprodução. Interrupção limpa ambas as filas; áudio
cancelado ou de outro turno é descartado. Ordem inválida/overflow encerram a
sessão explicitamente, sem perder áudio silenciosamente.

Captura, rede e reprodução param ao encerrar/fechar a tela. No iOS, background,
interrupção de áudio e desconexão do dispositivo de saída encerram a conversa;
reinicie manualmente. Não há reconexão automática nem replay de ações.

### Idempotência e resultados desconhecidos

Cada início explícito cria um novo `requestId`; há uma mutação de cada
alias/tool por pedido, limite atual do backend. Outra ação deliberada da mesma
tool requer uma nova conversa. Se a rede cair durante uma mutação, **não repita
a ação iniciando outra conversa**: um novo requestId pode duplicar o efeito.
Consulte Notes/Agenda primeiro; para retry durável use o cliente de terminal
com o requestId original (copiável em **Identificador do pedido**) conforme o runbook MCP. Recuperação explícita pelo
app será tratada na etapa de resiliência. Interromper a fala não desfaz uma
operação já gravada pelo MCP.

## Configuração Xcode

URL/provider/token também podem vir de `Config/Debug.local.xcconfig`, incluído
opcionalmente e ignorado pelo Git. Use `Debug.example.xcconfig` como referência.
A sintaxe `ws:/$()/127.0.0.1:8080/v1/voice` evita que `//` seja comentário no
xcconfig. Prefira digitar o token no SecureField em runtime: ele não é salvo.
Um token configurado por xcconfig fica no Info.plist do bundle; não distribua
esse build. **Nunca coloque Access Key, Secret Key, perfil ou credenciais AWS
em xcconfig/app.** Release não inclui o arquivo local Debug.

Áudio/transcrições ficam na memória, limitados, sem gravação em disco nem
logs de payloads. Enviar voz ao backend/AWS ocorre somente na conversa Nova.

## iPhone físico — etapa 5

O iPhone detectado localmente está no iOS 26.6 e indisponível para desenvolvimento.
O target atual exige iOS 27; é necessário aprovar reduzir o deployment mínimo
ou usar um dispositivo iOS 27. Xcode 27/SDK 27 não obriga deployment 27.
Depois, selecione sua Team em Signing & Capabilities, conecte/desbloqueie o
iPhone e habilite Developer Mode. Nenhuma conta/team é configurada pelo projeto.

Para rede local, no terminal do gateway:

```bash
export STS_GATEWAY_ADDRESS=0.0.0.0:8080
export STS_DEVELOPMENT_TOKEN=UM_TOKEN_LOCAL_FORTE
make gateway
```

Mantenha as variáveis Nova/MCP do terminal anterior. No app use
`ws://IP-DO-MAC:8080/v1/voice` e o mesmo token. `127.0.0.1` no iPhone aponta ao
próprio iPhone, não ao Mac. Autorize rede local/microfone no dispositivo e
conexões de entrada do gateway no firewall se solicitado. A ponte permanece
em loopback, não exponha sua porta. Não use forwarding no roteador.
`ws://` não criptografa voz/token: use só uma rede privada confiável para a POC;
fora dela é obrigatório configurar TLS e `wss://`. O app permite networking
local via ATS, sem liberar HTTP irrestrito globalmente.

## Testes e gates

```bash
make apple-core-test
make apple-build-macos
make apple-build-ios
make check
```

Para integração Swift → gateway Go e UI, em outro terminal:

```bash
STS_PROVIDER=fake STS_GATEWAY_ADDRESS=127.0.0.1:18080 \
  STS_DEVELOPMENT_TOKEN=apple-test-token make gateway
```

```bash
STS_APPLE_E2E_URL=ws://127.0.0.1:18080/v1/voice make apple-core-test
make apple-ui-test APPLE_SIMULATOR_ID=599499E4-DD38-4C6C-9459-D4FDA8B2AE43
```

O teste de UI usa `--ui-testing` somente em Debug e força fake, sem captura nem
reprodução de hardware. Testes automatizados não comprovam qualidade do áudio,
ASR, cancelamento de eco nem seleção de ferramentas pelo modelo. Aceite da etapa
4 exige teste manual Nova com microfone real no Mac; etapa 5 exige iPhone físico.
Não executar chamadas AWS automaticamente para validar builds.

### Resultado local — 16/09/2026

- Builds Debug macOS arm64 e iOS Simulator SDK 27: passaram.
- 11 testes Swift: passaram (protocolo, interrupção/ordem, ferramentas,
  configuração, conversão sintética e duas sessões Swift → gateway Go fake).
- XCUITest iPhone 18 Pro/iOS 27: iniciar, receber transcrições fake e encerrar
  passaram; comando final retornou zero em aproximadamente 25 s de execução.
- Go build/test/race/vet e quatro testes Python da ponte: passaram.
- Não validado aqui: fala real, eco, qualidade de reprodução no hardware,
  latência/barge-in real, seleção de MCP por Nova no app e iPhone físico.

O primeiro teste de UI passou, mas travou na finalização de diagnósticos após
um aviso de AVAudioSession. Foi encerrado somente o xcodebuild dessa execução;
o modo hardware-free foi corrigido para não desativar uma sessão não ativada,
e a repetição passou e finalizou normalmente. Ativação de áudio real no iOS
ainda usa a API síncrona: migrar para ativação assíncrona e medir responsividade
faz parte da revisão de áudio/dispositivo na etapa 5.
## Diagnóstico de captura e WebSocket

No modo Nova, a tela mostra nível do PCM convertido, buffers/bytes capturados e
frames enviados pelo WebSocket. O medidor usa RMS (com ganho visual de 10x), não
é reconhecimento de fala. O contador de enviados avança após `socket.send`
concluir: isso não comprova processamento pelo gateway ou pela AWS.

Após 5 s sem PCM convertido, aparece um aviso de captura parada. Após 10 s sem
nível acima de 0,001, aparece um aviso de sinal baixo. Esses avisos não encerram
a sessão. Os contadores reiniciam a cada conversa.

Execute pelo Xcode e abra **View → Debug Area → Activate Console**. Filtre por
`PCM`, `SEND`, `RECV` ou `WebSocket`. Os logs usam o subsistema
`com.sts.NovaVoice`, categorias `Audio` e `Gateway`:

- `Capture format`: formato de entrada e processamento de voz.
- `PCM buffers=... bytes=... sentFrames=... rms=... stalled=...`: resumo por segundo.
- `SEND` e `RECV`: tipo, sequência e tamanho Base64, sem payload. Áudio é
  registrado no primeiro frame e a cada 50 frames para não sobrecarregar o console.
- Falhas de rede: domínio e código do erro, sem descrição ou credenciais.

Não são registrados token, PCM/Base64, transcrições, argumentos/resultados de
tools nem mensagens de erro do servidor. Nenhum áudio é gravado em disco.
Se houver problema, compartilhe o trecho desses resumos enquanto fala por
10–15 s. Buffers zerados indicam ausência de PCM no consumidor; RMS próximo de
zero com buffers aumentando indica sinal silencioso/baixo após conversão;
frames enviados aumentando permite investigar o gateway/bridge em seguida.
