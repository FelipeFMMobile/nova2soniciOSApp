# LiteLLM: Realtime, MCP e custos

LiteLLM `v1.103.0-dev.2` é o gateway único entre o Go e serviços externos. A
mesma chave virtual autoriza `nova-sonic`, `/v1/realtime` e os MCPs escolhidos.

O Go continua sendo a autoridade de schemas, intenção explícita, confirmação,
idempotência e execução. LiteLLM autentica a chave, aplica grants e encaminha
as chamadas para os servidores stdio no container.

Variáveis do gateway:

```bash
STS_LITELLM_URL=http://127.0.0.1:4000
STS_LITELLM_API_KEY=...
STS_LITELLM_REALTIME_MODEL=nova-sonic
STS_MCP_BACKEND=litellm
STS_MCP_CONTEXT_SECRET=...
STS_LITELLM_MCP_SERVERS='[...]'
```

PostgreSQL mantém configuração, chaves e Usage/Spend. A configuração desliga
armazenamento de prompts e logging de mensagens/respostas brutas. Consulte o
[runbook](nova-litellm-runbook.md).
