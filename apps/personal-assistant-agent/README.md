# Personal Assistant Agent

Aplicação dona da conversa semântica: prompt, sessão Nova Realtime,
ferramentas, confirmação, perfis de integração e Host Context.

O binário publica `agent-session/v1alpha1` em loopback. O gateway pode usar esse
transporte com `STS_AGENT_MODE=remote`; `inprocess.Factory` permanece como
rollback e ainda é o padrão durante o rollout.

`STS_AGENT_PROMPT_FILE` é obrigatório e deve apontar para um arquivo regular,
não vazio e limitado a 64 KiB.

O serviço exige `STS_AGENT_TOKEN`, `STS_LITELLM_URL` e
`STS_LITELLM_API_KEY`. `STS_AGENT_ADDRESS` usa `127.0.0.1:8090` por padrão e
endereços não loopback são rejeitados enquanto TLS/mTLS não estiver disponível.
