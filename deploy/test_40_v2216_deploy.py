"""Offline checks for deployment drain and data compatibility gates."""
from pathlib import Path
import io
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


class DeploymentGates(unittest.TestCase):
    script_name = "40-bluegreen-v2216-20261117.sh"

    def run_gate(self, commands):
        source = Path(__file__).with_name(self.script_name).read_text()
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

    def test_rollback_rejects_database_read_failure(self):
        for failure in ("snapshot", "billing"):
            with self.subTest(failure=failure):
                result = self.run_gate(
                    'touch "$BACKUP_DIR/sub2api-totp-before" "$BACKUP_DIR/sub2api_mr-totp-before"\n'
                    'totp_compat_snapshot() { return ' + ('1' if failure == 'snapshot' else '0') + '; }\n'
                    'sudo() { echo 0; return ' + ('1' if failure == 'billing' else '0') + '; }\n'
                    'if rollback_compatible; then exit 0; else exit 7; fi'
                )
                self.assertEqual(result.returncode, 7, result.stderr)


class PR17DeploymentGates(DeploymentGates):
    script_name = "40-bluegreen-pr17-20261117.sh"

    def test_preserve_blue_asset_graph_and_ignore_external_sdk_paths(self):
        source = Path(__file__).with_name(self.script_name).read_text()
        code = source.split("preserve_assets() {", 1)[1].split("<<'PY'\n", 1)[1].split("\nPY", 1)[0]
        entry = '<script type="module" src="/assets/index-AbCd1234.js"></script>'
        modules = ','.join('"assets/part-%08d.js"' % i for i in range(101))
        routes = {'/': entry.encode(), '/assets/index-AbCd1234.js':
                  (modules + ',"./style-AbCd1234.css","/assets/elements.bundle.min.js"').encode(),
                  '/assets/style-AbCd1234.css': b'body{color:black}'}
        routes.update({'/assets/part-%08d.js' % i: b'export default 1' for i in range(101)})

        def read(url, timeout):
            path = '/' + url.split('/', 3)[3]
            self.assertIn(path, routes)
            response = io.BytesIO(routes[path])
            response.headers = {'Content-Type': 'text/html' if path == '/' else 'application/javascript'}
            return response

        with tempfile.TemporaryDirectory() as directory:
            origin, target = Path(directory)/'origin', Path(directory)/'target'
            origin.mkdir()
            with patch.object(sys, 'argv', ['fixture', str(origin), str(target)]), \
                    patch('urllib.request.urlopen', side_effect=read), patch('builtins.print'):
                exec(compile(code, self.script_name, 'exec'), {})
            self.assertEqual(len(list(target.rglob('*'))), 103)
            self.assertEqual((target/'index-AbCd1234.js').read_bytes(), routes['/assets/index-AbCd1234.js'])


if __name__ == "__main__":
    unittest.main()
