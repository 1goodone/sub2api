import os
import tempfile
import unittest
from pathlib import Path

from tools.openai_siwc_oauth_adapter import prepare_siwc_runtime, siwc_source


class SIWCAdapterTests(unittest.TestCase):
    def test_changed_dependency_is_rejected_without_modifying_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'source'
            (root / 'src').mkdir(parents=True)
            original = root / 'src/protocol-login.mjs'
            original.write_text('// unrecognized dependency')
            target = Path(directory) / 'target'
            target.mkdir()
            with self.assertRaises(ValueError):
                prepare_siwc_runtime(root, target)
            self.assertEqual(original.read_text(), '// unrecognized dependency')
            self.assertFalse((target / 'siwc-runtime').exists())

    @unittest.skipUnless(os.getenv('EXCEL_TEST_TOSUB2_ROOT'), 'pinned runtime is provided by packaging CI')
    def test_pinned_runtime_exports_callback_without_exchanging_tokens(self):
        root = Path(os.environ['EXCEL_TEST_TOSUB2_ROOT'])
        with tempfile.TemporaryDirectory() as directory:
            script = prepare_siwc_runtime(root, Path(directory))
            source = script.read_text()
            self.assertIn('siwc_callback_url:codex.callbackUrl', source)
            self.assertIn('if (client.siwcOAuth) return client.siwcOAuth', source)
            self.assertNotIn('console.log(codex.callbackUrl)', source)
            self.assertTrue((script.parent / 'openai_siwc_password_flow.mjs').is_file())
            self.assertNotEqual(source, (root / 'src/protocol-login.mjs').read_text())
            with self.assertRaises(ValueError):
                siwc_source(source)
