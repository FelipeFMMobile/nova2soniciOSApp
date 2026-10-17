# Agente Conta: login conversacional, MFA e edição de perfil

Plano aprovado pelo usuário. Registro em 2026-10-06.

**Estado:** implementação em andamento; este documento descreve o destino aprovado,
não certifica a conclusão. A execução e as pendências estão em
[account-agent-tasks.md](account-agent-tasks.md). Os registros anteriores de
`docs/domains/` continuam sendo históricos, sem reclassificar propostas antigas
como entregas concluídas.

## 1. Experiência e escopo

Adicionar **Conta** ao app iOS/macOS, mantendo **Agenda/notas** independente e
sem exigir login nessa funcionalidade nesta entrega.

Conta orienta por voz, apresenta A2UI e, após login, permite consultar e editar
**nome, idade, cidade, gênero e profissão**.

1. O usuário seleciona Conta e pede para entrar.
2. O agente aciona o MCP de identidade e apresenta um cartão A2UI com instruções
   e botão **Autenticar**.
3. O app abre Cognito Managed Login com `ASWebAuthenticationSession`.
4. O usuário informa senha e código recebido por e-mail na interface segura.
5. O backend valida a autenticação; o app retoma a conversa autenticada.
6. Alterações de perfil são apresentadas para confirmação antes da gravação.

Referências escolhidas:

