import importlib.util
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import time
import unittest
import urllib.request
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("dev", ROOT / "scripts/dev.py")
dev = importlib.util.module_from_spec(spec)
spec.loader.exec_module(dev)


class DevTests(unittest.TestCase):
    def test_existing_service_requires_confirmation_and_decline_preserves_it(self):
        with patch.object(dev, "check_port", side_effect=RuntimeError("busy")), \
             patch.object(dev.shutil, "which", return_value="/usr/sbin/lsof"), \
             patch.object(dev.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "123\n", "")), \
             patch.object(dev, "process_identity", return_value=("gateway", "started")), \
             patch.object(dev, "yes", return_value=False), patch.object(dev.os, "kill") as kill:
            with self.assertRaises(RuntimeError):
                dev.ensure_ports([(8080, "127.0.0.1")])
            kill.assert_not_called()

    def test_unknown_listener_is_never_stopped(self):
        with patch.object(dev, "check_port", side_effect=RuntimeError("busy")), \
             patch.object(dev.shutil, "which", return_value="/usr/sbin/lsof"), \
             patch.object(dev.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "123\n", "")), \
             patch.object(dev, "process_identity", return_value=None), patch.object(dev.os, "kill") as kill:
            with self.assertRaises(RuntimeError):
                dev.ensure_ports([(8080, "127.0.0.1")])
            kill.assert_not_called()

    def test_confirmed_service_is_stopped_gracefully(self):
        with patch.object(dev, "check_port", side_effect=[RuntimeError("busy"), None]), \
             patch.object(dev.shutil, "which", return_value="/usr/sbin/lsof"), \
             patch.object(dev.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "123\n", "")), \
             patch.object(dev, "process_identity", return_value=("gateway", "started")), \
             patch.object(dev, "yes", return_value=True), patch.object(dev.os, "kill") as kill:
            dev.ensure_ports([(8080, "127.0.0.1")])
            kill.assert_called_once_with(123, signal.SIGTERM)

    def test_changed_process_identity_is_not_signaled(self):
        with patch.object(dev, "check_port", side_effect=RuntimeError("busy")), \
             patch.object(dev.shutil, "which", return_value="/usr/sbin/lsof"), \
             patch.object(dev.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "123\n", "")), \
             patch.object(dev, "process_identity", side_effect=[("gateway", "old"), ("gateway", "new")]), \
             patch.object(dev, "yes", return_value=True), patch.object(dev.os, "kill") as kill:
            with self.assertRaises(RuntimeError):
                dev.ensure_ports([(8080, "127.0.0.1")])
            kill.assert_not_called()

    def test_mcp_selection_and_no_default_fixtures(self):
        self.assertEqual(dev.servers(1, False), [])
        self.assertEqual([s["alias"] for s in dev.servers(2, False)], ["memo"])
        self.assertEqual([s["alias"] for s in dev.servers(3, False)], ["local"])
        both = dev.servers(4, False)
        self.assertEqual(len(both), 2)
        for server in both:
            self.assertTrue(Path(server["command"]).is_absolute())
            self.assertTrue(Path(server["args"][1]).is_absolute())
            self.assertNotIn("-fixtures", server["args"])
        self.assertIn("-fixtures", dev.servers(3, True)[0]["args"])

    def test_environment_is_explicit_and_does_not_inherit_evidence_or_credentials(self):
        with patch.dict(os.environ, {"STS_MCP_COMMAND": "bad", "STS_MCP_EVIDENCE_PATH": "bad",
                                     "NOVA_BRIDGE_HOST": "0.0.0.0", "AWS_ACCESS_KEY_ID": "not-a-real-key"}):
            env = dev.environment("nova", "TerraformUser", True, "test-token", dev.servers(4, False))
        self.assertNotIn("AWS_ACCESS_KEY_ID", env)
        self.assertNotIn("STS_MCP_COMMAND", env)
        self.assertNotIn("STS_MCP_EVIDENCE_PATH", env)
        self.assertEqual(env["AWS_PROFILE"], "TerraformUser")
        self.assertEqual(env["STS_GATEWAY_ADDRESS"], "0.0.0.0:8080")
        self.assertEqual(env["NOVA_BRIDGE_HOST"], "127.0.0.1")
        self.assertNotIn("STS_MCP_SERVERS", env)

    def test_litellm_backend_uses_selected_servers_and_service_key(self):
        selected = dev.servers(2, False)
        env = {}
        local = {"LITELLM_MASTER_KEY": "admin", "LITELLM_SERVICE_KEY": "service-key",
                 "STS_MCP_CONTEXT_SECRET": "0" * 64}
        with patch.object(dev, "ensure_litellm_service_key", return_value="service-key"):
            dev.configure_litellm_backend(env, selected, local)
        configured = json.loads(env["STS_LITELLM_MCP_SERVERS"])
        self.assertEqual(env["STS_MCP_BACKEND"], "litellm")
        self.assertEqual(env["STS_LITELLM_API_KEY"], "service-key")
        self.assertEqual([server["server_id"] for server in configured], ["sts-notes"])
        self.assertNotIn("command", configured[0])

    def test_running_litellm_can_be_preserved(self):
        local = {"LITELLM_POSTGRES_PASSWORD": "db", "LITELLM_MASTER_KEY": "admin", "LITELLM_SERVICE_KEY": "key",
                 "STS_MCP_CONTEXT_SECRET": "0" * 64}
        with patch.object(dev, "load_local_litellm_env", return_value=local), \
             patch.object(dev, "running_litellm_services", return_value={"postgres", "litellm"}), \
             patch.object(dev, "yes", return_value=False), \
             patch.object(dev, "wait_litellm_ready") as ready, \
             patch.object(dev, "checked") as checked:
            self.assertEqual(dev.ensure_litellm({}, False), local)
        ready.assert_called_once()
        checked.assert_not_called()

    def test_stopped_litellm_is_started(self):
        local = {"LITELLM_POSTGRES_PASSWORD": "db", "LITELLM_MASTER_KEY": "admin", "LITELLM_SERVICE_KEY": "key",
                 "STS_MCP_CONTEXT_SECRET": "0" * 64}
        with patch.object(dev, "load_local_litellm_env", return_value=local), \
             patch.object(dev, "running_litellm_services", return_value=set()), \
             patch.object(dev, "wait_litellm_ready"), patch.object(dev, "checked") as checked:
            dev.ensure_litellm({}, False)
        self.assertIn("up", checked.call_args.args[0])

    def test_partial_litellm_requires_restart(self):
        local = {"LITELLM_POSTGRES_PASSWORD": "db", "LITELLM_MASTER_KEY": "admin", "LITELLM_SERVICE_KEY": "key",
                 "STS_MCP_CONTEXT_SECRET": "0" * 64}
        with patch.object(dev, "load_local_litellm_env", return_value=local), \
             patch.object(dev, "running_litellm_services", return_value={"postgres"}), \
             patch.object(dev, "yes", return_value=False), patch.object(dev, "checked") as checked:
            with self.assertRaises(RuntimeError):
                dev.ensure_litellm({}, False)
        checked.assert_not_called()

    def test_occupied_port_is_rejected_without_stopping_listener(self):
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0)); listener.listen()
            with self.assertRaises(RuntimeError):
                dev.check_port(listener.getsockname()[1])
            self.assertGreater(listener.fileno(), 0)

    def test_process_group_is_reaped(self):
        child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"], start_new_session=True)
        dev.stop_children([child])
        self.assertIsNotNone(child.poll())
        with self.assertRaises(ProcessLookupError):
            os.killpg(child.pid, 0)

    def test_nova_dry_run_does_not_call_aws_or_start_services(self):
        # Nova, existing default profile, both MCPs, no fixtures, loopback, known test token.
        result = subprocess.run([sys.executable, str(ROOT / "scripts/dev.py"), "--dry-run"],
                                input="2\n\n4\nn\n1\ntest-token\n", text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0)
        self.assertIn("MCPs: memo, local; fixtures=não", result.stdout)
        self.assertIn("Dry-run: sem AWS", result.stdout)
        self.assertNotIn("test-token", result.stdout)
        self.assertIn("Perfil AWS existente [default]", result.stdout)

    def test_enter_selects_default_aws_profile(self):
        with patch("builtins.input", side_effect=["2", "", "4", "n", "1", "test-token", "s"]), \
             patch.object(sys.stdin, "isatty", return_value=False), \
             patch.object(sys, "argv", ["dev.py"]), \
             patch.object(dev, "run") as launch:
            self.assertEqual(dev.main(), 0)
        self.assertEqual(launch.call_args.args[0]["AWS_PROFILE"], "default")

    def test_enter_selects_local_token(self):
        with patch("builtins.input", side_effect=["1", "1", "", "s"]), \
             patch.object(sys.stdin, "isatty", return_value=False), \
             patch.object(sys, "argv", ["dev.py"]), \
             patch.object(dev, "run") as launch:
            self.assertEqual(dev.main(), 0)
        self.assertEqual(launch.call_args.args[0]["STS_DEVELOPMENT_TOKEN"], "local")

    def test_decline_does_not_start_services(self):
        result = subprocess.run([sys.executable, str(ROOT / "scripts/dev.py")],
                                input="1\n1\ntest-token\nn\n", text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("Compilando", result.stdout)

    @unittest.skipUnless(os.getenv("STS_DEV_E2E") == "1", "Opt-in local fake startup test")
    def test_actual_fake_startup_and_signal_cleanup(self):
        with socket.socket() as available:
            available.bind(("127.0.0.1", 0))
            port = available.getsockname()[1]
        launcher = subprocess.Popen([sys.executable, "-u", str(ROOT / "scripts/dev.py"), "--gateway-port", str(port)],
                                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            launcher.stdin.write("1\n1\ntest-token\ns\n"); launcher.stdin.flush(); launcher.stdin.close()
            launcher.stdin = None
            deadline = time.monotonic() + 60
            ready = False
            while time.monotonic() < deadline:
                if launcher.poll() is not None:
                    self.fail("Launcher exited before readiness")
                try:
                    with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz", timeout=0.5) as response:
                        ready = response.status == 200
                        if ready:
                            break
                except OSError:
                    pass
                time.sleep(0.2)
            self.assertTrue(ready)
            prefix = []
            while True:
                line = launcher.stdout.readline()
                self.assertTrue(line, "Launcher exited before showing readiness")
                prefix.append(line)
                if "Ambiente pronto." in line:
                    break
            launcher.send_signal(signal.SIGTERM)
            stdout, stderr = launcher.communicate(timeout=15)
            stdout = "".join(prefix) + stdout
            self.assertEqual(launcher.returncode, 0, stderr)
            self.assertIn("gateway pronto", stdout)
            dev.check_port(port)
        finally:
            if launcher.poll() is None:
                launcher.send_signal(signal.SIGTERM)
                launcher.communicate(timeout=15)


if __name__ == "__main__":
    unittest.main()
