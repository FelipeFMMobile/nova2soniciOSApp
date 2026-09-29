# Plano revisado — gateway de voz e aplicações agênticas

Data do plano: 2026-09-28. As seções de inventário abaixo descrevem a base
`v0.7.0`; andamento atual em [implementação](agent-migration-implementation.md).

Base versionada: tag `v0.7.0`, commit `bd593ce4a73551da054bd98efec2bfdec63eb795`.
Branch de trabalho: `codex/agent-migration-v1`.

Este documento reconcilia `/private/tmp/sts-agent-migration-plan.md` com o
estado observado do repositório. Ele não declara diretórios futuros como
existentes nem autoriza remover o caminho legado antes dos gates descritos.

## Conclusão da análise

A direção é válida: `voice-gateway` deve se tornar um BFF de transporte e
roteamento, enquanto `personal-assistant-agent` passa a ser dono da conversa
semântica, prompt, modelo, ferramentas, confirmação e integrações MCP.

A separação não cria um novo domínio de negócio. Conversação assistida continua
sendo o core domain; o agente é a aplicação que passa a possuí-lo. O gateway é
uma capacidade genérica de borda. Notes e Agenda mantêm seus bounded contexts,
contratos, regras, dados e ledgers próprios.

O plano original pode ser executado depois das correções abaixo. A principal é
extrair uma porta de **sessão agêntica completa**, não apenas uma interface de
turno. Hoje o caminho Nova real está em
`apps/voice-gateway/internal/adapters/voice/nova.go:runNova` e contorna a porta
`VoiceProvider`, usada pelo fake. Uma abstração baseada somente em `Respond`
não caracteriza renovação, VAD contínuo, confirmação, tools ou histórico.

## Estado confirmado na base v0.7.0

- A tag `v0.7.0` já contém apps independentes para gateway, cliente, Notes e
  Agenda. Esta proposta começa depois da migração modular anterior.
- `/v1/voice` e `voicewire` publicam o Voice Protocol v1; clientes enviam JSON
  com áudio PCM16 em base64. Campos desconhecidos e versão diferente falham.
- `runNova` possui a sessão Realtime contínua, renovação, histórico, eventos,
  execução de tools e tratamento de cancelamento.
- `actions.Session` mantém intenção, confirmação posterior, cache local e a
  classificação de resultado desconhecido. O ledger durável permanece nos
  MCPs, que são donos dos efeitos.
- O prompt ainda é a constante `systemPrompt` no adaptador Nova.
- Configuração e credenciais de LiteLLM/MCP ainda entram no gateway.
- O contrato Host Context v1 é publicado pelo gateway e implementado em
  `packages/go/mcpwire`; ao final, seu produtor será o agente.

## Fronteiras e responsabilidades alvo

### `voice-gateway`

- autentica o cliente e constrói o principal confiável;
- aplica limites da conexão pública, origem, heartbeat e backpressure;
- escolhe um agente exclusivamente pelo catálogo confiável;
- traduz Voice Protocol v1 para Agent Session;
- encaminha confirmação da UI sem interpretar a política da ferramenta;
- mantém métricas de transporte e fecha a sessão de forma determinística;
- não recebe credenciais do modelo, schemas de tools nem segredos MCP.

### `personal-assistant-agent`

- possui prompt, configuração de modelo/voz e histórico semântico;
- mantém a sessão Realtime, renovação, VAD e mapeamento de turnos;
- descobre, seleciona e executa ferramentas;
- possui perfis de autorização conversacional, confirmação e intenção;
- produz Host Context v1, usando o segredo de integração;
- preserva idempotência e nunca repete automaticamente mutação de resultado
  desconhecido;
- não possui tabelas ou regras internas de Notes/Agenda.

### MCPs

Notes e Agenda continuam validando regras, confirmação recebida, assinatura,
idempotência e transação local. A mudança do chamador de gateway para agente
não transfere seus dados nem enfraquece a validação no servidor.

## Correções obrigatórias ao plano de origem

1. **Porta no nível de sessão.** A criação recebe `agentId`, principal,
   `sessionId`, `requestId`, áudio e versão. A sessão expõe áudio, commit,
   cancelamento, confirmação, eventos e fechamento. O contrato deve definir
   quem fecha canais, erro terminal e concorrência; `Events()` sozinho não é
   suficiente.
2. **Semântica primeiro, processo depois.** Mover `runNova`, `actions`, MCP e
   prompt para um módulo de agente ainda in-process antes do WebSocket interno.
   O adaptador público deve depender apenas da porta de sessão já nessa fase.
