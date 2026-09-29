#!/usr/bin/env python3
"""Interactive local supervisor. No AWS resource provisioning or credential storage."""
from __future__ import annotations

import argparse
import getpass
import json
import os
from pathlib import Path
import shutil
import secrets
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
LITELLM_ENV = ROOT / "deploy/litellm/.env"
LITELLM_COMPOSE = ROOT / "deploy/litellm/compose.yaml"
LITELLM_URL = "http://127.0.0.1:4000"
LITELLM_SERVICES = {"postgres", "litellm"}


def choose(question: str, options: list[str], default: int) -> int:
    print(f"\n{question}")
    for index, option in enumerate(options, 1):
        print(f"  {index}. {option}")
    while True:
        answer = input(f"Escolha [{default}]: ").strip() or str(default)
        if answer.isdigit() and 1 <= int(answer) <= len(options):
            return int(answer)
        print("Escolha um número da lista.")


def yes(question: str, default: bool = False) -> bool:
    while True:
        answer = input(f"{question} [{'S/n' if default else 's/N'}]: ").strip().lower()
        if not answer:
            return default
        if answer in ("s", "sim", "y", "yes"):
            return True
        if answer in ("n", "não", "nao", "no"):
            return False
        print("Responda s ou n.")


def integration_directories() -> list[Path]:
    directories = [ROOT / "apps/personal-assistant-agent/config/integrations"]
    external = os.environ.get("STS_INTEGRATIONS_DIR")
    if external:
        directories.append(Path(external))
    return directories


def integration_catalog() -> list[dict]:
    catalog = []
    names = set()
    for directory in integration_directories():
        for path in sorted(directory.glob("*/runtime.json")):
            entry = json.loads(path.read_text())
            profiles = json.loads((path.parent / "profile.json").read_text())["tools"]
            name = entry.get("name")
            if not name or name in names:
                raise RuntimeError(f"Integração duplicada ou sem nome: {path}.")
            names.add(name)
            entry["allowed_tools"] = list(profiles)
            entry["policies"] = {tool: value["policy"] for tool, value in profiles.items()}
            catalog.append(entry)
    # Keep the legacy menu order; additional applications are discovered below.
    return sorted(catalog, key=lambda entry: ({"notes": 0, "agenda": 1}.get(entry["name"], 2), entry["name"]))


def servers(mode: int, fixtures: bool, names: list[str] | None = None) -> list[dict]:
    selected_names = names if names is not None else {1: [], 2: ["notes"], 3: ["agenda"]}.get(mode)
    selected = []
    for entry in integration_catalog():
        if selected_names is not None and entry["name"] not in selected_names:
            continue
        entry["command"] = str(ROOT / entry["command"])
        entry["cwd"] = str(ROOT / entry["cwd"])
        entry["args"] = [arg.replace("{root}", str(ROOT)) for arg in entry["args"]]
        if fixtures:
            entry["args"].extend(entry["fixture_args"])
        selected.append(entry)
    if selected_names is not None and set(selected_names) != {entry["name"] for entry in selected}:
        raise RuntimeError("MCP solicitado não existe no catálogo de integrações.")
    return selected


def environment(provider: str, profile: str, lan: bool, token: str, selected: list[dict], gateway_port: int = 8080) -> dict[str, str]:
    env = dict(os.environ)
    integrations_dir = env.get("STS_INTEGRATIONS_DIR")
    # Do not accidentally inherit a previous MCP demo, listen address or evidence capture.
    for key in list(env):
        if key.startswith("STS_") or key.startswith("NOVA_"):
            env.pop(key)
    for key in list(env):
        if key.startswith("AWS_"):
            env.pop(key, None)  # AWS credentials belong only to the LiteLLM container.
    env.update(STS_PROVIDER=provider, STS_ENV="development",
               STS_GATEWAY_ADDRESS=f"0.0.0.0:{gateway_port}" if lan else f"127.0.0.1:{gateway_port}",
               STS_DEVELOPMENT_TOKEN=token, STS_LITELLM_URL=LITELLM_URL,
               STS_LITELLM_REALTIME_MODEL="nova-sonic",
               STS_AGENT_PROMPT_FILE=str(ROOT / "apps/personal-assistant-agent/prompts/system.md"))
    if integrations_dir:
        env["STS_INTEGRATIONS_DIR"] = integrations_dir
    env.setdefault("GOCACHE", "/private/tmp/sts-go-cache")
    return env


