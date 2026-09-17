#!/usr/bin/env python3
"""Interactive local supervisor. No AWS resource provisioning or credential storage."""
from __future__ import annotations

import argparse
import getpass
import json
import os
from pathlib import Path
import secrets
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


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


def servers(mode: int, fixtures: bool) -> list[dict]:
    # Preserve the aliases/policies used by the existing multi-MCP runbook.
    entries = json.loads((ROOT / "docs/mcp-servers.example.json").read_text())
    selected = []
    for entry in entries:
        kind = "notes" if entry["alias"] == "memo" else "agenda"
        if mode == 1 or (mode == 2 and kind != "notes") or (mode == 3 and kind != "agenda"):
            continue
        entry["command"] = str(ROOT / f"bin/mcp-{kind}")
        entry["args"] = ["-db", str(ROOT / "data" / ("sts.sqlite" if kind == "notes" else "agenda.sqlite"))]
        if kind == "agenda" and fixtures:
            entry["args"].append("-fixtures")
        selected.append(entry)
    return selected


def environment(provider: str, profile: str, lan: bool, token: str, selected: list[dict], gateway_port: int = 8080, bridge_port: int = 8091) -> dict[str, str]:
    env = dict(os.environ)
    # Do not accidentally inherit a previous MCP demo, listen address or evidence capture.
    for key in list(env):
        if key.startswith("STS_") or key.startswith("NOVA_"):
            env.pop(key)
    for key in ("AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN"):
        env.pop(key, None)  # The explicitly selected AWS profile is authoritative.
    env.update(STS_PROVIDER=provider, STS_ENV="development",
               STS_GATEWAY_ADDRESS=f"0.0.0.0:{gateway_port}" if lan else f"127.0.0.1:{gateway_port}",
               STS_DEVELOPMENT_TOKEN=token, STS_NOVA_BRIDGE_URL=f"ws://127.0.0.1:{bridge_port}",
               STS_DATABASE_PATH=str(ROOT / "data/sts.sqlite"),
               AWS_PROFILE=profile, AWS_REGION="us-east-1", AWS_DEFAULT_REGION="us-east-1",
               NOVA_MODEL_ID="amazon.nova-2-sonic-v1:0", NOVA_VOICE_ID="carolina",
               NOVA_BRIDGE_HOST="127.0.0.1", NOVA_BRIDGE_PORT=str(bridge_port),
               PYTHONPATH=str(ROOT / "services/nova-bridge"))
    env.setdefault("GOCACHE", "/private/tmp/sts-go-cache")
    if selected:
        env["STS_MCP_SERVERS"] = json.dumps(selected)
    return env


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
    if f"n{ROOT / 'bin/gateway'}" in executable.splitlines():
        return "gateway", started
    command = query(["ps", "-p", str(pid), "-o", "command="])
    try:
        argv = shlex.split(command)
    except ValueError:
        return None
    if argv == [str(ROOT / "services/nova-bridge/.venv/bin/python"), "-m", "nova_bridge.server"]:
        return "bridge", started
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