3. **Framing de áudio explícito.** Se áudio usar frames binários, cada frame
   precisa carregar ou herdar de forma inequívoca sessão, turno, sequência e
   encoding. O protocolo deve rejeitar frame órfão, gap, replay e tamanho
   excessivo. Uma mensagem de controle anterior implícita sem regra de
   atomicidade não é suficiente.
4. **Autorização em duas camadas.** O gateway autoriza cliente → agente. O
   agente autoriza conversa → tool. O `principal` é produzido pelo gateway,
   nunca aceito diretamente do frontend, e deve atravessar canal autenticado.
   Bind fora de loopback exige TLS ou mTLS; bearer simples é aceitável apenas
   para o rollout local explicitamente restrito.
5. **Falha e replay definidos.** Na perda do agente, o gateway não reenvia
   áudio, commit ou tool automaticamente. Ele encerra a sessão pública com erro
   recuperável. Um novo cliente pode reutilizar `requestId`; a mesma mutação e
   argumentos geram a mesma chave, e resultado desconhecido continua sem retry.
6. **Catálogo não concede política de tool.** O catálogo do gateway contém
   endpoint, protocolo e clientes permitidos. Perfis de tools e segredos saem
   dele e pertencem ao agente. Agente, endpoint ou política ausentes falham
   fechados.
7. **Supervisor alinhado ao runtime atual.** No modo LiteLLM, os MCPs stdio
   continuam mediados pela imagem LiteLLM; não são automaticamente novos
   processos do supervisor. No modo stdio direto, é o agente que inicia os MCPs.
   A lista de ambiente do plano deve incluir backend, catálogo de MCPs,
   timeouts e segredo Host Context, além de LiteLLM.
8. **Privacidade como gate automatizado.** Logs do gateway, agente e LiteLLM
   não registram áudio, prompt, transcrição, argumentos ou resultados. Os checks
   existentes de Usage/Spend e ausência de conteúdo devem ser repetidos nos
   dois modos.

## Contratos e versionamento

Versão do repositório e versão de protocolo são independentes.

### Voice Protocol

- permanece `v1` durante toda a extração;
- `provider` continua aceito durante a janela legada, mas passa a ser ignorado
  ou validado contra a configuração compatível, nunca convertido em endpoint;
- seleção de `agentId` não entra silenciosamente em v1: primeiro deve ser
  anunciada por capability e coberta por testes de cliente antigo/servidor novo
  e cliente novo/servidor antigo;
- nenhuma atualização do app Apple é requisito para tornar o agente remoto o
  padrão.

### Agent Session

- desenvolver inicialmente em `contracts/agent-session/v1alpha1/`;
- promover para `v1/` somente após vetores positivos/negativos, testes de
  conformidade in-process/remoto e definição de framing, limites e fechamento;
- o handshake usa um identificador inequívoco, por exemplo
  `agent-session/v1alpha1`, e não apenas um inteiro solto;
- não há downgrade automático. Perfil e agente sem versão comum falham antes de
  aceitar áudio;
- mudanças aditivas podem usar capabilities; mudança incompatível cria `v2` e
  exige coexistência no catálogo durante a migração.

### Host Context e contratos MCP

- Host Context continua wire-compatible em `v1`; não alterar canonicalização,
  HMAC, TTL ou vetores nesta separação;
- a fonte canônica move para `personal-assistant-agent` no mesmo commit em que
  ele se torna o produtor. Não manter duas cópias editáveis;
- MCP Notes/Agenda continuam em seus contratos v1. Alterações incompatíveis de
  schema ou resultado são independentes desta migração e exigem nova versão do
  respectivo fornecedor.

### Releases do monorepo

| Release proposta | Conteúdo | Compatibilidade/rollback |
| --- | --- | --- |
| `v0.8.0` | caracterização e porta de sessão in-process | runtime e Voice v1 inalterados |
| `v0.9.0` | app agente independente + Agent Session `v1alpha1`; remoto opt-in | `in-process` é padrão |
| `v0.10.0` | Agent Session v1; remoto torna-se padrão | `in-process` permanece por uma minor |
| `v0.11.0` | remoção do caminho legado e das variáveis de modelo no gateway | rollback exige voltar o conjunto de binários/configuração |

Patches corrigem uma release sem ampliar contrato. Cada tag exige changelog com
versões dos contratos suportadas e matriz de binários compatíveis. Embora o
repositório ainda esteja em `0.x`, Voice v1 e Host Context v1 não podem sofrer
quebra silenciosa.

## Estrutura alvo planejada