def runtime_environments(env: dict[str, str]) -> tuple[dict[str, str], dict[str, str] | None]:
    """Separate edge credentials from semantic credentials before spawning either app."""
    if env.get("STS_AGENT_MODE") != "remote":
        return dict(env), None
    internal_token = secrets.token_hex(32)
    address = env.get("STS_AGENT_ADDRESS", "127.0.0.1:8090")
    edge_keys = {"STS_ENV", "STS_PROVIDER", "STS_AGENT_MODE", "STS_GATEWAY_ADDRESS",
                 "STS_DEVELOPMENT_TOKEN", "STS_READ_TIMEOUT", "STS_WRITE_TIMEOUT",
                 "STS_IDLE_TIMEOUT", "STS_MAX_SESSIONS", "STS_MAX_EVENT_BYTES", "STS_TURN_TIMEOUT"}
    gateway = {key: value for key, value in env.items()
               if not key.startswith("STS_") or key in edge_keys}
    gateway.update(STS_AGENT_TOKEN=internal_token, STS_DEFAULT_AGENT_ID="personal-assistant",
                   STS_AGENT_CATALOG=json.dumps([{"id": "personal-assistant",
                       "endpoint": f"ws://{address}/v1/session",
                       "protocol": "agent-session/v1", "token_env": "STS_AGENT_TOKEN"}]))
    agent = {key: value for key, value in env.items()
             if key not in edge_keys or key in {"STS_PROVIDER", "STS_TURN_TIMEOUT"}}
    agent.update(STS_AGENT_ADDRESS=address, STS_AGENT_TOKEN=internal_token)
    return gateway, agent


def load_local_litellm_env() -> dict[str, str]:
    values = {}
    try:
        lines = LITELLM_ENV.read_text().splitlines()
    except OSError as error:
        raise RuntimeError(f"Configuração local ausente: {LITELLM_ENV}") from error
    for line in lines:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        if not separator or not key or any(char.isspace() for char in key):
            raise RuntimeError(f"Linha inválida em {LITELLM_ENV.name}.")
        values[key] = value
    required = {"LITELLM_POSTGRES_PASSWORD", "LITELLM_MASTER_KEY", "LITELLM_SERVICE_KEY",
                "STS_MCP_CONTEXT_SECRET"}
    if not required.issubset(values):
        raise RuntimeError(f"Preencha todas as chaves obrigatórias em {LITELLM_ENV}.")
    secret = values["STS_MCP_CONTEXT_SECRET"]
    if len(secret) != 64 or any(char not in "0123456789abcdef" for char in secret):
        raise RuntimeError("STS_MCP_CONTEXT_SECRET deve ter 64 caracteres hexadecimais minúsculos.")
    return values


def compose_command(*arguments: str) -> list[str]:
    return ["docker", "compose", "--env-file", str(LITELLM_ENV), "-f", str(LITELLM_COMPOSE), *arguments]


def running_litellm_services(env: dict[str, str]) -> set[str]:
    result = subprocess.run(compose_command("ps", "--status", "running", "--services"), cwd=ROOT, env=env,
                            capture_output=True, text=True)
    if result.returncode:
        detail = result.stderr.strip().splitlines()
        raise RuntimeError(f"Docker Compose indisponível: {detail[-1] if detail else 'verifique o Docker Desktop'}")
    return set(result.stdout.split())


def wait_litellm_ready(timeout: float = 90) -> None:
    deadline = time.monotonic() + timeout
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    while time.monotonic() < deadline:
        try:
            with opener.open(LITELLM_URL + "/health/liveliness", timeout=1) as response:
                if response.status == 200:
                    return
        except OSError:
            pass
        time.sleep(0.5)
    raise RuntimeError("LiteLLM não ficou saudável no prazo. Consulte: docker compose -f deploy/litellm/compose.yaml logs")


