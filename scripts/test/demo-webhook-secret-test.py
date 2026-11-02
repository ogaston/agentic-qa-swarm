"""El secreto que up.sh pone en el webhook de GitHub tiene que coincidir con el que go-intake verifica.

Si el repo está registrado en go-governance (asistente de proyectos) con webhook_secret_version >= 1, go-intake
verifica con el secreto derivado; si no, con el de plataforma. Sin esto, cada push da 401 invalid_signature.
"""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/demo/webhook-secret.py'
UP = (ROOT / 'scripts/demo/up.sh').read_text()


class WebhookSecretTest(unittest.TestCase):
    def test_golden_vectors_match_go(self):
        fixture = json.loads((ROOT / 'contracts/examples/webhook-secrets.json').read_text())
        self.assertEqual(fixture['context'], 'aqs/webhook-secret/v1')
        with tempfile.TemporaryDirectory() as d:
            for v in fixture['vectors']:
                key, out = Path(d, 'key'), Path(d, 'out')
                key.write_text(v['platform_key'])
                r = subprocess.run([sys.executable, str(SCRIPT), str(key), v['repo'], str(v['version']), str(out)],
                                   capture_output=True, text=True)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(out.read_text(), v['secret'], v['repo'])
                self.assertEqual(out.stat().st_mode & 0o777, 0o600)
                self.assertNotIn(v['secret'], r.stdout + r.stderr)

    def test_invalid_input_fails_without_output(self):
        with tempfile.TemporaryDirectory() as d:
            key, out = Path(d, 'key'), Path(d, 'out')
            key.write_text('k')
            for repo, ver in (('acme/shop', '0'), ('no-slash', '1')):
                r = subprocess.run([sys.executable, str(SCRIPT), str(key), repo, ver, str(out)],
                                   capture_output=True, text=True)
                self.assertEqual(r.returncode, 1)
                self.assertFalse(out.exists())

    def test_up_uses_registry_version(self):
        self.assertIn('/data/repositories.json', UP)
        self.assertIn('scripts/demo/webhook-secret.py', UP)
        # El secreto derivado se calcula antes de armar la petición a GitHub.
        self.assertLess(UP.index('webhook-secret.py'), UP.index('--rawfile secret'))


if __name__ == '__main__':
    unittest.main(verbosity=2)