def run(env: dict[str, str], selected: list[dict], nova: bool) -> None:
    for executable in ("go", "make") + (("aws",) if nova else ()):
        if not shutil.which(executable):
            raise RuntimeError(f"Ferramenta ausente: {executable}. Veja os pré-requisitos no README.")
    gateway_port = int(env["STS_GATEWAY_ADDRESS"].rsplit(":", 1)[1])
    bridge_port = int(env["NOVA_BRIDGE_PORT"])
    ports = [(gateway_port, "0.0.0.0" if env["STS_GATEWAY_ADDRESS"].startswith("0.0.0.0") else "127.0.0.1")]
    if nova:
        ports.append((bridge_port, "127.0.0.1"))
    ensure_ports(ports)
    if nova:
        checked(["aws", "sts", "get-caller-identity", "--profile", env["AWS_PROFILE"], "--region", "us-east-1"], env)
        interpreter = ROOT / "services/nova-bridge/.venv/bin/python"
        if not interpreter.exists():
            if not yes("A ponte Python não está instalada. Instalar com make nova-install?"):
                raise RuntimeError("Instale a ponte com make nova-install e tente novamente.")
            checked(["make", "nova-install"], env)
        checked([str(interpreter), "-c", "import nova_bridge.server"], env)
    print("\nCompilando gateway e MCPs selecionados…", flush=True)
    (ROOT / "bin").mkdir(exist_ok=True)
    checked(["go", "build", "-o", str(ROOT / "bin/gateway"), "./cmd/gateway"], env)
    for entry in selected:
        name = Path(entry["command"]).name
        checked(["go", "build", "-o", entry["command"], f"./cmd/{name}"], env)
    (ROOT / "data").mkdir(exist_ok=True)
    logs = ROOT / "logs/dev"
    logs.mkdir(parents=True, exist_ok=True)
    logs.chmod(0o700)
    children = []
    handles = []
    try:
        for name, command in (([("bridge", [str(interpreter), "-m", "nova_bridge.server"])] if nova else []) +
                              [("gateway", [str(ROOT / "bin/gateway")])]):
            path = logs / f"{name}-{time.time_ns()}.log"
            handle = path.open("x")
            path.chmod(0o600)
            handles.append(handle)
            children.append(subprocess.Popen(command, cwd=ROOT, env=env, stdin=subprocess.DEVNULL,
                                             stdout=handle, stderr=handle, start_new_session=True))
            wait_ready(children, bridge_port if name == "bridge" else gateway_port)
            print(f"{name} pronto. Log: {path}", flush=True)
        print("\nAmbiente pronto. Configure o app:")
        host = "IP-DO-MAC" if env["STS_GATEWAY_ADDRESS"].startswith("0.0.0.0") else "127.0.0.1"
        print(f"  URL: ws://{host}:{gateway_port}/v1/voice")
        print(f"  Provider: {env['STS_PROVIDER']}")
        print(f"  Token local: {env['STS_DEVELOPMENT_TOKEN']}")
        print("Os MCPs selecionados iniciam por sessão de voz, não como serviços TCP.")
        print("Ctrl+C encerra os serviços deste assistente; os bancos são preservados.", flush=True)
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
    parser.add_argument("--bridge-port", type=int, default=8091, help="Porta privada da ponte (padrão 8091).")
    args = parser.parse_args()
    if not (1 <= args.gateway_port <= 65535 and 1 <= args.bridge_port <= 65535) or args.gateway_port == args.bridge_port:
        parser.error("Use portas distintas entre 1 e 65535.")
    print("Nova Voice · assistente de ambiente local")
    nova = choose("Qual ambiente deseja iniciar?", ["Fake — sem microfone/AWS/MCPs", "Nova 2 Sonic — AWS + MCPs opcionais"], 2) == 2
    profile = (input("Perfil AWS existente [default]: ").strip() or "default") if nova else "default"
    if len(profile) > 128 or any(ord(char) < 32 for char in profile):
        raise RuntimeError("Nome de perfil inválido.")
    mode = choose("Quais MCPs habilitar?", ["Nenhum", "Notes", "Agenda", "Notes + Agenda"], 4) if nova else 1
    fixtures = yes("Semear Agenda com evento fictício em 16/05/2030, 10–11h?") if mode in (3, 4) else False
    lan = choose("Onde o app vai rodar?", ["Mac / iOS Simulator — somente loopback", "iPhone físico — gateway na rede local"], 1) == 2
    if lan:
        print("Atenção: ws:// envia áudio/token sem TLS. Use somente Wi-Fi privado confiável; ponte permanece privada.")
        if not yes("Autoriza expor o gateway na rede local?"):
            return 0
    prompt = "Token local (Enter gera um token forte; não é chave AWS): "
    token = (getpass.getpass(prompt) if sys.stdin.isatty() else input(prompt)) or secrets.token_urlsafe(24)
    if len(token) > 4096 or any(ord(char) <= 32 or ord(char) >= 127 for char in token):
        raise RuntimeError("Token inválido.")
    selected = servers(mode, fixtures)
    env = environment("nova" if nova else "fake", profile, lan, token, selected, args.gateway_port, args.bridge_port)
    print(f"\nResumo: provider={env['STS_PROVIDER']}, região=us-east-1, gateway={env['STS_GATEWAY_ADDRESS']}")
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
    run(env, selected, nova)
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