def ensure_litellm(env: dict[str, str], fixtures: bool, profile: str) -> dict[str, str]:
    local = load_local_litellm_env()
    compose_env = dict(env)
    compose_env.update(local)
    compose_env.update(AWS_PROFILE=profile, AWS_REGION="us-east-1")
    compose_env["STS_AGENDA_FIXTURES"] = "enabled" if fixtures else "disabled"
    active = running_litellm_services(compose_env)
    start = not active
    if active:
        print(f"\nLiteLLM Docker ativo: {', '.join(sorted(active))}.")
        if yes("Deseja reiniciar o LiteLLM e o PostgreSQL?"):
            checked(compose_command("down"), compose_env)
            start = True
        elif active != LITELLM_SERVICES:
            raise RuntimeError("Ambiente LiteLLM parcialmente ativo; autorize o reinício para recuperá-lo.")
        elif fixtures:
            raise RuntimeError("Semear a Agenda exige reiniciar o LiteLLM para aplicar a configuração.")
        else:
            print("Containers existentes preservados.")
    if start:
        print("\nIniciando LiteLLM e PostgreSQL locais…", flush=True)
        checked(compose_command("up", "--build", "-d"), compose_env)
    wait_litellm_ready()
    print(f"LiteLLM pronto. Painel: {LITELLM_URL}/ui", flush=True)
    return local


def check_litellm_aws_identity(env: dict[str, str], profile: str) -> None:
    code = ("import boto3,sys; "
            "identity=boto3.Session(profile_name=sys.argv[1],region_name='us-east-1').client('sts').get_caller_identity(); "
            "print('AWS account:', identity['Account'])")
    checked(compose_command("exec", "-T", "litellm", "python", "-c", code, profile), env)


def gateway_servers(selected: list[dict]) -> list[dict]:
    return [{"alias": entry["alias"], "server_id": entry["server_id"],
             "allowed_tools": entry["allowed_tools"], "policies": entry["policies"]}
            for entry in selected]


def ensure_litellm_service_key(master_key: str, service_key: str, selected: list[dict]) -> str:
    configured = gateway_servers(selected)
    key_config = {
        "key": service_key,
        "key_alias": "sts-go-gateway",
        "models": ["nova-sonic"],
        "allowed_routes": ["/v1/realtime", "mcp_routes"],
        "object_permission": {
            "mcp_servers": [server["server_id"] for server in configured],
            "mcp_tool_permissions": {server["server_id"]: server["allowed_tools"] for server in configured},
        },
        "mcp_rpm_limit": {server["alias"]: 60 for server in configured},
    }
    headers = {"Authorization": "Bearer " + master_key, "Content-Type": "application/json"}
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def request(path: str, body: dict = key_config) -> None:
        operation = urllib.request.Request(LITELLM_URL + path, data=json.dumps(body).encode(),
                                           method="POST", headers=headers)
        with opener.open(operation, timeout=15) as response:
            json.load(response)

    lookup = urllib.request.Request(
        LITELLM_URL + "/key/info?key=" + urllib.parse.quote(service_key, safe=""),
        headers={"Authorization": "Bearer " + master_key},
    )
    try:
        with opener.open(lookup, timeout=15) as response:
            token_id = json.load(response).get("key")
    except urllib.error.HTTPError as error:
        if error.code not in (400, 404):
            raise RuntimeError("LiteLLM não consultou a chave de serviço local.") from error
        try:
            request("/key/generate")
        except (OSError, ValueError) as create_error:
            raise RuntimeError("LiteLLM não configurou a chave de serviço local.") from create_error
    except (OSError, ValueError) as error:
        raise RuntimeError("LiteLLM não consultou a chave de serviço local.") from error
    else:
        if not token_id:
            raise RuntimeError("LiteLLM não localizou a chave de serviço local.")
        update_config = dict(key_config)
        update_config["key"] = token_id
        # Preserve an existing alias. Local databases may already contain
        # another key with the canonical alias; scope the configured raw key
        # without deleting or silently taking over that credential.
        update_config.pop("key_alias", None)
        try:
            request("/key/update", update_config)
        except (OSError, ValueError) as update_error:
            raise RuntimeError("LiteLLM não atualizou a chave de serviço local.") from update_error
    return service_key


def configure_litellm_backend(env: dict[str, str], selected: list[dict], local: dict[str, str]) -> None:
    env.update(
        STS_MCP_BACKEND="litellm" if selected else "",
        STS_LITELLM_URL=LITELLM_URL,
        STS_LITELLM_REALTIME_MODEL="nova-sonic",
        STS_LITELLM_API_KEY=ensure_litellm_service_key(
            local["LITELLM_MASTER_KEY"], local["LITELLM_SERVICE_KEY"], selected),
        STS_MCP_CONTEXT_SECRET=local["STS_MCP_CONTEXT_SECRET"],
        STS_LITELLM_MCP_SERVERS=json.dumps(gateway_servers(selected), separators=(",", ":")),
    )