- [Account linking da Alexa](https://developer.amazon.com/docs/alexaplus/account-linking/app-to-app-account-linking-starting-from-your-app.html).
- [OAuth Authorization Code com PKCE no Cognito](https://docs.aws.amazon.com/cognito/latest/developerguide/using-pkce-in-authorization-code.html).
- [MFA por e-mail do Cognito com SES](https://docs.aws.amazon.com/cognito/latest/developerguide/user-pool-settings-mfa-sms-email-message.html).

A voz orienta o fluxo; não será usada como biometria.

## 2. Aplicações, limites e propriedade de dados

| Componente | Responsabilidade |
|---|---|
| `account-agent`, Go | Conversação de Conta, prompt próprio, políticas, confirmação e A2UI |
| `mcp-identity`, Spring AI | Identidade local, vínculo Cognito, transações OAuth, sessões, revogação e `CLIENTE_CODE` |
| `mcp-profile`, Spring AI | Cinco campos de perfil, validação, versionamento e ledger de idempotência |
| Voice gateway | Seleção autorizada do agente, transporte e identidade validada |
| LiteLLM | Inferência, acesso aos MCPs, chaves por agente e guardrails |
| App Apple | Menu, renderização A2UI, autenticação do sistema e Keychain |

- Identidade é domínio genérico; perfil é domínio de suporte. Spring, MCP e banco
  são escolhas técnicas, não domínios de negócio.
- Os MCPs terão builds Maven independentes, Java 21, Spring AI estável com BOM
  compatível e versões fixadas. Usar Streamable HTTP, Spring Security,
  autenticação entre serviços e portas internas.
- O [transporte Spring AI não fornece autorização automaticamente](https://docs.spring.io/spring-ai/reference/api/mcp/mcp-server-boot-starter-docs.html).
- Cada MCP terá PostgreSQL, credenciais, migrações e volume próprios, separados
  do banco LiteLLM. Nenhuma aplicação consultará tabelas de outra.
- Identidade guarda ID interno, vínculo `issuer/sub`, e-mail de autenticação e
  estado da conta. Perfil é o único dono dos cinco campos editáveis.
- Extrair apenas infraestrutura reutilizável de conversação/transporte para
  pacotes técnicos Go. Prompts e regras permanecem nas aplicações respectivas.
- Atualizar análise e plano em `docs/domains/` conforme o harness, preservando
  histórico e registrando evidências, classificação e coesão.

## 3. Voz e A2UI

- Preservar Voice v1 e introduzir Voice v2 e AgentSession v2 para seleção de
  agente, eventos A2UI e contexto autenticado, com coexistência explícita.
- O cliente escolhe somente IDs permitidos pelo catálogo. O modelo não fornece
  endpoints, credenciais nem identidade.
- Implementar renderizador SwiftUI limitado ao catálogo necessário de A2UI
  v0.9.1: texto, agrupamento e botão; criação, atualização e remoção de
  superfícies e ações tipadas.
- Usar templates determinísticos; rejeitar componentes, ações e destinos
  desconhecidos. Não depender do [renderizador SwiftUI oficial, ainda indicado
  como planejado na referência escolhida](https://github.com/a2ui-project/a2ui/blob/main/docs/public/reference/renderers.md).
- Botão aciona operação nativa conhecida. Senhas, OTPs, códigos OAuth e tokens
  não passam por ações do modelo, resultados MCP ou transcrições.
- Suspender áudio durante autenticação. Retomar a conversa após retorno do
  navegador, inclusive após encerramento da conexão em segundo plano no iOS.
- Trocar agente encerra a conversa anterior e descarta confirmações pendentes.

## 4. Identidade, OAuth e sessão

- Ferramentas: `identity.begin_login`, `identity.login_status`, `identity.logout`.
- Início retorna apenas estado e referência opaca. Dados sensíveis de autorização
  passam por canal de controle autenticado do app.
- Endpoints separados para preparar/completar OAuth, renovar sessão e sair.
  Troca de códigos/tokens nunca é ferramenta do modelo.
- Vincular transação ao app iniciador, com PKCE S256, `state`, `nonce`, expiração
  e consumo único. Validar assinatura, emissor, cliente, finalidade, prazo e
  associação dos tokens Cognito.
- Sessão opaca: acesso de 15 minutos e renovação rotativa por até 30 dias,
  limitada também pela validade Cognito. Credencial renovável no Keychain,
  sem sincronização entre dispositivos.
- Persistir hashes de credenciais opacas e criptografar tokens Cognito.
- Logout revoga backend e limpa Keychain.
- Conversa pré-login usa identidade técnica anônima sem permissões de perfil.
  Login concluído abre nova sessão de voz com identidade validada.

## 5. CLIENTE_CODE e autorização

- Gerar uma única vez `CLIENTE_CODE` com HMAC-SHA256 de UUID interno e chave
  dedicada; persistir resultado e versão. Não derivar de e-mail/telefone.
- Contexto MCP v2 assinado contém identidade, código, referência de sessão e
  escopos, além de audience, ferramenta, hash do payload, confirmação,
  expiração e idempotência.
- Preservar MCP v1 de Notes/Agenda; publicar vetores Go/Java v2 de
  canonicalização e adulteração.
- Perfil verifica assinatura e sessão ativa via serviço de identidade; se a
  verificação estiver indisponível, recusa a operação.
- `CLIENTE_CODE` é identificador, nunca credencial. Ferramentas de perfil não
  recebem identificador de usuário como argumento de negócio.

## 6. Perfil e guardrails

Ferramentas: `profile.get_me` e `profile.update_me`; atualização parcial com
versão esperada e confirmação vinculada ao conteúdo exato.

| Campo | Regra |
|---|---|
| Nome | Obrigatório; até 120 caracteres |
| Idade | Inteiro entre 0 e 130; sem incremento automático |
| Cidade | Opcional; até 120 caracteres |
| Gênero | Texto opcional até 80 caracteres; sem inferência |
| Profissão | Opcional; até 120 caracteres |

Campos opcionais podem ser removidos.

- Atualização e ledger na mesma transação. Mesma operação retorna resultado
  anterior; versão divergente exige leitura e nova confirmação.
- Chave LiteLLM exclusiva de Conta; ferramentas permitidas e guardrails
  obrigatórios de entrada/saída MCP.
- Bloquear campos extras, substituição de identidade, credenciais em payloads
  e resultados fora do contrato.
- Preservar os cinco campos nas respostas; não usar mascaramento genérico que
  inviabilize edição legítima.
- Validar hooks na imagem LiteLLM fixada usando `/mcp`, com bloqueios antes e
  depois da chamada, conforme a [referência MCP guardrails](https://docs.litellm.ai/docs/mcp_guardrail).
- Não presumir que guardrails textuais inspecionam áudio Realtime.
- Conta só fica pronta com guardrails registrados e testes de bloqueio passando.
  Payloads assinados são rejeitados, nunca reescritos.
- Logs, spend e diagnóstico não expõem segredos nem envelopes internos.
  Métricas registram resultado, duração e motivo, sem conteúdo pessoal.

## 7. Implementação incremental

1. **Contratos e infraestrutura:** manifestos, wrappers Maven, containers,
   bancos, contratos v2, CI Java; provar Go → LiteLLM → Spring antes de AWS.
2. **Identidade:** OAuth, sessões, código, APIs internas e Cognito fake local.
3. **Conta e Apple:** agente, prompt, menu, A2UI, autenticação do sistema,
   Keychain e retomada.
4. **Perfil e guardrails:** consulta, confirmação, atualização e autorização.
5. **Validação integrada:** regressões, builds Apple, testes Java/Python e
   smoke real com AWS configurada.

## 8. make dev e fixtures

- Selecionar agentes e subir serviços necessários.
- Solicitar região, User Pool, app client, domínio e e-mails fictícios.
- Verificar Essentials ou superior, SES, MFA por e-mail, callbacks e
  permissões sem alterar configuração global dos recursos existentes.
- Semear três perfis fictícios idempotentemente. No modo AWS, cada conta
  habilitada usa e-mail de teste; permitir menos contas reais se houver
  somente um destinatário.
- Criar apenas usuários explicitamente marcados como fixtures; nunca
  sobrescrever contas ou redefinir senhas existentes. Usar convite Cognito.
- Manter fake sem AWS e `--dry-run` sem mutações.

## 9. Testes obrigatórios e aceite

- Login completo, cancelamento, código inválido/expirado, callback repetido,
  PKCE/state/nonce incorretos e indisponibilidade SES/Cognito.
- Primeiro acesso e contas realmente sujeitas a MFA. Token emitido sozinho
  não é evidência de segundo fator.
- Reinício do app, renovação, logout, sessão revogada e retorno após suspensão.
- A não lê/edita B, inclusive com argumentos fabricados pelo modelo.
- Confirmação, concorrência, replay e resultado desconhecido da atualização.
- Guardrail bloqueia chamada antes do efeito e resultado antes do modelo.
- Ausência de segredos em logs, spend, ações A2UI e diagnóstico Apple.
- Regressão Agenda/notas, Voice v1, interrupção, áudio e confirmação.

## 10. Ativação, rollback e limites

Entrega habilitada por configuração. Rollback desabilita Conta e seus MCPs,
preserva bancos e mantém funcionamento anterior de Agenda/notas.

- Usar Cognito/SES existentes; IDs fornecidos na configuração local, sem
  compartilhar segredos no chat.
- Fora de escopo: recuperação administrativa, troca de e-mail MFA, cadastro
  público e biometria de voz.
- iPhone físico exige HTTPS/WSS com certificado confiável. HTTP apenas em
  loopback nos testes locais.
- Nenhuma alteração nos dados de Agenda/notas.
