# Runbook — Nova 2 Sonic via LiteLLM

## Arquitetura e fronteira de credenciais

O fluxo de produção local é:

```text
App Apple → /v1/voice no gateway Go → /v1/realtime no LiteLLM → Bedrock/Nova
                                      ↕
                            MCP Notes e Agenda
```

O Go controla sessão, turnos, barge-in, renovação, políticas, confirmação e
execução das ferramentas. Ele recebe somente a chave virtual
`sts-go-gateway`. O perfil AWS é montado em `/root/.aws:ro` somente no
container LiteLLM. Não exporte credenciais AWS para o gateway.

O PostgreSQL do LiteLLM armazena chaves, configuração e Usage/Spend. Prompts,
transcrições, áudio, argumentos e resultados MCP não devem ser persistidos. Os
SQLite de Notes e Agenda continuam funcionais e mantêm seus ledgers de
idempotência. Bancos antigos não são apagados; as antigas tabelas de auditoria
do Go apenas deixam de ser acessadas.

A imagem aplica correções pequenas e versionadas sobre `v1.103.0-dev.2`. Elas
removem `arguments` e `result` de `metadata.mcp_tool_call_metadata` quando
`store_prompts_in_spend_logs` está desligado e corrigem a transformação
Bedrock Realtime para endpointing e chamadas de ferramentas da Nova 2 Sonic.
Os detalhes e o procedimento de rebuild estão na seção **Imagem LiteLLM
customizada** do README. O build falha se os trechos da versão fixada mudarem,
obrigando revisão explícita dos patches.

## Preparar AWS e iniciar

1. Habilite `amazon.nova-2-sonic-v1:0` em `us-east-1`.
2. Anexe [`deploy/aws/iam-policy.json`](../deploy/aws/iam-policy.json) ao role ou
   usuário. A identidade precisa de
   `bedrock:InvokeModelWithBidirectionalStream` para o modelo.
3. Crie ou atualize um perfil local, preferencialmente com SSO:

   ```bash
   aws configure sso --profile sts-poc
   aws sso login --profile sts-poc
   aws sts get-caller-identity --profile sts-poc --region us-east-1
   ```

4. Crie `deploy/litellm/.env` a partir do exemplo local e mantenha-o fora do
   Git. Nunca grave access keys nesse arquivo.
5. Execute `make dev`, selecione Nova e informe `sts-poc`. O supervisor sobe
   LiteLLM/PostgreSQL, valida a identidade AWS de dentro do container, cria ou
   atualiza a chave virtual e inicia somente o gateway Go.

A configuração canônica é [`deploy/litellm/config.yaml`](../deploy/litellm/config.yaml):

```yaml
model_name: nova-sonic
model: bedrock/amazon.nova-2-sonic-v1:0
aws_region_name: us-east-1
model_info:
  mode: realtime
```

Para início manual, suba o LiteLLM com `AWS_PROFILE=sts-poc make litellm-up`,
gere a chave com `scripts/litellm_service_key.py` e exporte:

```bash
export STS_PROVIDER=nova
export STS_LITELLM_URL=http://127.0.0.1:4000
export STS_LITELLM_API_KEY='chave-virtual-sts-go-gateway'
export STS_LITELLM_REALTIME_MODEL=nova-sonic
export STS_DEVELOPMENT_TOKEN=local-demo-token
make gateway
```

## Cadastrar ou conferir pelo painel

Este procedimento foi escrito para a versão fixada
`v1.103.0-dev.2`. O YAML versionado continua sendo a fonte canônica e
reproduzível.

1. Abra <http://127.0.0.1:4000/ui> e autentique-se como administrador.
2. Acesse **Models + Endpoints → Add Model**.
3. Selecione **AWS Bedrock**.
4. Use `nova-sonic` como nome público.
5. Informe `amazon.nova-2-sonic-v1:0`, região `us-east-1` e modo `realtime`.
6. Se a tela oferecer autenticação por perfil/ambiente, selecione o perfil já
   montado. Não cole credenciais permanentes no painel.
7. Salve e use **Test Connection** somente sabendo que uma conexão Realtime
   real pode gerar cobrança.
8. Em **Virtual Keys**, edite `sts-go-gateway`, permita apenas o modelo
   `nova-sonic`, a rota `/v1/realtime` e os MCPs necessários.
9. Execute uma conversa curta em PT-BR e confira o primeiro registro em
   **Usage/Spend**, filtrando por modelo `nova-sonic`, chave e session ID.

Nesta versão, se a UI não expuser `mode: realtime` ou autenticação por perfil
para Nova Sonic, não improvise campos. Crie/edite o modelo no YAML (ou pela API
administrativa equivalente), reinicie o LiteLLM e use o painel apenas para
administrar a chave e monitorar Usage/Spend.

## Editar, testar e remover

- Editar: altere primeiro o YAML, execute `make litellm-down` e depois
  `AWS_PROFILE=sts-poc make litellm-up`; confirme o modelo em **Models +
  Endpoints**.
- Testar: abra uma sessão Realtime curta pelo gateway. O aceite exige áudio,
  fechamento imediato da sessão e spend positivo associado ao modelo/chave/
  sessão.
- Remover: retire `nova-sonic` da chave virtual antes de remover o cadastro do
  modelo. Não apague o PostgreSQL nem execute `docker compose down -v`; assim os
  spend logs históricos permanecem.

## Verificação de privacidade e custo

Antes do corte, confirme:

1. `Usage/Spend` mostra custo maior que zero para a sessão real.
2. O processo do gateway não possui `AWS_*` nem acesso a `~/.aws`.
3. Logs do gateway/LiteLLM e PostgreSQL não contêm uma frase de teste,
   transcrição, áudio base64, argumentos ou resultados MCP.
4. `general_settings.disable_spend_logs` está `false`, enquanto
   `store_prompts_in_spend_logs`, `turn_off_message_logging` e
   `log_raw_request_response` mantêm conteúdo desabilitado.
5. Uma queda do LiteLLM produz `provider_unavailable`; não existe fallback
   direto para AWS.

Referências: [provider Bedrock do LiteLLM](https://github.com/BerriAI/litellm-docs/blob/main/docs/providers/bedrock.md),
[cost tracking](https://github.com/BerriAI/litellm-docs/blob/main/docs/proxy/cost_tracking.md) e
[Admin UI](https://github.com/BerriAI/litellm-docs/blob/main/docs/proxy/docker_quick_start.md).
