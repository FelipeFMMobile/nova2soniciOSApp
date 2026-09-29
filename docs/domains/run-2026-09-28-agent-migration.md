# Registro — análise da separação gateway/agente

Data: 2026-09-28.

## Revisão e checkout

- Base: `v0.7.0` / `bd593ce4a73551da054bd98efec2bfdec63eb795`.
- Worktree: `/Users/felipemenezes/.codex/worktrees/agent-migration-v1/StsModel`.
- Branch: `codex/agent-migration-v1`.
- Plano de entrada: `/private/tmp/sts-agent-migration-plan.md`.
- Harness: `python3 harness/run.py --check` passou antes da análise.

## Método e evidências

Foram lidos `AGENTS.md`, o roteiro, os dois perfis e a skill local completa.
A análise confrontou o plano com os contratos Voice/Host Context, os manifests,
o adaptador público, `runNova`, a porta de provider, `actions.Session`, a
configuração do gateway, o supervisor e os documentos da migração `v0.7.0`.
Não foram lidos `.env`, bancos, logs ou credenciais.

Conclusão principal: a fronteira proposta é coerente, mas a primeira porta deve
representar a sessão semântica contínua. O caminho Nova real não usa a abstração
de turno do fake, portanto extrair apenas `SendAudio/Commit/Cancel/Events` sem
contrato de abertura, confirmação e lifecycle deixaria responsabilidades no
gateway ou criaria duas implementações divergentes.

## Interpretação da coesão

A skill traz fórmula dividida por 10 e faixas em 0–10; foi usada a soma bruta.
Sem histórico de coalteração analisado, mudança permanece desconhecida.

| Grupo | L | U | D | M | Total | Leitura |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Semântica do assistente | 3 | 3 | 2 | ? | 8–10 | prompt, turnos, Realtime e actions mudam como uma capacidade |
| Transporte/roteamento de voz | 3 | 3 | 1 | ? | 7–9 | autenticação, buffers, sessões e catálogo são coesos |
| Gateway + semântica após a porta | 1 | 1 | 0 | ? | 2–4 | integração necessária, mas não justificativa para o mesmo processo |

## Limites

Esta execução atualizou somente documentação e versionamento do trabalho. Não
criou a aplicação futura, não alterou runtime, contratos ou deploy e não marcou
a migração como concluída.

Validação executada na worktree: `git diff --check`,
`python3 harness/run.py --check` e `make check` (build, testes, race detector,
vet e verificação de fronteiras de todos os módulos) passaram. Testes opt-in
com Docker/AWS não foram executados porque o runtime não mudou.