```text
apps/
  voice-gateway/
    contracts/voice/v1/
    contracts/agent-session/v1alpha1/  # promovido depois para v1
    config/agents/
    internal/{auth,routing,session,adapters,bootstrap}/
  personal-assistant-agent/
    prompts/system.md
    config/integrations/
    contracts/host-context/v1/
    internal/{agent,actions,ports,adapters}/
    app.json
    go.mod
    Dockerfile
  voice-client/
  mcp-notes/
  mcp-agenda/
packages/go/
  voicewire/
  agentwire/
  mcpwire/
  audio/
```

`agentwire` implementa o protocolo Go, mas a especificação canônica pertence ao
gateway, fornecedor do serviço de sessão para os agentes. `mcpwire` permanece
técnico; o contrato Host Context canônico pertence ao produtor, o agente.

## Sequência revisada

### 1. Caracterização

- congelar fixtures do Voice v1 e do comportamento Nova;
- cobrir renovação, barge-in, UI/voz para confirmação, timeout durante tool,
  desconexão depois de enviar mutação e reutilização de `requestId`;
- registrar p50/p95 de primeiro áudio e memória/backpressure;
- provar ausência de conteúdo em logs e Usage/Spend.

Gate: a mesma suíte passa no estado atual e não depende de AWS, exceto smoke
explicitamente opt-in.

### 2. Porta `AgentSession` in-process

- introduzir fábrica/conector de sessão com metadados de abertura;
- adaptar primeiro o caminho Nova completo; manter fake como outra
  implementação da mesma porta;
- fazer o WebSocket público depender apenas dessa porta;
- não criar processo ou novo protocolo ainda.

Gate: nenhum handler de voz importa Nova, actions ou MCP; comportamento e
latência permanecem dentro do baseline.

### 3. Isolar a aplicação semântica

- criar `personal-assistant-agent` com build independente;
- mover prompt, Realtime, actions, MCP, perfis e Host Context;
- manter um adaptador in-process temporário para rollback;
- carregar prompt de arquivo e falhar se ausente/vazio.

Gate: builds `GOWORK=off`, testes de fronteira e nenhum segredo LiteLLM/MCP no
processo do gateway no modo remoto.

### 4. Agent Session remoto

- publicar `v1alpha1`, implementação `agentwire` e testes de conformidade;
- autenticar os dois lados, impor limites menores que os públicos e filas
  limitadas;
- mapear todos os eventos Voice v1, inclusive confirmação, renovação, erro e
  fechamento;
- cadastrar agente fake adicional apenas por configuração.

Gate: segundo agente sem branch por nome; endpoint arbitrário do cliente é
impossível; incompatibilidade de versão e agente desconhecido falham fechados.

### 5. Rollout e promoção para v1

- executar testes espelhados `in-process`/`remote`, nunca os dois para a mesma
  sessão;
- medir custo de latência do salto interno;
- promover o contrato para v1 e tornar `remote` padrão;
- manter rollback por uma release minor completa.

Gate: app Apple e CLI atuais funcionam sem atualização, mutações não duplicam e
privacidade continua comprovada.

### 6. Remoção do legado

- remover Nova, actions, MCP, prompt e `STS_LITELLM_*` do gateway;
- remover o modo in-process apenas depois da janela prometida;
- atualizar supervisor, imagens, CI, diagrama e runbooks;
- adicionar checks que proíbem imports de Nova/MCP pelo gateway.

## Critérios de conclusão

- o gateway não contém prompt, histórico semântico, política de tool, Nova ou
  cliente MCP;
- o agente não contém regras nem persistência interna de Notes/Agenda;
- Voice v1, confirmação, idempotência e resultado desconhecido preservam os
  testes de caracterização;
- Agent Session v1 possui especificação, vetores e conformidade independente da
  linguagem;
- principal, agente e endpoint não são controlados livremente pelo cliente;
- builds e imagens do gateway e agente são independentes;
- nenhum conteúdo de conversa aparece nos logs ou no banco de Usage/Spend;
- a latência adicional possui limite definido e medição p50/p95;
- rollback foi exercitado antes da remoção do legado.

## Hipóteses ainda a validar

- agentes futuros representarão capacidades de negócio reais, e não apenas
  variantes técnicas do mesmo assistente;
- o transporte interno inicial será WebSocket local; mudança para rede remota
  exige TLS/mTLS, descoberta e threat model adicionais;
- seleção explícita de agente pelo usuário tem requisito de produto ainda não
  demonstrado; o primeiro rollout usa somente o agente padrão;
- o limite aceitável de latência adicional ainda precisa de valor de produto.
