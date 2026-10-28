#!/usr/bin/env python3
"""Regresiones sin clúster del límite entre preparar la demo y confirmar corridas."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SMOKE = (ROOT / 'scripts/kind/kind-smoke.sh').read_text()
SETUP = (ROOT / 'scripts/demo/00-setup.sh').read_text()


def function(name):
    start = SMOKE.index(name + '() {')
    return SMOKE[start:SMOKE.index('\n}\n', start) + 3]


class PreflightTest(unittest.TestCase):
    def shell(self, script, **values):
        with tempfile.TemporaryDirectory(prefix='demo-preflight-test.') as work:
            env = dict(os.environ, TEST_ROOT=str(ROOT), TEST_WORK=work, **values)
            return subprocess.run(['bash', '-c', 'set -euo pipefail\nroot=$TEST_ROOT\nwork=$TEST_WORK\n' + script],
                                  env=env, text=True, capture_output=True, timeout=10)

    def test_preflight_never_calls_active_cycle(self):
        result = self.shell('preflight=1\ncheck() { echo unexpected-active-cycle; exit 97; }\n' +
                            function('check_ciclo_corrida') + '\ncheck_ciclo_corrida')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('PENDIENTE ciclo-webhook-confirm-corrida', result.stdout)
        self.assertNotIn('unexpected-active-cycle', result.stdout)

    def test_default_smoke_preserves_active_cycle(self):
        result = self.shell('preflight=0\ncheck() { "$2"; }\n'
                            'ciclo_webhook_confirm_corrida() { echo simulated-active-cycle; }\n' +
                            function('check_ciclo_corrida') + '\ncheck_ciclo_corrida')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('simulated-active-cycle', result.stdout)

    def test_warm_must_be_ready_and_verified(self):
        for state, verified, expected in [('dirty', 'false', 1), ('ready', 'false', 1),
                                          ('dirty', 'true', 1), ('ready', 'true', 0)]:
            with self.subTest(state=state, reset_verified=verified):
                result = self.shell('preflight=1\nP_UI=1\nTOKEN=simulated\n'
                                    'expect_http() { printf "%s" "$TEST_BODY" >"$work/body"; }\n' +
                                    function('warm_estado') + '\nwarm_estado',
                                    TEST_BODY='{"state":"' + state + '","reset_verified":' + verified + '}')
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)

    def test_setup_uses_preflight(self):
        block = SETUP[SETUP.index('# --- 6. Humo'):SETUP.index('# --- 7. Resumen')]
        result = self.shell('fail=0\nbash() { printf "%s\\n" "$*" >"$work/args"; echo "OK simulated-smoke"; }\n' +
                            block + '\ncat "$work/args"')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('scripts/kind/kind-smoke.sh --preflight', result.stdout)

    def test_unknown_option_rejected_before_cluster_access(self):
        result = subprocess.run(['bash', str(ROOT / 'scripts/kind/kind-smoke.sh'), '--unknown'],
                                text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 2)
        self.assertIn('uso:', result.stderr)


if __name__ == '__main__':
    unittest.main(verbosity=2)
