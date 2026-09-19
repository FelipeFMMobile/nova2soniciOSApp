# LiteLLM como gateway MCP local

Branch: `codex/litellm-mcp-gateway`. LiteLLM 1.101.0 e PostgreSQL rodam
localmente em Docker, expostos somente em `127.0.0.1:4000`. O painel fica em
<http://127.0.0.1:4000/ui> e usa a chave administrativa.

```text
Cliente de voz -> gateway Go -> ponte Python -> Nova Sonic / Bedrock
                       |
                       `-> LiteLLM MCP REST -> Notes (stdio) -> SQLite
                                            `-> Agenda (stdio) -> SQLite
```

O LiteLLM centraliza autenticação, permissões por servidor/ferramenta e limites.
O Go continua dono de schema, intenção, confirmação posterior, identidade de
operação e auditoria. Notes e Agenda continuam donos das regras transacionais e
do replay durável. Chamadas Nova não passam pelo LiteLLM nesta etapa.

## Compatibilidade de contexto

LiteLLM 1.101.0 encaminha argumentos de ferramentas, mas não preserva `_meta`
arbitrário até o SDK MCP upstream. A verificação reproduzível permanece em
`scripts/check_litellm_metadata.py`.

Os MCPs locais agora oferecem `-context-mode envelope`. Nesse modo, a descoberta
expõe um envelope em `arguments`, mas o backend Go remove esse envelope antes de
anunciar a ferramenta ao Nova. Depois da decisão do host, o Go envia:

```json
{
  "payload": {"argumentos": "originais"},
  "host_context": {
    "version": 1,
    "audience": "memo",
    "tool": "notes.create",
    "payload_hash": "...",
    "idempotency_key": "...",
    "confirmed": false,
    "expires_at": 1800000060,
    "signature": "..."
  }
}
```

A assinatura HMAC-SHA256 cobre servidor, ferramenta, hash canônico do payload,
chave de operação, confirmação e validade de até 60 segundos. O MCP rejeita
alteração, expiração, replay entre ferramentas/servidores e chamadas sem
assinatura. O segredo deve ter exatamente 32 bytes em hexadecimal e nunca é
enviado ao modelo. O modo legado `_meta` continua disponível para o backend
stdio existente.

## Inicialização

O fluxo normal é `make dev`. O assistente inicia os containers automaticamente;
se eles já estiverem ativos, pergunta se devem ser reiniciados. A resposta
negativa preserva um ambiente saudável. `Ctrl+C` encerra gateway e ponte, mas
mantém LiteLLM e PostgreSQL como serviços locais.

`deploy/litellm/.env` contém chaves fictícias e fixas para esta POC local. Elas
não podem ser reutilizadas em rede compartilhada, homologação ou produção.

Para iniciar somente a camada Docker manualmente:

```bash
make litellm-up
```

### Chave de serviço no fluxo manual

O LiteLLM usa duas credenciais diferentes. `LITELLM_MASTER_KEY` é a chave
administrativa: ela permite acessar o painel e criar ou revogar outras chaves.
O gateway Go não deve receber essa credencial. Ele usa uma chave de serviço,
armazenada em `STS_LITELLM_API_KEY`, limitada aos MCPs e ferramentas necessários.

Primeiro carregue a configuração local do compose no terminal. Depois use a
chave administrativa somente para solicitar a chave de serviço:

```bash
set -a
source deploy/litellm/.env
set +a

python3 scripts/litellm_service_key.py \
  --master-key "$LITELLM_MASTER_KEY" > /private/tmp/sts-litellm-exports
source /private/tmp/sts-litellm-exports

export STS_MCP_CONTEXT_SECRET='<o mesmo segredo do compose>'
export STS_PROVIDER=fake
export STS_DEVELOPMENT_TOKEN=local
make gateway
```

`set -a` exporta para os comandos seguintes as variáveis lidas do arquivo
`.env`. O script chama a API administrativa `/key/generate` do LiteLLM e imprime
comandos de shell semelhantes a:

```bash
export STS_MCP_BACKEND=litellm
export STS_LITELLM_MCP_URL=http://127.0.0.1:4000
export STS_LITELLM_API_KEY=sk-chave-gerada-pelo-litellm
export STS_LITELLM_MCP_SERVERS='[...]'
```

O operador `>` grava a saída no arquivo temporário sem mostrar a chave no
terminal. O comando `source /private/tmp/sts-litellm-exports` aplica esses
`export` ao shell atual. A partir daí, o gateway conhece o endereço do LiteLLM,
sua chave de serviço e os servidores e ferramentas autorizados.

`STS_MCP_CONTEXT_SECRET` tem uma finalidade diferente: ele assina e valida o
`host_context` transportado nos argumentos MCP. Ele não autentica o gateway no
LiteLLM e precisa ter o mesmo valor no gateway Go e nos containers MCP.

O `make dev` cria ou atualiza a chave de serviço local fixa, limitando-a aos
servidores escolhidos sem acumular novas credenciais a cada execução.
O script manual concede os dois servidores e as sete ferramentas, com limite de
60 chamadas/minuto por servidor. `allow_all_keys` permanece desabilitado. Não
use a chave administrativa no gateway Go.

`general_settings.disable_spend_logs` fica habilitado porque os registros MCP
do LiteLLM incluem argumentos e resultados. Assim, o painel oferece saúde,
cadastro e administração, mas o histórico operacional sem conteúdo permanece
na auditoria SQLite do Go. Habilitar a tela de logs de chamadas sem antes criar
uma integração de redação armazenaria conteúdo de notas e agenda.

Para encerrar os containers sem apagar os volumes:

```bash
make litellm-down
```

## Configuração do Go

- `STS_MCP_BACKEND=litellm`
- `STS_LITELLM_MCP_URL=http://127.0.0.1:4000`
- `STS_LITELLM_API_KEY`: chave de serviço
- `STS_MCP_CONTEXT_SECRET`: segredo compartilhado
- `STS_LITELLM_MCP_SERVERS`: aliases, IDs estáveis, allowlists e políticas

Esse modo é incompatível com `STS_MCP_COMMAND` e `STS_MCP_SERVERS`; combinações
ambíguas falham na inicialização. Falhas do LiteLLM não causam retry automático
de mutações nem fallback direto para stdio.

## Validação local — 19/09/2026

- LiteLLM 1.101.0 e PostgreSQL iniciaram saudáveis; painel `/ui/` respondeu 200.
- A chave de serviço descobriu exatamente as sete ferramentas permitidas; o
  limite `memo=60`, `local=60` foi persistido nos metadados da chave.
- Notes atravessou Go client → LiteLLM REST → MCP stdio → SQLite: criação,
  replay com a mesma identidade, bloqueio sem confirmação e exclusão confirmada.
- Uma chamada REST direta com argumentos sem assinatura retornou resultado MCP
  de erro; chave inválida retornou HTTP 401.
- Os payloads sintéticos não apareceram nos logs do container com spend logs
  desativados.
- `make check` passou (build, testes, race detector e vet); os 19 testes Python
  aplicáveis passaram, com um teste e2e opt-in não executado.

Fontes: [MCP gateway](https://docs.litellm.ai/docs/mcp),
[permissões e limites](https://docs.litellm.ai/docs/mcp_control),
[especificação `_meta`](https://modelcontextprotocol.io/specification/2025-11-25/basic#meta).
