# Contexto do host v1

Contrato publicado pelo gateway; a implementação Go está em `mcpwire`.
`vectors.json` contém segredos fictícios e relógio fixo para testes de
interoperabilidade. Não são credenciais de nenhum ambiente.

O envelope tem `payload` (objeto de argumentos de negócio) e `host_context`.
O contexto contém, nesta ordem de serialização para a assinatura: `version`,
`audience`, `tool`, `payload_hash`, `idempotency_key`, `confirmed`, `expires_at`,
`signature`. Na assinatura, `signature` é a string vazia. A entrada do HMAC é
`sts-mcp-envelope-v1` seguida de um byte zero e desse JSON UTF-8 compacto.
HMAC-SHA256 e SHA256 usam hexadecimal minúsculo; a chave é de 32 bytes.

`payload_hash` usa o objeto JSON reserializado pelo encoding/json do Go, com
chaves de mapas ordenadas, UseNumber (sem conversão a float64), escaping padrão
incluindo HTML e U+2028/U+2029. A representação textual de números é preservada.
Isso não é uma afirmação de conformidade com RFC 8785/JCS. Outras linguagens
precisam reproduzir esses bytes; não assinar diretamente a saída padrão de
Jackson. Campos desconhecidos no envelope/contexto e dados JSON após o objeto
são rejeitados. O payload segue o schema da ferramenta.

O receptor verifica versão 1, audience, nome da ferramenta, hash, assinatura,
chave de idempotência não vazia até 256 bytes e validade temporal. expires_at
é Unix em segundos, estritamente futuro e no máximo 60 segundos à frente.
Payload canônico é limitado a 16 KiB na entrada; envelope, a 32 KiB.

Mutação e ledger são responsabilidade da aplicação executora. Confirmação é
uma decisão do host vinculada à ferramenta/alvo; o modelo não fornece esses
campos. No modo envelope, `_meta` não é fallback de autorização. O modo local
`_meta` só se aplica a processos confiáveis lançados pelo host.

Uma mudança incompatível de canonicalização exige nova versão e coexistência;
a migração atual preserva o algoritmo existente. A suíte Go verifica os vetores
publicados e rejeita adulteração, audience/tool incorretos e expiração.
