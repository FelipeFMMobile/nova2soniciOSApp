# Integração Nova via LiteLLM

`STS_PROVIDER=nova` abre uma sessão em
`$STS_LITELLM_URL/v1/realtime?model=$STS_LITELLM_REALTIME_MODEL` usando
`STS_LITELLM_API_KEY`. Não existe bridge Python nem acesso direto do Go ao
Bedrock.

O `session.update` configura prompt PT-BR, voz `carolina`, PCM16 16 kHz de
entrada, PCM16 24 kHz de saída, VAD e function tools. O adaptador converte
áudio, transcrições, interrupções, texto, tool calls, resultados e conclusão
para os eventos internos já consumidos pelo gateway.

As sessões renovam antes do limite do Nova. Até 32 mensagens/64 KiB de
histórico textual finalizado são incorporados às instruções da nova sessão;
áudio, conteúdo especulativo e turnos cancelados não são reconstruídos.

Falha de handshake, timeout de leitura, erro Realtime e fechamento anormal são
falhas do provider. Não existe retry automático de mutações com resultado
desconhecido e não existe fallback para AWS.

Testes de unidade usam um WebSocket falso. O smoke real e a validação de custo
estão em [nova-litellm-runbook.md](nova-litellm-runbook.md).