def check_port(port: int, host: str = "127.0.0.1") -> None:
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            probe.bind((host, port))
        except OSError as error:
            raise RuntimeError(f"Porta {port} indisponível. Encerre o serviço anterior; nenhum processo existente será parado.") from error


def process_identity(pid: int) -> tuple[str, str] | None:
    """Recognize this checkout's services, not arbitrary processes named Python/gateway."""
    def query(command: list[str]) -> str:
        result = subprocess.run(command, capture_output=True, text=True)
        return result.stdout.strip() if result.returncode == 0 else ""
    owner = query(["ps", "-p", str(pid), "-o", "uid="])
    if owner != str(os.getuid()):
        return None
    started = query(["ps", "-p", str(pid), "-o", "lstart="])
    cwd = query(["lsof", "-a", "-p", str(pid), "-d", "cwd", "-Fn"])
    if f"n{ROOT}" not in cwd.splitlines() or not started:
        return None
    executable = query(["lsof", "-a", "-p", str(pid), "-d", "txt", "-Fn"])
    for name in ("gateway", "personal-assistant-agent"):
        if f"n{ROOT / 'bin' / name}" in executable.splitlines():
            return name, started
    return None


def ensure_ports(ports: list[tuple[int, str]]) -> None:
    """Offer graceful stop of recognized listeners only, rechecking identity before signaling."""
    occupied = []
    for port, host in ports:
        try:
            check_port(port, host)
            continue
        except RuntimeError:
            pass
        if not shutil.which("lsof"):
            raise RuntimeError("Porta ocupada; lsof ausente. Encerre o serviço manualmente.")
        result = subprocess.run(["lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"],
                                capture_output=True, text=True)
        ids = result.stdout.split()
        if not ids or any(not pid.isdigit() for pid in ids):
            raise RuntimeError(f"Não foi possível identificar a porta {port}. Nenhum processo será encerrado.")
        for pid in sorted({int(pid) for pid in ids}):
            identity = process_identity(pid)
            if identity is None:
                raise RuntimeError(f"Porta {port} pertence a processo não reconhecido desta POC (PID {pid}). Encerre manualmente ou escolha outra porta.")
            occupied.append((port, pid, identity))
    if not occupied:
        return
    print("\nServiços desta POC já ativos:")
    for port, pid, identity in occupied:
        print(f"  {identity[0]} · porta {port} · PID {pid}")
    print("Encerrar interrompe conversas ativas. Bancos serão preservados.")
    if not yes("Encerrar esses serviços para iniciar o ambiente escolhido?"):
        raise RuntimeError("Inicialização cancelada; serviços existentes preservados.")
    for _, pid, identity in occupied:
        current = process_identity(pid)
        if current is None:
            # Another supervisor may have already stopped its services.
            try:
                os.kill(pid, 0)
            except ProcessLookupError:
                continue
        if current != identity:
            raise RuntimeError("A identidade de um processo mudou. Encerramento interrompido por segurança.")
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        try:
            for port, host in ports:
                check_port(port, host)
            print("Portas liberadas; iniciando o novo ambiente.")
            return
        except RuntimeError:
            time.sleep(0.2)
    raise RuntimeError("Serviço não liberou a porta em 10 s. Nenhum encerramento forçado foi aplicado; confira o terminal anterior.")


