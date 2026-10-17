# Agente Conta — tarefas e estado da implementação

Atualizado em 2026-10-06. Plano: [account-agent-plan.md](account-agent-plan.md).
Base: `4706b73f8c0a29774e07561c39c83712ce1574b5`.
As alterações estão no checkout, ainda sem commit. Marcas de implementação
não equivalem a homologação. A entrega completa permanece em andamento.

## 1. Arquitetura e infraestrutura

- [x] Ler harness, roteiro e skill local; validar cópia do harness.
- [x] Criar aplicações Go Conta e Java Identidade/Perfil com manifestos próprios.
- [x] Criar builds Maven independentes, Java 21, Spring Boot 3.5.9 e Spring AI 1.1.2.
- [x] Criar containers e PostgreSQL separados, migrações e volumes próprios.
- [x] Extrair adaptador Realtime e orquestração técnica para pacotes Go.
- [x] Adicionar CI Java e seleção de linguagem nos builds agregados.
- [x] Registrar plano aprovado e esta lista de tarefas.
- [x] Concluir atualização da análise de domínios, coesão, evidências e roadmap.
- [x] Validar builds isolados e imagens finais após todas as alterações.

## 2. Protocolos e autorização

- [x] Implementar negociação explícita Voice v2 e AgentSession v2.
- [x] Preservar testes e contratos Voice v1/AgentSession v1.
- [x] Propagar identidade validada e seleção por catálogo autorizado.
- [x] Implementar envelope MCP v2 assinado com identidade, escopos e sessão.
- [x] Criar vetores Go/Java e testes de adulteração/canonicalização.
- [x] Documentar protocolos v2, negociação, headers e canal de controle.
- [x] Acrescentar exemplos de abertura, login/A2UI, confirmação/cancelamento e
      encerramento aos contratos públicos v2; áudio/transcrição seguem v1.
- [x] Ampliar testes gateway: seleção, transporte seguro, headers e canal OAuth.

## 3. Identidade

- [x] Implementar início, preparação e conclusão OAuth fora das ferramentas.
- [x] Implementar PKCE S256, state, nonce, vínculo ao dispositivo e consumo único.
- [x] Implementar adaptador Cognito e fake restrito a desenvolvimento.
- [x] Validar tokens Cognito e preferência EMAIL_OTP do usuário.
- [x] Gerar e persistir CLIENTE_CODE por HMAC de UUID interno.
- [x] Persistir hashes de credenciais e criptografar refresh Cognito com AES-GCM.
- [x] Implementar renovação rotativa, revogação e detecção de replay.
- [x] Permitir logout com credencial renovável após expiração do acesso.
- [x] Passar testes Java de ciclo de sessão, expiração, callback repetido,
      state/dispositivo incorretos, código inválido, fixtures e criptografia.
- [x] Adicionar testes locais do adaptador Cognito: JWT assinado, nonce, audiência,
      PKCE, expiração, MFA configurado, refresh/revogação e falhas HTTP.
- [x] Provisionar Cognito Essentials/SES, cliente público, MFA por e-mail e ambiente local.
- [ ] Homologar primeiro acesso e MFA obrigatório real no Cognito/SES configurado.

## 4. Perfil

- [x] Implementar cinco campos, validação, remoção opcional e versão esperada.
- [x] Implementar consulta de sessão ao serviço de identidade, recusando falhas.
- [x] Implementar atualização e ledger na mesma transação.
- [x] Implementar políticas de confirmação no agente.
- [x] Passar testes unitários Java de validação dos campos.
- [x] Corrigir `invalid_host_context` na atualização: transporte MCP Spring agora
      preserva valores nulos no payload assinado; regressões Go/Java adicionadas.
- [x] Passar teste integrado de isolamento A/B, replay, versão e revogação.
- [x] Validar concorrência e resultado desconhecido de gravação contra PostgreSQL:
      replay simultâneo, versão concorrente e perda de resposta após commit.

## 5. Conta, Apple e A2UI

- [x] Adicionar prompt e políticas próprias do agente Conta.
- [x] Adicionar menu Conta / Agenda-notas no app.
- [x] Implementar catálogo A2UI restrito com operações de superfície.
- [x] Implementar ASWebAuthenticationSession e Keychain sem sincronização.
- [x] Suspender áudio e abrir nova conversa após autenticação.
- [x] Encerrar conversa e confirmações ao trocar agente.
- [x] Ocultar payloads técnicos de Conta no diagnóstico do app.
- [x] Adicionar testes de versão explícita e rejeição de A2UI desconhecida.
- [x] Corrigir compilação Swift no logout com `self.error`.
- [x] Reexecutar testes Swift e builds macOS/iOS finais.
- [x] Validar login fake, reinício/restauração pelo Keychain, logout e troca de
      agentes no iOS Simulator (XCTest).
- [ ] Validar cancelamento e suspensão/retomada do navegador real no iOS.
- [x] Validar confirmação/cancelamento local: transcrição final simulada e
      handler do botão, com MCP real, chave exata, expiração e gravação única.
- [ ] Homologar edição por ASR Nova e cliques no app em conversa completa.

## 6. LiteLLM e guardrails

- [x] Criar configuração dedicada e chaves com permissões separadas por agente.
- [x] Implementar guardrails de contrato pre_mcp_call e post_mcp_call.
- [x] Corrigir headers internos para `static_headers` na imagem fixada.
- [x] Provar Go → LiteLLM → Spring por `/mcp` em containers locais.
- [x] Passar canários reais de bloqueio de entrada e saída pelo LiteLLM.
- [x] Condicionar prontidão de Conta ao sucesso dos dois canários.
- [x] Adicionar testes Python do guardrail e prevenção de vazamento em erros.
- [x] Auditar logs dos serviços/diagnóstico local com canário de perfil/nota e
      segredos sintéticos; resultados Apple sanitizados com regressão específica.
