# Personal Assistant Agent

Aplicação dona da conversa semântica: prompt, sessão Nova Realtime,
ferramentas, confirmação, perfis de integração e Host Context.

Nesta fase o gateway usa `inprocess.Factory` como adapter temporário. O binário
independente valida sua configuração e o prompt; o transporte Agent Session
remoto será adicionado na fase seguinte.

`STS_AGENT_PROMPT_FILE` é obrigatório e deve apontar para um arquivo regular,
não vazio e limitado a 64 KiB.
