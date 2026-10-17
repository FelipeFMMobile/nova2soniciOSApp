# Configuração AWS de desenvolvimento — Conta

Solicitação autorizada em 2026-10-06: perfil `default`, conta `111122223333`,
região `us-east-1`, prefixo/tag Project `sts-account-dev` e endereço
`account-dev@example.com` como remetente e destinatário de teste.

Os identificadores abaixo são **exemplos**, não a configuração real do ambiente.
Substitua conta, usuário IAM, e-mail, pool, cliente e domínio pelos seus valores
antes de aplicar políticas/comandos. A configuração efetiva fica somente em
`deploy/account/.env` e `data/account/aws-dev-resources.json`, ignorados pelo Git.

## Estado

O perfil autentica como `arn:aws:iam::111122223333:user/ProvisionerUser`.
O administrador aplicou a política, e as consultas Cognito/SES passaram.
A identidade SES `account-dev@example.com` foi criada e verificada em `us-east-1`:
`VerificationStatus=SUCCESS`, `VerifiedForSendingStatus=true`. Conta SES em
sandbox, envio habilitado. O administrador anexou o complemento de tagging
como customer-managed policy após atingir o limite agregado de políticas inline.

Recursos provisionados e conferidos por API:

- User Pool `us-east-1_EXAMPLE`, plano Essentials, MFA `ON` por e-mail.
- App client público `example-public-client-id`, sem secret, fluxo code/PKCE.
- Domínio `https://sts-account-dev-example.auth.us-east-1.amazoncognito.com`,
  status `ACTIVE`, Managed Login v2 e branding padrão associado ao cliente.
- Callback `stsvoice://oauth/callback` e remetente SES verificado.
- Discovery OIDC e página Managed Login verificados (HTTP 200, título Sign-in).
- Preflight Cognito/SES aprovado; `deploy/account/.env` privado (0600) configurado.

Nenhum usuário foi criado nem convite enviado nesta etapa. Primeiro acesso,
troca da senha temporária e recebimento/validação de MFA ainda precisam de ensaio.

O limite de 2048 caracteres é agregado para todas as políticas inline do usuário.
Criar outra inline não contorna esse limite. Preservar a política original e
criar uma **customer-managed policy** em IAM → Policies → Create policy → JSON,
usando [`aws-tagging-policy.json`](../../deploy/account/aws-tagging-policy.json)
e nome `StsAccountDevTagging`. Depois anexá-la ao usuário ProvisionerUser por
Add permissions → Attach policies directly. O complemento não consome o limite
agregado inline. Alternativamente, a política completa pode ser criada como
customer-managed policy (2002 caracteres compactados, abaixo do limite 6144).

## Permissões a aplicar por administrador

No console IAM **da conta 111122223333**, abrir Users → ProvisionerUser →
Permissions → Add permissions → Create inline policy → JSON. Usar o conteúdo de
[`aws-provisioner-policy.json`](../../deploy/account/aws-provisioner-policy.json)
e salvar com nome `StsAccountDevProvisioner`.

A política limita configurações de pools à tag `Project=sts-account-dev`, SES
ao endereço escolhido e a criação IAM ao service-linked role de e-mail Cognito.
Não concede administração IAM, remoção de recursos ou alteração global SES.
Não foi aplicada pelo agente; o usuário confirmou aplicação por administrador.

## Sequência após a liberação

1. Verificar/criar a identidade SES; o usuário confirma o link na caixa Gmail.
2. Criar User Pool dedicado Essentials, tag Project, senha e e-mail, cadastro
   somente por administrador, recuperação administrativa e remetente SES.
3. Configurar MFA obrigatório por e-mail, app client público Authorization Code
   com PKCE, scopes `openid email aws.cognito.signin.user.admin` e callback
   `stsvoice://oauth/callback`.
4. Criar domínio Managed Login v2 e branding padrão para o app client.
5. Registrar IDs e configurar ambiente local separado do fake, preservando
   volumes/segredos existentes. Semear uma fixture por convite.
6. Ensaiar senha + MFA e retorno do navegador; não certificar pelo simples JWT.

Provisionamento retomável (SDK atualizado necessário):

```sh
data/account/aws-runtime/bin/python scripts/provision_account_aws.py \
  --account 111122223333 --sender account-dev@example.com
```

O script confere conta e remetente, recusa pools sem tag de propriedade e
salva progresso em `data/account/aws-dev-resources.json` (privado, ignorado pelo
Git). Não cria usuários nem imprime credenciais. Após o provisionamento, o
administrador pode remover a concessão temporária CreateUserPool/TagResource.

O AWS CLI global (2.9.16) omite os campos UserPoolTier e EmailMfaConfiguration,
fazendo a validação antiga rejeitar um pool corretamente configurado. Em
2026-10-17 o runtime temporário foi substituído pelo ambiente local ignorado pelo
Git `data/account/aws-runtime`. O supervisor usa automaticamente seu executável
AWS, preservando as ferramentas e credenciais globais. Para recriá-lo:

```sh
python3 -m venv data/account/aws-runtime
data/account/aws-runtime/bin/python -m pip install 'awscli==1.46.1' 'boto3==1.43.108'
```

O preflight agora distingue ausência de campos na resposta de plano/MFA inválidos.

SES sandbox só permite envio a destinatários verificados. O estado de produção
foi consultado: `ProductionAccessEnabled=false`. Solicitação de acesso de produção,
se necessária, deve descrever uso transacional e requer informações adicionais.

Referências: [MFA Cognito](https://docs.aws.amazon.com/cognito/latest/developerguide/user-pool-settings-mfa-sms-email-message.html),
[service-linked role](https://docs.aws.amazon.com/cognito/latest/developerguide/using-service-linked-roles.html),
[tags e IAM](https://docs.aws.amazon.com/cognito/latest/developerguide/tagging.html),
[SES sandbox](https://docs.aws.amazon.com/ses/latest/dg/request-production-access.html).

Para iniciar o ambiente e preparar o convite de desenvolvimento:

```sh
make dev
```

Selecionar a opção 4 (Conta), manter o modo Cognito e perfil `default`, e usar
`account-dev@example.com` como destinatário da fixture. Os identificadores já
estão no arquivo local. A senha temporária e o desafio MFA devem ser preenchidos
pelo usuário na página Cognito, sem compartilhamento no chat.