- [ ] Auditar conteúdo/spend Realtime em conversa Nova; smoke MCP não certifica
      esse critério se não houver linhas de spend produzidas.
- [x] Validar regressão Agenda/notas v1 na composição ampliada, com chave própria:
      criação/replay/exclusão confirmada de nota e consultas de Agenda.

## 7. Desenvolvimento e entrega

- [x] Estender make dev com ambiente Conta fake/AWS e preservação do fluxo legado.
- [x] Implementar configuração local, preflight Cognito/SES, convites e seed idempotente.
- [x] Preservar dry-run sem escrita, criação AWS ou inicialização de serviços.
- [x] Executar regressão Go completa, race, vet e builds isolados de Conta,
      gateway e agente pessoal na retomada; ver registro de execução.
- [x] Acrescentar testes de configuração/seed/dry-run Conta, guardrails e runner;
      regressão Python: 39 testes, um opt-in não executado.
- [x] Testar supervisor: falha parcial de lançamento/prontidão, processo encerrado
      e Ctrl+C; cleanup dos filhos/logs, bancos preservados, credenciais isoladas.
- [x] Executar make dev fake completo e publicar comando de smoke reproduzível.
- [x] Executar race, vet, builds isolados e regressão final.
- [ ] Executar smoke real AWS somente após configuração local disponível.
- [x] Documentar operação, limitações verificadas, ativação e rollback.
- [ ] Revisar diff final e registrar evidências sem segredos.

## Evidências e limites atuais

Registro detalhado: [run-2026-10-06.md](run-2026-10-06.md).

- Java: 18 testes de identidade aprovados, incluindo 9 específicos Cognito com
  HTTP simulado e validação JWT real; 5 de perfil no checkpoint anterior.
- Go: build, test, race, vet e builds isolados aprovados na retomada.
- Três testes live aprovados contra LiteLLM/Spring: guardrails, identidade/perfil
  com confirmação, A/B, replay, versão e revogação; AgentSession v2/A2UI.
- Nova execução em imagens reconstruídas aprovou três subtestes de concorrência
  e perda de resposta; confirmação de gravação única pelo versionamento do perfil.
- Swift VoiceCore: 28 testes, um opt-in não executado. Builds macOS/iOS aprovados.
- XCTest: dois testes aprovados; Conta fake com login, restart/Keychain, logout,
  troca de agentes, e conversa Agenda/notas. Runner mantém assinatura ad hoc.
- Smoke reproduzível: `make account-smoke` / `make account-ui-test`.
- Não houve smoke AWS, comprovação de MFA real nem homologação em iPhone físico.
  Cognito/SES foram provisionados posteriormente; preflight real aprovado,
  primeiro acesso e desafio MFA ainda pendentes.
- Ambiente smoke usa dados fictícios e remove seus containers/volumes/credenciais.

## Próximas tarefas locais

1. Revisão final do diff e ensaio completo de perfil no app com Nova/ASR,
   incluindo auditoria de conteúdo e spend Realtime.
2. Homologação Cognito/SES/MFA e suspensão/retorno do navegador com a
   configuração AWS disponível; não considerar homologados pelo modo fake.


## Experiência Conta Mac/iOS — 2026-10-17

- [x] Apresentação exata ao iniciar uma sessão anônima, falada pelo agente via Nova Sonic.
- [x] Esperar sim/não por voz finalizada ou pelos botões; recusa encerra a sessão.
- [x] Criar transação de login somente após consentimento explícito.
- [x] Abrir modal “Entre com seus dados” com botão Autenticar.
- [x] Carregar Cognito/MFA em WKWebView dentro do mesmo modal no Mac e iOS.
- [x] Interromper microfone/conversa durante autenticação, restringir navegação ao
      domínio configurado, usar cookies temporários e preservar validação PKCE/state.
- [x] Compilar Mac/iOS e testar navegação permitida/bloqueada, sim/não e concorrência.
- [x] Executar XCTest no simulador: recusa, aceite, modal, login fake, restore e logout.
- [ ] Repetir login e MFA reais no modal com o usuário; teste fake não comprova AWS.


### Correção da voz da apresentação — 2026-10-17

- [x] Remover AVSpeechSynthesizer da apresentação no app Mac/iOS.
- [x] Agente inicia saudação real via LiteLLM/Nova Sonic, com áudio e transcrição do provedor.
- [x] Captura inicial não cancela a saudação; VAD mantém a interrupção por fala real.
- [x] Bootstrap técnico do adaptador não aparece como fala nem consentimento do usuário.
- [x] Teste real Nova: saudação sem microfone e agente com microfone silencioso,
      áudio recebido e zero chamadas de login antes do consentimento.
- [x] Builds macOS/iOS e regressões Go com race aprovados.


### Confirmação de conta conectada — 2026-10-17

- [x] Falar pela Nova Sonic “Conta conectada com sucesso, agora você pode consultar
      e editar informações da sua conta” ao iniciar sessão autenticada.
- [x] Aplicar ao retorno do login e à restauração pelo Keychain/refresh, usando
      somente identidade autenticada confirmada pelo gateway.
- [x] Não solicitar novo consentimento de login nem consultar/editar dados na confirmação.
- [x] Testes do runtime e ensaio com Nova real aprovados; 72 quadros de áudio,
      zero chamadas de login e nenhum bootstrap publicado como fala do usuário.
- [x] XCTest integrado aprovado: confirmação presente após login fake e após restauração no restart.
