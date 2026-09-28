# Harness de arquitetura

Harness local, versionado e independente de fornecedor de agente. Reúne a
skill, os papéis e o roteiro para uma execução assistida por um agente de IA.
Não instala plugins globais, não chama uma API de LLM e não inicia agentes sozinho.

## Executar

1. Na raiz do repo, execute `python3 harness/run.py --check` para validar a cópia.
2. Execute `python3 harness/run.py` para emitir o contexto (roteiro, perfis e skill).
3. Peça ao agente: “Execute `harness/domain-analysis.md` neste checkout usando
   a skill local e atualize a análise e o plano em `docs/domains/`”.
4. O agente deve ler os arquivos de produção e testes referenciados, aplicar as
   seis fases e registrar evidências, hipóteses e validação. Emitir contexto
   por si só não executa análise semântica.

Primeira execução: [análise](../docs/domains/analysis.md),
[plano](../docs/domains/migration-plan.md),
[registro](../docs/domains/run-2026-09-27.md).

## Conteúdo

- `domain-analysis.md`: entradas, sequência e critérios de saída.
- `agents/domain-analyst.md`: identificação e linguagem de negócio.
- `agents/modular-architect.md`: limites, contratos e migração.
- `skills/domain-analysis/SKILL.md`: cópia integral e sem alterações da skill
  de Tech Leads Club solicitada pelo usuário.
- `skills/domain-analysis/source.json`: origem, revisão e SHA-256.
- `skills/domain-analysis/LICENSE.upstream`: licença original e nota sobre
  CC-BY-4.0 para o conteúdo das skills.

Atribuição: [Tech Leads Club — domain-analysis](https://agent-skills.techleads.club/skills/domain-analysis/).
Conteúdo da skill licenciado sob [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
Ao atualizar, fixe uma revisão upstream, revise as instruções, preserve a
atribuição e atualize checksum e registro. Não atualize automaticamente em runtime.
