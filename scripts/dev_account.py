#!/usr/bin/env python3
"""Opt-in Account supervisor. Existing Cognito resources only; no global mutations."""
from __future__ import annotations
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
ENV_FILE = ROOT / "deploy/account/.env"
FIXTURES = ROOT / "data/account/fixtures.json"
FIXTURE_NAMES = {"alice": "Alice Exemplo", "bruno": "Bruno Exemplo", "carla": "Carla Exemplo"}
SECRET_KEYS = ("STS_IDENTITY_DB_PASSWORD", "STS_PROFILE_DB_PASSWORD", "STS_USAGE_DB_PASSWORD",
               "STS_IDENTITY_SERVICE_TOKEN", "STS_PROFILE_SERVICE_TOKEN", "STS_ACCOUNT_CONTEXT_SECRET",
               "STS_LEGACY_CONTEXT_SECRET", "STS_CLIENT_CODE_KEY", "STS_TOKEN_ENCRYPTION_KEY", "LITELLM_MASTER_KEY")
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))

def load_settings(path=ENV_FILE):
    if not path.exists():
        return {}
    settings = {}
    for line in path.read_text().splitlines():
        if line and not line.startswith("#"):
            key, sep, value = line.partition("=")
            if not sep or not key.replace("_", "").isalnum():
                raise RuntimeError("Configuração Conta inválida.")
            settings[key] = value
    return settings

def save_settings(values, path=ENV_FILE):
    path.parent.mkdir(parents=True, exist_ok=True)
    if any("\n" in str(v) or "\r" in str(v) for v in values.values()):
        raise RuntimeError("Configuração contém quebra de linha.")
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    os.fchmod(fd, 0o600)
    with os.fdopen(fd, "w") as handle:
        handle.write("".join(f"{k}={v}\n" for k, v in sorted(values.items())))

