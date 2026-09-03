"""Exercise rollout timing without network calls or real sleeps."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("smoke.sh")
FAKE_TOOL = r'''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path

root = Path(os.environ["SMOKE_TEST_STATE"])
def read(name):
    path = root / name
    return int(path.read_text()) if path.exists() else 0
def write(name, value):
    (root / name).write_text(str(value))

tool = Path(sys.argv[0]).name
if tool == "sleep":
    write("clock", read("clock") + int(sys.argv[1]))
elif tool == "date":
    print(read("clock") * (1000 if sys.argv[1] == "+%s%3N" else 1))
else:
    url = next(arg for arg in sys.argv[1:] if arg.startswith("https://"))
    if url.endswith("getChatMenuButton"):
        count = read("menus") + 1
        write("menus", count)
        version = "2.0.0" if count >= int(os.environ["SMOKE_TEST_READY"]) else "2.0.01"
        print(json.dumps({"ok": True, "result": {"type": "web_app", "web_app": {
            "url": "https://example.test/app/?v=" + version}}}, separators=(",", ":")))
    elif "/api/" in url:
        method = sys.argv[sys.argv.index("-X")+1] if "-X" in sys.argv else "GET"
        # Fetch-based edge proxies cannot construct GET/HEAD requests with bodies.
        print("500" if method in ("GET", "HEAD") and "-d" in sys.argv else "401")
    elif url.endswith("/healthz"):
        print('{"status":"ok","version":"2.0.0"}')
    elif url.endswith("/readyz"):
        print('{"database":true,"migration_version":22}')
    elif url.endswith("/status"):
        print('{"oldest_pending_command_s":0}')
    elif url.endswith("/app/version"):
        print('{"version":"2.0.0"}')
    elif url.endswith("/app/"):
        print('telegram-web-app.js a/2.0.0/js/main.js')
    else:
        print('budget-view budget-form plan-goals loan-view --brass:')
'''


class SmokeRolloutTest(unittest.TestCase):
    def run_smoke(self, ready):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tool = root / "fake-tool"
            tool.write_text(FAKE_TOOL)
            tool.chmod(0o755)
            for name in ("curl", "date", "sleep"):
                (root / name).symlink_to(tool)
            env = {
                **os.environ,
                "PATH": str(root) + os.pathsep + os.environ["PATH"],
                "SMOKE_TEST_STATE": str(root),
                "SMOKE_TEST_READY": str(ready),
                "SMOKE_DEADLINE_S": "240",
                "MARUM_BOT_TOKEN": "local-test",
            }
            result = subprocess.run(
                ["bash", str(SCRIPT), "https://example.test", "2.0.0"],
                env=env, capture_output=True, text=True, timeout=30,
            )
            return result, int((root / "menus").read_text()) if (root / "menus").exists() else 0

    def test_unsigned_get_probes_have_no_body(self):
        result, _ = self.run_smoke(1)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("smoke passed", result.stdout)

    def test_same_version_container_does_not_shorten_menu_rollout(self):
        result, polls = self.run_smoke(15)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(polls, 15)
        self.assertIn("smoke passed", result.stdout)

    def test_similar_but_wrong_menu_version_never_passes(self):
        result, polls = self.run_smoke(999)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(polls, 48)
        self.assertIn("does not point at 2.0.0 after 240s", result.stdout)
        self.assertNotIn("smoke passed", result.stdout)


if __name__ == "__main__":
    unittest.main()
