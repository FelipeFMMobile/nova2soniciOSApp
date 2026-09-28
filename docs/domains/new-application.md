# Adicionar uma aplicação independente

1. Crie `apps/<nome>` com build/test próprios. Go usa go.mod; Spring pode usar
   Maven Wrapper e pom.xml. Organize módulos por capacidade de negócio, com
   interfaces explícitas e dados/migrations sob responsabilidade da aplicação.
2. Publique schemas, resultados, erros e exemplos em `contracts/v1/` dentro da
   app. O adaptador MCP deve anunciar esses mesmos schemas. Implemente ledger
   atômico junto do efeito e valide autorização antes de ações destrutivas.
3. Crie `app.json` com `name`, `language` e arrays `build`, `test`, `run`. Os
   comandos executam no diretório da aplicação, sem shell. O agregador de build
   descobre os manifestos. Para Spring, um build pode ser
   `["./mvnw", "-B", "verify"]`; os testes Maven devem fazer parte desse build.
   O CI Go enumera módulos Go; adicione um job Java com o JDK escolhido ao
   implementar o primeiro app Spring. Nenhuma versão de Spring é imposta agora.
4. No gateway, adicione um perfil JSON confiável. Exemplo para uma nova tool:

   ```json
   {"tools":{"inventory.remove":{"policy":"confirm_later","confirmation_phrase":"confirmo remover item"}}}
   ```

   Carregue-o via `STS_INTEGRATIONS_DIR` sem recompilar o gateway, ou mantenha-o
   em `apps/voice-gateway/config/integrations/<nome>/profile.json` para embutir
   no build. Perfis adicionais não podem sobrescrever os embutidos. Políticas
   permitidas são read_only, explicit_intent e confirm_later. explicit_intent
   exige exatamente `direct: true` ou `intent_pattern` (regex sobre fala
   normalizada); confirm_later exige frase. `allow_ui_confirmation` é opt-in.
   `datetime_fields` valida formato RFC3339 de campos, não regras de agenda.
5. Configure allowlist e política do servidor em `STS_MCP_SERVERS` (stdio) ou
   `STS_LITELLM_MCP_SERVERS` (LiteLLM). Nomes/políticas devem concordar com o
   perfil. No stdio, comando/cwd são absolutos e `env` referencia nomes de
   variáveis do host. Para o supervisor, adicione `runtime.json` ao diretório
   de integração, seguindo Notes/Agenda, e selecione `--mcps <nome>`.
6. Para LiteLLM, cadastre o servidor na plataforma e use seu server_id no
   runtime. Java stdio requer JRE no ambiente que inicia o processo. Uma imagem
   Docker independente não cria transporte HTTP automaticamente. Valide o
   transporte remoto antes de mudar a composição atual; a migração preserva
   os binários stdio de Notes/Agenda na imagem LiteLLM.
7. Execute os testes de contrato da app, os vetores do envelope publicados
   pelo gateway e integração de confirmação/replay/timeout. Bibliotecas Go
   não são dependência de uma implementação Java. Os vetores atuais foram
   verificados em Go; a interoperabilidade Java deve ser provada na primeira app.

Não compartilhar banco, entidade ou import interno com outra aplicação. Caso
haja consumidor fora deste checkout, publicar versões das bibliotecas técnicas
em registry antes de substituir os `replace` locais usados no monorepo.