def json_request(base, path, token, body=None):
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(base + path, data=data, headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    try:
        with OPENER.open(request, timeout=20) as response:
            return json.load(response)
    except (OSError, ValueError) as error:
        raise RuntimeError("Serviço rejeitou a operação. Nenhum payload ou segredo foi registrado.") from None

def wait_http(url, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with OPENER.open(url, timeout=2) as response:
                if response.status == 200:
                    return
        except OSError:
            pass
        time.sleep(0.5)
    raise RuntimeError("Serviço Conta não ficou disponível no prazo; confira o log do serviço.")

def aws_json(profile, region, service, operation, *arguments, missing_ok=False):
    local_cli = ROOT / "data/account/aws-runtime/bin/aws"
    executable = str(local_cli) if local_cli.is_file() else "aws"
    # AWS_PAGER works with CLI v2 and is harmless with an isolated, current CLI v1.
    result = subprocess.run([executable, "--profile", profile, "--region", region, service, operation, *arguments, "--output", "json"],
                            capture_output=True, text=True, env=dict(os.environ, AWS_PAGER=""))
    if result.returncode:
        if missing_ok and "UserNotFoundException" in result.stderr:
            return None
        raise RuntimeError(f"AWS {service}/{operation} falhou. Verifique o perfil e as permissões; detalhes sensíveis omitidos.")
    return json.loads(result.stdout or "{}")

def preflight(settings, profile, emails):
    region, pool, client = (settings[k] for k in ("STS_COGNITO_REGION", "STS_COGNITO_USER_POOL_ID", "STS_COGNITO_CLIENT_ID"))
    call = lambda operation, *args: aws_json(profile, region, "cognito-idp", operation, *args)
    user_pool = call("describe-user-pool", "--user-pool-id", pool)["UserPool"]
    mfa = call("get-user-pool-mfa-config", "--user-pool-id", pool)
    app = call("describe-user-pool-client", "--user-pool-id", pool, "--client-id", client)["UserPoolClient"]
    if "UserPoolTier" not in user_pool:
        raise RuntimeError("AWS CLI não retornou UserPoolTier; versões antigas omitem campos Cognito. "
                           "Instale o runtime atualizado conforme docs/domains/account-aws-setup.md e tente novamente. "
                           "Isso não comprova configuração inválida do pool.")
    if user_pool.get("UserPoolTier") not in {"ESSENTIALS", "PLUS"} or mfa.get("MfaConfiguration") != "ON" or not mfa.get("EmailMfaConfiguration"):
        raise RuntimeError("Use Cognito Essentials/Plus com MFA obrigatório e EmailMfaConfiguration. O supervisor não altera o pool.")
    if app.get("ClientSecret") or "code" not in app.get("AllowedOAuthFlows", []) or not app.get("AllowedOAuthFlowsUserPoolClient"):
        raise RuntimeError("Use app client público com Authorization Code habilitado.")
    if "stsvoice://oauth/callback" not in app.get("CallbackURLs", []) or not {"openid", "email", "aws.cognito.signin.user.admin"} <= set(app.get("AllowedOAuthScopes", [])):
        raise RuntimeError("Configure callback stsvoice://oauth/callback e scopes openid, email, aws.cognito.signin.user.admin.")
    if set(app.get("SupportedIdentityProviders", ["COGNITO"])) != {"COGNITO"}:
        raise RuntimeError("O app client desta POC deve aceitar somente COGNITO; federação pode ter outra política MFA.")
    email = user_pool.get("EmailConfiguration", {})
    if email.get("EmailSendingAccount") != "DEVELOPER" or not email.get("SourceArn"):
        raise RuntimeError("Configure o remetente SES existente no Cognito.")
    arn = email["SourceArn"].split(":", 5)
    ses_region, sender = arn[3], arn[5].removeprefix("identity/")
    source = aws_json(profile, ses_region, "sesv2", "get-email-identity", "--email-identity", sender)
    if not source.get("VerifiedForSendingStatus"):
        raise RuntimeError("Remetente SES ainda não verificado.")
    ses = aws_json(profile, ses_region, "sesv2", "get-account")
    if not ses.get("ProductionAccessEnabled"):
        for destination in emails:
            verified = aws_json(profile, ses_region, "sesv2", "get-email-identity", "--email-identity", destination)
            if not verified.get("VerifiedForSendingStatus"):
                raise RuntimeError("No sandbox SES, verifique cada destinatário antes de executar o seed.")
    expected = f"https://cognito-idp.{region}.amazonaws.com/{pool}"
    if settings["STS_COGNITO_ISSUER"] != expected:
        raise RuntimeError("Issuer Cognito não corresponde à região e pool.")
    domain = settings["STS_COGNITO_DOMAIN"]
    parsed = urllib.parse.urlparse(domain)
    if parsed.scheme != "https" or not parsed.hostname or parsed.path not in ("", "/"):
        raise RuntimeError("Domínio Cognito deve ser uma origem HTTPS.")
    hostname = parsed.hostname
    described = call("describe-user-pool-domain", "--domain", hostname.split(".auth.")[0] if ".auth." in hostname else hostname)["DomainDescription"]
    if described.get("UserPoolId") != pool:
        raise RuntimeError("Domínio não pertence ao User Pool escolhido.")

def seed(settings, profile, emails, identity_url, profile_url):
    records = json.loads(FIXTURES.read_text()) if FIXTURES.exists() else {}
    fake = settings["STS_IDENTITY_PROVIDER"] == "fake"
    chosen = list(FIXTURE_NAMES) if fake else list(FIXTURE_NAMES)[:len(emails)]
    for index, fixture in enumerate(chosen):
        subject, email = fixture, fixture + "@example.invalid"
        if not fake:
            region, pool = settings["STS_COGNITO_REGION"], settings["STS_COGNITO_USER_POOL_ID"]
            email = emails[index]
            username = "sts-fixture-" + settings["STS_FIXTURE_NAMESPACE"] + "-" + fixture
            record_key = region + "/" + pool + "/" + username
            user = aws_json(profile, region, "cognito-idp", "admin-get-user", "--user-pool-id", pool, "--username", username, missing_ok=True)
            if user is not None and record_key not in records:
                raise RuntimeError("Usuário já existe sem registro local de fixture. Nenhuma conta existente será assumida ou sobrescrita.")
            if user is None:
                response = aws_json(profile, region, "cognito-idp", "admin-create-user", "--user-pool-id", pool, "--username", username,
                                    "--user-attributes", json.dumps([{"Name": "email", "Value": email}]), "--desired-delivery-mediums", "EMAIL")
                attrs = {v["Name"]: v["Value"] for v in response["User"]["Attributes"]}
                records[record_key] = {"sub": attrs["sub"], "email": email}
                FIXTURES.parent.mkdir(parents=True, exist_ok=True)
                fd = os.open(FIXTURES, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
                with os.fdopen(fd, "w") as handle:
                    json.dump(records, handle)
                user = {"UserAttributes": response["User"]["Attributes"]}
            attrs = {v["Name"]: v["Value"] for v in user["UserAttributes"]}
            if attrs.get("sub") != records[record_key]["sub"] or attrs.get("email") != email:
                raise RuntimeError("Vínculo de fixture divergiu. Dados existentes preservados.")
            subject = attrs["sub"]
            aws_json(profile, region, "cognito-idp", "admin-set-user-mfa-preference", "--user-pool-id", pool, "--username", username,
                     "--email-mfa-settings", "Enabled=true,PreferredMfa=true")
        user = json_request(identity_url, "/internal/fixtures", settings["STS_IDENTITY_SERVICE_TOKEN"], {"fixture": fixture, "subject": subject, "email": email})
        json_request(profile_url, "/internal/fixtures", settings["STS_PROFILE_SERVICE_TOKEN"], {"client_code": user["client_code"], "name": FIXTURE_NAMES[fixture]})
    print(f"Fixtures prontas: {len(chosen)}. Perfis existentes preservados.")

def service_key(base, master, alias, servers):
    body = {"key_alias": alias + "-" + secrets.token_hex(4), "duration": "24h", "models": ["nova-sonic"],
            "allowed_routes": ["/v1/realtime", "mcp_routes"],
            "object_permission": {"mcp_servers": list(servers), "mcp_tool_permissions": servers},
            "metadata": {"account_policy_required": alias == "sts-account"}}
    return json_request(base, "/key/generate", master, body)["key"]

def interactive(args, nova=None):
    import dev
    if nova is None:
        nova = dev.choose("Conta: qual ambiente?", ["Fake — login simulado local", "Nova — Cognito/SES e voz AWS"], 1) == 2
    settings = load_settings()
    desired = "cognito" if nova else "fake"
    if settings.get("STS_IDENTITY_PROVIDER") not in (None, desired):
        raise RuntimeError("Use ambientes separados para fake e Cognito: preserve o .env e volumes atuais antes de trocar o provedor.")
    settings["STS_IDENTITY_PROVIDER"] = desired
    profile = "default"
    emails = []
    if nova:
        profile = input("Perfil AWS existente [default]: ").strip() or "default"
        for key, label, default in [("STS_COGNITO_REGION", "Região Cognito", "us-east-1"), ("STS_COGNITO_USER_POOL_ID", "User Pool existente", ""), ("STS_COGNITO_CLIENT_ID", "App client existente", ""), ("STS_COGNITO_DOMAIN", "Domínio HTTPS do Managed Login", "")]:
            current = settings.get(key, default)
            settings[key] = input(f"{label} [{current}]: ").strip() or current
            if not settings[key]:
                raise RuntimeError("Preencha os identificadores Cognito existentes.")
        settings["STS_COGNITO_DOMAIN"] = settings["STS_COGNITO_DOMAIN"].rstrip("/")
        settings["STS_COGNITO_ISSUER"] = f"https://cognito-idp.{settings['STS_COGNITO_REGION']}.amazonaws.com/{settings['STS_COGNITO_USER_POOL_ID']}"
        emails = [v.strip() for v in input("De 1 a 3 e-mails de teste para MFA, separados por vírgula: ").split(",") if v.strip()]
        if not 1 <= len(emails) <= 3 or len(set(emails)) != len(emails) or any("@" not in e or any(c.isspace() for c in e) for e in emails):
            raise RuntimeError("Informe de 1 a 3 e-mails distintos.")
    lan = dev.choose("Onde o app Conta vai rodar?", ["Mac / Simulator — loopback", "iPhone físico — HTTPS/WSS obrigatório"], 1) == 2
    cert = key = ""
    if lan:
        cert = input("Caminho do certificado TLS confiável pelo iPhone: ").strip()
        key = input("Caminho da chave TLS: ").strip()
        if not Path(cert).is_file() or not Path(key).is_file():
            raise RuntimeError("Certificado e chave TLS são obrigatórios.")
    token = dev.getpass.getpass("Token local do gateway [gerar aleatório]: ") if dev.sys.stdin.isatty() else input("Token local do gateway [gerar aleatório]: ")
    token = token or secrets.token_urlsafe(32)
    if any(ord(c) <= 32 or ord(c) >= 127 for c in token) or len(token) > 4096:
        raise RuntimeError("Token local inválido.")
    print(f"Conta + Agenda/notas; MFA={desired}; gateway={'TLS' if lan else 'loopback'}; bancos independentes.")
    if args.dry_run:
        print("Dry-run: nenhum arquivo, recurso AWS, fixture, build ou serviço alterado.")
        return 0
    if not dev.yes("Iniciar este ambiente e popular as fixtures selecionadas?"):
        return 0
    if nova:
        preflight(settings, profile, emails)
    for name in SECRET_KEYS:
        settings.setdefault(name, ("sk-" if name == "LITELLM_MASTER_KEY" else "") + secrets.token_hex(32))
    settings.setdefault("STS_FIXTURE_NAMESPACE", secrets.token_hex(6))
    save_settings(settings)
    run(args, settings, profile, emails, nova, lan, token, cert, key)
    return 0

def run(args, settings, profile, emails, nova, lan, token, cert, key):
    import dev
    account_port = args.agent_port + 1
    if account_port > 65535 or account_port == args.gateway_port:
        raise RuntimeError("Portas dos agentes e gateway precisam ser distintas.")
    dev.ensure_ports([(args.gateway_port, "0.0.0.0" if lan else "127.0.0.1"), (args.agent_port, "127.0.0.1"), (account_port, "127.0.0.1")])
    compose = ["docker", "compose", "-p", "sts-account-dev", "--env-file", str(ENV_FILE), "-f", str(ROOT / "deploy/account/compose.yaml")]
    if nova:
        compose += ["-f", str(ROOT / "deploy/account/compose.nova.yaml")]
    # Only Docker receives AWS profile settings. Go processes inherit no AWS credentials.
    compose_env = dict(os.environ, **settings, AWS_PROFILE=profile)
    dev.checked(compose + ["up", "--build", "-d"], compose_env)
    identity_url = "http://127.0.0.1:" + settings.get("STS_IDENTITY_PORT", "8092")
    profile_url = "http://127.0.0.1:" + settings.get("STS_PROFILE_PORT", "8093")
    litellm_url = "http://127.0.0.1:" + settings.get("STS_ACCOUNT_LITELLM_PORT", "4001")
    for url in [identity_url + "/healthz", profile_url + "/healthz", litellm_url + "/health/liveliness"]:
        wait_http(url)
    seed(settings, profile, emails, identity_url, profile_url)
    account_key = service_key(litellm_url, settings["LITELLM_MASTER_KEY"], "sts-account", {"sts-identity": ["identity.begin_login", "identity.login_status", "identity.logout"], "sts-profile": ["profile.get_me", "profile.update_me"]})
    personal_servers = dev.servers(4, False)
    personal_key = service_key(litellm_url, settings["LITELLM_MASTER_KEY"], "sts-personal", {s["server_id"]: s["allowed_tools"] for s in personal_servers})
    env = dev.environment("nova" if nova else "fake", profile, lan, token, personal_servers, args.gateway_port)
    env.update(STS_AGENT_ADDRESS=f"127.0.0.1:{args.agent_port}", STS_MCP_BACKEND="litellm", STS_LITELLM_URL=litellm_url,
               STS_LITELLM_API_KEY=personal_key, STS_MCP_CONTEXT_SECRET=settings["STS_LEGACY_CONTEXT_SECRET"], STS_LITELLM_MCP_SERVERS=json.dumps(dev.gateway_servers(personal_servers)))
    edge, personal = dev.runtime_environments(env)
    account_token = secrets.token_hex(32)
    account = {k: v for k, v in personal.items() if not k.startswith("STS_")}
    account.update(STS_PROVIDER=env["STS_PROVIDER"], STS_AGENT_ADDRESS=f"127.0.0.1:{account_port}", STS_AGENT_TOKEN=account_token,
                   STS_AGENT_PROMPT_FILE=str(ROOT / "apps/account-agent/prompts/system.md"), STS_LITELLM_URL=litellm_url,
                   STS_LITELLM_API_KEY=account_key, STS_MCP_CONTEXT_SECRET=settings["STS_ACCOUNT_CONTEXT_SECRET"])
    catalog = json.loads(edge["STS_AGENT_CATALOG"])
    catalog.append({"id": "account", "endpoint": f"ws://127.0.0.1:{account_port}/v2/session", "protocol": "agent-session/v2", "token_env": "STS_AGENT_TOKEN_ACCOUNT"})
    edge.update(STS_AGENT_CATALOG=json.dumps(catalog), STS_AGENT_TOKEN_ACCOUNT=account_token,
                STS_IDENTITY_URL=identity_url, STS_IDENTITY_SERVICE_TOKEN=settings["STS_IDENTITY_SERVICE_TOKEN"])
    if lan:
        edge.update(STS_TLS_CERT=cert, STS_TLS_KEY=key)
    for name, command in [("gateway", "./apps/voice-gateway/cmd/gateway"), ("personal-assistant-agent", "./apps/personal-assistant-agent/cmd/agent"), ("account-agent", "./apps/account-agent/cmd/account-agent")]:
        dev.checked(["go", "build", "-o", str(ROOT / "bin" / name), command], env)
    logs = ROOT / "logs/dev"; logs.mkdir(parents=True, exist_ok=True); logs.chmod(0o700)
    children, handles = [], []
    try:
        for name, child_env, port in [("personal-assistant-agent", personal, args.agent_port), ("account-agent", account, account_port), ("gateway", edge, args.gateway_port)]:
            path = logs / f"{name}-{time.time_ns()}.log"; handle = path.open("x"); path.chmod(0o600); handles.append(handle)
            children.append(subprocess.Popen([str(ROOT / "bin" / name)], cwd=ROOT, env=child_env, stdin=subprocess.DEVNULL, stdout=handle, stderr=handle, start_new_session=True))
            if not (name == "gateway" and lan):
                dev.wait_ready(children, port)
            print(f"{name} pronto; log: {path}")
        print(f"App: {'wss://HOST-DO-CERTIFICADO' if lan else 'ws://127.0.0.1'}:{args.gateway_port}/v1/voice")
        print(f"Provider: {env['STS_PROVIDER']}; token local configurado (não exibido).")
        print("Selecione Conta no app. Ctrl+C encerra os agentes; os bancos são preservados.")
        while all(child.poll() is None for child in children):
            time.sleep(0.3)
        raise RuntimeError("Serviço Conta encerrou; consulte logs/dev.")
    finally:
        dev.stop_children(children)
        for handle in handles:
            handle.close()
