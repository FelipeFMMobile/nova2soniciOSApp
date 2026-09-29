# Alternativa futura — LiteLLM como Agent Gateway

Data do registro: 2026-09-28.

Estado: **adiada; não faz parte do plano vigente**.

## Decisão atual

A migração seguirá o plano em
[`agent-migration-plan.md`](agent-migration-plan.md): App Apple e
`voice-client` são clientes irmãos do `voice-gateway`, e o gateway se conecta
diretamente ao `personal-assistant-agent` por meio do Agent Session versionado.

```text
App Apple ─────┐
               ├── Voice Protocol v1 ──> voice-gateway
voice-client ──┘                              │
                                             └── Agent Session ──> personal-assistant-agent
```

Este registro não altera fases, contratos, releases ou critérios de conclusão
do plano atual.

## Alternativa a reavaliar no futuro

O LiteLLM oferece um Agent Gateway baseado em A2A, além dos gateways de modelos
e MCP. Uma evolução futura poderia registrar o `personal-assistant-agent` no
LiteLLM e fazer o `voice-gateway` invocá-lo por essa camada:

```text
App Apple / voice-client
          │
          │ Voice Protocol v1
          ▼
    voice-gateway
          │
          │ A2A
          ▼
       LiteLLM
          │
          ▼
personal-assistant-agent
          │
          ├── Realtime via LiteLLM ──> Nova
          └── MCP via LiteLLM ───────> Notes / Agenda
```

Nessa topologia, o LiteLLM poderia centralizar catálogo de agentes,
autenticação, autorização por chave/equipe, rate limiting, balanceamento,
versão A2A, tracing e atribuição de uso. O `voice-gateway` ainda seria o BFF de
voz: os clientes não dependeriam diretamente de A2A, Realtime ou credenciais
do LiteLLM.

## Por que a alternativa foi adiada

- A documentação de A2A cobre mensagens, tasks, artifacts e streaming, mas isso
  não comprova transporte full-duplex de PCM16 com a latência exigida.
- Barge-in, cancelamento imediato, ordenação de áudio, backpressure e confirmação
  da UI precisam de equivalência demonstrada, não presumida.
- O Agent Gateway registra conteúdo de request/response nos logs por padrão; a
  política deste repositório exige provar ausência de áudio, prompt,
  transcrições e payloads MCP.
- O salto adicional pode aumentar a latência até o primeiro áudio.
- O agente chamaria o mesmo LiteLLM novamente para Realtime e MCP; tracing,
  limites, falhas recursivas e isolamento precisam ser testados.
- A versão do LiteLLM fixada pelo repositório deve ser validada diretamente;
  documentação atual não prova compatibilidade do artefato local.
- A migração atual já possui um caminho incremental e rollback explícito. Trocar
  o transporte ao mesmo tempo ampliaria o risco sem necessidade imediata.

## Opções para um futuro spike

1. **A2A puro:** LiteLLM transporta toda a sessão agêntica, inclusive mídia, se
   a conformidade de voz for comprovada.
2. **Híbrido:** LiteLLM governa catálogo, identidade e descoberta; áudio usa um
   canal dedicado entre gateway e agente.
3. **Agent Session próprio:** manter o protocolo atual e usar LiteLLM somente
   para modelo e MCP.

A opção híbrida é a candidata inicial mais plausível, mas não é uma decisão.

## Gates antes de qualquer adoção

- provar suporte a PCM16 contínuo, duplex, sequência e limites;
- medir p50/p95 de primeiro áudio contra conexão direta;
- cobrir barge-in, cancelamento, timeout e fechamento parcial;
- preservar confirmação, idempotência e mutação de resultado desconhecido;
- impedir endpoint/agente arbitrário escolhido pelo cliente;
- provar logs, Usage/Spend e banco sem conteúdo da conversa;
- validar A2A e recursos necessários na versão exata do LiteLLM usada;
- exercer falha do LiteLLM entre gateway e agente sem repetir mutações;
- definir rollout e rollback independentes do upgrade de LiteLLM.

## Fontes para revalidação

Consultadas em 2026-09-28; capacidades e comportamento podem mudar:

- [LiteLLM Agent Gateway — A2A](https://docs.litellm.ai/docs/a2a)
- [LiteLLM — topologias de modelos, MCPs e agentes](https://docs.litellm.ai/docs/mcp_deployment)
- [LiteLLM — visão geral](https://docs.litellm.ai/docs/)
