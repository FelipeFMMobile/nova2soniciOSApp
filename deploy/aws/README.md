# AWS Bedrock

O LiteLLM é o único componente que acessa a AWS. O gateway Go não deve receber
variáveis `AWS_*`, arquivos de perfil ou permissões Bedrock.

1. Habilite `amazon.nova-2-sonic-v1:0` em `us-east-1`.
2. Anexe `iam-policy.json` à identidade de desenvolvimento.
3. Configure um perfil local/SSO e autentique-o.
4. Inicie com `AWS_PROFILE=sts-poc make litellm-up` ou use `make dev`.

O Compose monta `${HOME}/.aws` em `/root/.aws:ro` somente no container
LiteLLM. Não coloque access keys no repositório ou em `deploy/litellm/.env`.

A identidade precisa de `bedrock:InvokeModelWithBidirectionalStream` para:

```text
arn:aws:bedrock:us-east-1::foundation-model/amazon.nova-2-sonic-v1:0
```

Veja [o runbook completo](../../docs/nova-litellm-runbook.md) para configurar o
modelo, executar o smoke e conferir Usage/Spend.
