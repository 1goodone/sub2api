"""Offline checks for deployment drain and data compatibility gates."""
from pathlib import Path
import subprocess
import tempfile
import unittest


class DeploymentGates(unittest.TestCase):
    def run_gate(self, commands):
        source = Path(__file__).with_name("40-bluegreen-v2216-20261117.sh").read_text()
        source = source.split('case "${1:-}" in', 1)[0]
        source = source.replace('BASE=$(tr -d \'\\r\\n\' < "$SOURCE/../source-commit")', 'BASE=fixture')
        with tempfile.TemporaryDirectory() as directory:
            return subprocess.run(
                ["bash", "-s", "--", directory],
                input=source + '\nBACKUP_DIR="$1"\n' + commands,
                text=True, capture_output=True, timeout=10,
            )

    def test_idle_requires_both_databases(self):
        for busy in ("sub2api", "sub2api_mr", "none"):
            with self.subTest(busy=busy):
                result = self.run_gate('sudo() { case " $* " in *" -d ' + busy + ' "*) echo 1 ;; *) echo 0 ;; esac; }; if idle; then exit 0; else exit 7; fi')
                self.assertEqual(result.returncode, 0 if busy == "none" else 7, result.stderr)

    def test_rollback_rejects_missing_snapshot(self):
        result = self.run_gate('if rollback_compatible; then exit 0; else exit 7; fi')
        self.assertEqual(result.returncode, 7, result.stderr)

    def test_rollback_checks_state_and_billing(self):
        for changed, free in ((False, 0), (True, 0), (False, 1)):
            with self.subTest(changed=changed, free=free):
                result = self.run_gate(
                    'printf "original\\n" > "$BACKUP_DIR/sub2api-totp-before"\n'
                    'printf "original\\n" > "$BACKUP_DIR/sub2api_mr-totp-before"\n'
                    'totp_compat_snapshot() { printf "' + ('changed' if changed else 'original') + '\\n"; }\n'
                    'sudo() { echo ' + str(free) + '; }\n'
                    'if rollback_compatible; then exit 0; else exit 7; fi'
                )
                self.assertEqual(result.returncode, 7 if changed or free else 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
