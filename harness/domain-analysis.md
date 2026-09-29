# Roteiro: domínios e aplicações

## Objetivo e escopo

Identificar capacidades de negócio e planejar um monorepo poliglota com
monólitos modulares por aplicação. Novos MCPs podem usar Spring ou outra stack.
Esta execução entrega análise e plano; implantação e migração do runtime são
trabalhos posteriores. O usuário pode ampliar esse escopo explicitamente.

## Execução

1. Leia AGENTS.md, os dois perfis e skills/domain-analysis/SKILL.md relativos
   a este harness. Valide source.json com `python3 harness/run.py --check`.
2. Registre HEAD e alterações locais. Leia README, entrypoints, entidades,
   regras, persistência, protocolos, configuração, testes, build e deploy.
   Não leia .env reais, bancos, logs ou credenciais para fazer esse inventário.
3. Extraia conceitos e operações; agrupe por linguagem; classifique subdomínios
   em core/supporting/generic; avalie coesão; identifique problemas; proponha
   bounded contexts. Vincule cada conclusão a arquivo/símbolo verificável.
4. Use notas brutas de 0–10: linguagem 0–3, uso 0–3, dados 0–2, mudança 0–2.
   A fórmula upstream divide por 10 mas os intervalos usam 0–10; mantenha a
   cópia original e explicite esta interpretação no relatório. Sem histórico
   analisado, marque mudança como desconhecida e mostre intervalo possível.
5. Projete aplicações, módulos internos, propriedade de dados, contratos e
   dependências permitidas. Diferencie biblioteca técnica, módulo e processo.
   Contratos e testes de conformidade pertencem à aplicação fornecedora;
   perfis e testes de autorização pertencem à aplicação agêntica, com configuração em
   `apps/personal-assistant-agent/config/integrations/` e segredos via ambiente. A raiz
   mantém apenas testes de integração entre aplicações, sem pastas centrais
   de contratos ou perfis. Não transforme cada pacote técnico em domínio ou
   cada domínio em serviço.
6. Produza roadmap incremental com origem/destino, ordem, testes, rollback,
   riscos, compatibilidade e critérios de aceite. Não pressuponha suporte
   HTTP MCP remoto ou compatibilidade Go/Java sem testes de contrato.
7. Revise se o plano permite uma aplicação nova sem editar regras do host.
   Registre hipóteses de produto que ainda precisam de validação.

## Saídas

- `docs/domains/analysis.md`: inventário, linguagem, classificações, coesão,
  problemas priorizados e mapa de contextos.
- `docs/domains/migration-plan.md`: decisão arquitetural, estrutura e fases.
- `docs/domains/run-AAAA-MM-DD.md`: revisão, evidências, execução e limites.

Critérios de saída: todo contexto tem dono de dados, integração explícita e
justificativa de negócio; todo problema alto possui fase corretiva; o plano
mantém as garantias existentes e contempla builds independentes Go/Spring.