def stop_children(children: list[subprocess.Popen]) -> None:
    # Each child starts its own group, containing only the services we launched (including MCPs).
    for child in reversed(children):
        try:
            os.killpg(child.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + 5
    for child in reversed(children):
        try:
            child.wait(timeout=max(0.01, deadline - time.monotonic()))
        except subprocess.TimeoutExpired:
            pass
    for child in reversed(children):
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.wait()


def checked(command: list[str], env: dict[str, str]) -> None:
    """Also reap installation/build/authentication children if startup is interrupted."""
    child = subprocess.Popen(command, cwd=ROOT, env=env, start_new_session=True)
    try:
        status = child.wait()
        if status:
            raise subprocess.CalledProcessError(status, command)
    except BaseException:
        stop_children([child])
        raise


def wait_ready(children: list[subprocess.Popen], port: int) -> None:
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if any(child.poll() is not None for child in children):
            raise RuntimeError("Um serviço encerrou antes de ficar pronto. Consulte os logs em logs/dev/.")
        try:
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(f"http://127.0.0.1:{port}/healthz", timeout=0.5) as response:
                if response.status == 200:
                    return
        except OSError:
            pass
        time.sleep(0.2)
    raise RuntimeError("O serviço não ficou pronto em 30 s. Consulte logs/dev/.")


def run(env: dict[str, str], selected: list[dict], nova: bool, fixtures: bool = False) -> None:
    for executable in (("go", "make", "docker") if nova else ("go", "make")):
        if not shutil.which(executable):
            raise RuntimeError(f"Ferramenta ausente: {executable}. Veja os pré-requisitos no README.")
    gateway_port = int(env["STS_GATEWAY_ADDRESS"].rsplit(":", 1)[1])
    ports = [(gateway_port, "0.0.0.0" if env["STS_GATEWAY_ADDRESS"].startswith("0.0.0.0") else "127.0.0.1")]
    if env.get("STS_AGENT_MODE") == "remote":
        ports.append((int(env["STS_AGENT_ADDRESS"].rsplit(":", 1)[1]), "127.0.0.1"))
    ensure_ports(ports)
    profile = env.pop("_STS_AWS_PROFILE", "default")
    if nova:
        local_litellm = ensure_litellm(env, fixtures, profile)
        compose_env = dict(env)
        compose_env.update(load_local_litellm_env(), AWS_PROFILE=profile, AWS_REGION="us-east-1")
        check_litellm_aws_identity(compose_env, profile)
        configure_litellm_backend(env, selected, local_litellm)
    gateway_env, agent_env = runtime_environments(env)
    print("\nCompilando gateway…", flush=True)
    (ROOT / "bin").mkdir(exist_ok=True)
    checked(["go", "build", "-o", str(ROOT / "bin/gateway"), "./apps/voice-gateway/cmd/gateway"], env)
    if agent_env is not None:
        checked(["go", "build", "-o", str(ROOT / "bin/personal-assistant-agent"), "./apps/personal-assistant-agent/cmd/agent"], agent_env)
    (ROOT / "data").mkdir(exist_ok=True)
    logs = ROOT / "logs/dev"
    logs.mkdir(parents=True, exist_ok=True)
    logs.chmod(0o700)
    children = []
    handles = []
    try:
        services = []
        if agent_env is not None:
            services.append(("personal-assistant-agent", agent_env, int(agent_env["STS_AGENT_ADDRESS"].rsplit(":", 1)[1])))
        services.append(("gateway", gateway_env, gateway_port))
        for name, service_env, service_port in services:
            command = [str(ROOT / "bin" / name)]
            path = logs / f"{name}-{time.time_ns()}.log"
            handle = path.open("x")
            path.chmod(0o600)
            handles.append(handle)
            children.append(subprocess.Popen(command, cwd=ROOT, env=service_env, stdin=subprocess.DEVNULL,
                                             stdout=handle, stderr=handle, start_new_session=True))
            wait_ready(children, service_port)
            print(f"{name} pronto. Log: {path}", flush=True)
        print("\nAmbiente pronto. Configure o app:")
        host = "IP-DO-MAC" if env["STS_GATEWAY_ADDRESS"].startswith("0.0.0.0") else "127.0.0.1"
        print(f"  URL: ws://{host}:{gateway_port}/v1/voice")
        print(f"  Provider: {env['STS_PROVIDER']}")
        print(f"  Token local: {env['STS_DEVELOPMENT_TOKEN']}")
        print("Os MCPs selecionados são mediados pelo LiteLLM local." if selected else "Nenhum MCP selecionado.")
        print("Ctrl+C encerra gateway e agente; LiteLLM e bancos permanecem ativos.", flush=True)
        while True:
            if any(child.poll() is not None for child in children):
                raise RuntimeError("Um serviço encerrou. O assistente vai parar os demais; consulte logs/dev/.")
            time.sleep(0.3)
    finally:
        stop_children(children)
        for handle in handles:
            handle.close()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dry-run", action="store_true", help="Pergunta e valida escolhas sem autenticar, compilar ou iniciar serviços.")
    parser.add_argument("--gateway-port", type=int, default=8080, help="Porta local do gateway (padrão 8080).")
    parser.add_argument("--agent-mode", choices=["remote", "inprocess"], default="remote", help="Transporte Nova; inprocess permite rollback.")
    parser.add_argument("--agent-port", type=int, default=8090, help="Porta loopback do agente remoto.")
    parser.add_argument("--mcps", help="Nomes do catálogo separados por vírgula; substitui a seleção interativa de MCPs.")
    args = parser.parse_args()
    if not 1 <= args.gateway_port <= 65535 or not 1 <= args.agent_port <= 65535 or args.agent_port == args.gateway_port:
        parser.error("Use uma porta entre 1 e 65535.")
    print("Nova Voice · assistente de ambiente local")
    nova = choose("Qual ambiente deseja iniciar?", ["Fake — sem microfone/AWS/MCPs", "Nova 2 Sonic — AWS + MCPs opcionais"], 2) == 2
    profile = (input("Perfil AWS existente [default]: ").strip() or "default") if nova else "default"
    if len(profile) > 128 or any(ord(char) < 32 for char in profile):
        raise RuntimeError("Nome de perfil inválido.")
    mode = choose("Quais MCPs habilitar?", ["Nenhum", "Notes", "Agenda", "Todos do catálogo"], 4) if nova and args.mcps is None else (4 if nova else 1)
    fixtures = yes("Semear Agenda com evento fictício em 16/05/2030, 10–11h?") if mode in (3, 4) else False
    lan = choose("Onde o app vai rodar?", ["Mac / iOS Simulator — somente loopback", "iPhone físico — gateway na rede local"], 1) == 2
    if lan:
        print("Atenção: ws:// envia áudio/token sem TLS. Use somente Wi-Fi privado confiável; LiteLLM permanece em loopback.")
        if not yes("Autoriza expor o gateway na rede local?"):
            return 0
    prompt = "Token local [local] (Enter usa local; não é chave AWS): "
    token = (getpass.getpass(prompt) if sys.stdin.isatty() else input(prompt)) or "local"
    if len(token) > 4096 or any(ord(char) <= 32 or ord(char) >= 127 for char in token):
        raise RuntimeError("Token inválido.")
    if lan and token == "local":
        print("Atenção: o token padrão é previsível. Use outro token para acesso pela rede local.")
    selected = servers(mode, fixtures, args.mcps.split(",") if args.mcps is not None else None)
    env = environment("nova" if nova else "fake", profile, lan, token, selected, args.gateway_port)
    env["STS_AGENT_MODE"] = args.agent_mode if nova else "inprocess"
    env["STS_AGENT_ADDRESS"] = f"127.0.0.1:{args.agent_port}"
    env["_STS_AWS_PROFILE"] = profile
    print(f"\nResumo: provider={env['STS_PROVIDER']}, agent={env['STS_AGENT_MODE']}, região=us-east-1, gateway={env['STS_GATEWAY_ADDRESS']}")
    print(f"MCPs: {', '.join(entry['alias'] for entry in selected) or 'nenhum'}; fixtures={'sim' if fixtures else 'não'}")
    print(f"Bancos persistentes: {ROOT / 'data'}; nenhum dado existente será apagado.")
    if args.dry_run:
        print("Dry-run: sem AWS, instalação, build, arquivos de runtime ou processos. Token não exibido.")
        return 0
    if nova:
        print(f"Perfil: {profile}. Verificaremos identidade AWS; isso não comprova permissão Bedrock.")
        print("Conversas Nova geram custos Bedrock. Iniciar os serviços não abre inferência automaticamente.")
    if not yes("Iniciar este ambiente?"):
        return 0
    run(env, selected, nova, fixtures)
    return 0


if __name__ == "__main__":
    def terminate(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, terminate)
    try:
        sys.exit(main())
    except (KeyboardInterrupt, EOFError):
        print("\nAmbiente encerrado/cancelado.")
        sys.exit(0)
    except (RuntimeError, OSError, subprocess.CalledProcessError) as error:
        print(f"\nFalha: {error}", file=sys.stderr)
        sys.exit(1)
