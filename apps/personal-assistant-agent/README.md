# Personal Assistant Agent

Aplicação dona da conversa semântica: prompt, sessão Nova Realtime,
ferramentas, confirmação, perfis de integração e Host Context.

O binário publica `agent-session/v1` em loopback. O gateway pode usar esse
transporte para Nova e fake. `session.Factory` compõe o runtime dentro do agente;
o gateway não importa esse pacote nem qualquer outro código desta aplicação.

Para Nova, `STS_AGENT_PROMPT_FILE` é obrigatório e deve apontar para um arquivo regular,
não vazio e limitado a 64 KiB.

O serviço exige `STS_AGENT_TOKEN`; Nova exige também `STS_LITELLM_URL` e
`STS_LITELLM_API_KEY`. `STS_PROVIDER=fake` dispensa modelo, prompt e MCP. `STS_AGENT_ADDRESS` usa `127.0.0.1:8090` por padrão e
endereços não loopback são rejeitados enquanto TLS/mTLS não estiver disponível.
