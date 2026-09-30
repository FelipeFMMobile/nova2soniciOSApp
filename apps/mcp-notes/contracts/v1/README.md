# Contrato MCP v1

`tools.json` é a fonte dos nomes, descrições e schemas de entrada anunciados
pelo servidor. `results.schema.json` descreve o JSON dentro de `content[].text`
do resultado MCP; `examples.json` contém exemplos fictícios. `isError` sinaliza
falha. Nomes públicos e formatos permanecem compatíveis com a POC.

Cada mutação exige chave de idempotência fornecida pelo host, com replay
atômico de resultado; chave reutilizada com outros argumentos é conflito.
O modo local usa `_meta`; o modo LiteLLM exige envelope assinado conforme o
contrato do agente em `../../../personal-assistant-agent/contracts/host-context/v1/`
(caminho a partir da raiz desta aplicação). Não aceitar confirmação como
argumento de negócio. Exclusão/cancelamento exige autorização do host.

Não repetir automaticamente uma mutação de resultado desconhecido. Erros de
negócio são retornados como resultado MCP, não como sucesso presumido.
