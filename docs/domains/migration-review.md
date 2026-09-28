# Revisão da migração modular

Data: 2026-09-28. Branch: `codex/modular-domain-migration`.

## Resultado

O repositório passou a ser um monorepo poliglota orientado a aplicações. As
quatro aplicações Go têm módulo, manifesto, Dockerfile, testes e ciclo de build
próprios: `voice-gateway`, `voice-client`, `mcp-notes` e `mcp-agenda`.
`go.work` oferece conveniência local; o agregador executa cada módulo com
`GOWORK=off` para detectar dependências acidentais entre aplicações.

Notes e Agenda agora possuem modelos, adaptadores SQLite, migrations, contratos
e ledgers de idempotência separados. Ao abrir bancos legados, cada aplicação
copia apenas suas operações da antiga tabela `operations` para seu ledger e
preserva as tabelas antigas. Nenhuma remoção automática de dados faz parte do
rollout.

O gateway mantém os módulos de conversação, ações, portas e adaptadores. As
políticas de ferramentas saíram das condições por nome e vivem em perfis
confiáveis em `apps/voice-gateway/config/integrations/`. Perfis adicionais podem
ser carregados por `STS_INTEGRATIONS_DIR`; políticas ausentes, divergentes ou
inválidas falham no carregamento. Annotations remotas não concedem permissão.

## Contratos e extensibilidade

- Notes e Agenda publicam `tools.json`, schema de resultados e exemplos em
  seus próprios `contracts/v1/`.
- O gateway publica voz v1 e o envelope assinado, incluindo vetores positivos
  e negativos para implementações em outras linguagens.
- `runtime.json` descreve descoberta/execução; segredos continuam sendo apenas
  referências a variáveis de ambiente.
- O supervisor descobre integrações embutidas e externas e aceita seleção por
  `--mcps`. Um novo MCP pode ser registrado sem condição específica no núcleo.
- A primeira aplicação Spring foi adiada até existir um domínio real. O guia
  `new-application.md` define o onboarding e os testes necessários.

## Compatibilidade e rollout

Nomes públicos `notes.*` e `agenda.*`, aliases, schemas, protocolo de voz,
confirmação posterior, idempotência e comportamento diante de resultado
desconhecido foram preservados. Os caminhos padrão continuam
`data/sts.sqlite` e `data/agenda.sqlite`. Bancos que por configuração apontem
para o mesmo arquivo passam a usar ledgers separados; as tabelas de negócio
continuam pertencendo a suas aplicações.

A composição LiteLLM mantém os MCPs stdio embutidos por compatibilidade. Cada
app também possui Dockerfile independente, mas isso não ativa transporte MCP
HTTP remoto. Essa troca exige validação específica do LiteLLM antes do rollout.

## Validação

- `make check`: build, testes, race detector, vet e verificação de fronteiras
  em todos os módulos Go.
- `make dev-test`: catálogo, supervisor e metadados LiteLLM.
- `make apple-core-test`: 25 testes, 1 ignorado, sem falhas.
- `python3 harness/run.py --check`: skill vendorizada e checksum válidos.
- Testes adicionais cobrem isolamento de schema, migração de ledger legado,
  contratos publicados, envelope, perfis fail-closed e integração por processos.

O build de `deploy/litellm/Dockerfile` foi repetido em 2026-09-28 e concluído,
gerando a imagem local `sts-litellm:modular-review`. A tentativa anterior havia
parado ao consultar metadados das imagens-base; com Docker Hub e GHCR acessíveis,
os binários de Notes e Agenda foram compilados e copiados para a imagem final.
Nenhum container ou banco em execução foi alterado.
