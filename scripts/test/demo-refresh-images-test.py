#!/usr/bin/env python3
"""Regresiones sin clúster de scripts/kind/refresh-images.sh y del refresco en scripts/demo/up.sh.

kubectl, kind y podman se sustituyen por ejecutables en un PATH temporal que registran cada
invocación; jq es el real. Ninguna prueba habla con el clúster kind-aqs.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
REFRESH = ROOT / 'scripts/kind/refresh-images.sh'
UP = (ROOT / 'scripts/demo/up.sh').read_text()
PREFIX = 'ghcr.io/ogaston/agentic-qa-swarm'
FUTURE = '2099-01-01T00:00:00Z'
PAST = '2000-01-01T00:00:00Z'

KUBECTL = r'''#!/usr/bin/env bash
printf '%s\n' "$*" >> "$AQS_TEST_LOG/kubectl.log"
ns=""
words=()
args=("$@")
i=0
while [ "$i" -lt "${#args[@]}" ]; do
  a=${args[$i]}
  case "$a" in
    --context) i=$((i + 2)); continue ;;
    -n) ns=${args[$((i + 1))]}; i=$((i + 2)); continue ;;
    -l|-o) i=$((i + 2)); continue ;;
    --timeout=*|-A|--all-namespaces) i=$((i + 1)); continue ;;
    *) words+=("$a"); i=$((i + 1)) ;;
  esac
done
if [ "${words[0]:-}" = config ] && [ "${words[1]:-}" = current-context ]; then
  cat "$AQS_TEST_DIR/context"
  exit 0
fi
if [ "${words[0]:-}" = get ] && [ "${words[1]:-}" = deployments ]; then
  cat "$AQS_TEST_DIR/deployments.json"
  exit 0
fi
if [ "${words[0]:-}" = get ] && [ "${words[1]:-}" = pods ]; then
  cat "$AQS_TEST_DIR/pods-$ns.json" 2>/dev/null || printf '{"items":[]}\n'
  exit 0
fi
if [ "${words[0]:-}" = rollout ]; then
  exit 0
fi
exit 1
'''

KIND = r'''#!/usr/bin/env bash
printf '%s\n' "$*" >> "$AQS_TEST_LOG/kind.log"
exit 0
'''

PODMAN = r'''#!/usr/bin/env bash
printf '%s\n' "$*" >> "$AQS_TEST_LOG/podman.log"
prev=""
for a in "$@"; do
  if [ "$prev" = -o ]; then : > "$a"; fi
  prev=$a
done
exit 0
'''


def deployment(namespace, name, image, labels):
    return {
        'metadata': {'namespace': namespace, 'name': name},
        'spec': {
            'selector': {'matchLabels': labels},
            'template': {'spec': {'containers': [{'name': 'app', 'image': image}]}},
        },
    }


def pod(namespace, name, start_time, phase='Running', terminating=False):
    item = {
        'metadata': {'namespace': namespace, 'name': name},
        'status': {'phase': phase, 'startTime': start_time},
    }
    if terminating:
        item['metadata']['deletionTimestamp'] = start_time
    return item


class RefreshImagesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='demo-refresh-images-test.')
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        self.bin = self.dir / 'bin'
        self.bin.mkdir()
        for name, body in (('kubectl', KUBECTL), ('kind', KIND), ('podman', PODMAN)):
            path = self.bin / name
            path.write_text(body)
            path.chmod(0o755)
        self.log = self.dir / 'calls'
        self.log.mkdir()
        (self.dir / 'context').write_text('kind-aqs\n')
        self.set_deployments([
            deployment('aqs-system', 'go-run-controller',
                       PREFIX + '/go-run-controller:0.0.0', {'app': 'go-run-controller'}),
            deployment('aqs-system', 'ui-api',
                       PREFIX + '/ui-api:0.0.0', {'app': 'ui-api'}),
            deployment('aqs-test', 'warm-app',
                       'ghcr.io/ogaston/demo-target-app-qa-swarm:abc', {'app': 'warm-app'}),
        ])
        self.set_pods('aqs-system', [
            pod('aqs-system', 'go-run-controller-1', FUTURE),
            pod('aqs-system', 'ui-api-1', FUTURE),
        ])
        self.set_pods('aqs-test', [pod('aqs-test', 'warm-app-1', FUTURE)])

    def set_deployments(self, items):
        (self.dir / 'deployments.json').write_text(json.dumps({'items': items}) + '\n')

    def set_pods(self, namespace, items):
        (self.dir / ('pods-' + namespace + '.json')).write_text(json.dumps({'items': items}) + '\n')

    def run_refresh(self, **extra):
        env = {k: v for k, v in os.environ.items() if k != 'AQS_IMAGES'}
        env['PATH'] = str(self.bin) + os.pathsep + env['PATH']
        env['AQS_TEST_DIR'] = str(self.dir)
        env['AQS_TEST_LOG'] = str(self.log)
        env.update({k: str(v) for k, v in extra.items()})
        return subprocess.run(['bash', str(REFRESH)], env=env, text=True,
                              capture_output=True, timeout=120)

    def calls(self, tool):
        path = self.log / (tool + '.log')
        return path.read_text().splitlines() if path.exists() else []

    def test_restarts_only_platform_deployments(self):
        result = self.run_refresh()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('refresh-images: OK (2 deployments reiniciados)', result.stdout)
        calls = self.calls('kubectl')
        self.assertFalse([line for line in calls if 'warm-app' in line], calls)
        for name in ('go-run-controller', 'ui-api'):
            restart = [i for i, line in enumerate(calls)
                       if 'rollout restart deployment/' + name in line]
            self.assertEqual(len(restart), 1, calls)
            self.assertIn('-n aqs-system', calls[restart[0]], calls)
            status = [i for i, line in enumerate(calls)
                      if 'rollout status deployment/' + name in line]
            self.assertEqual(len(status), 1, calls)
            self.assertLess(restart[0], status[0], calls)

    def test_fails_if_pod_predates_restart(self):
        self.set_pods('aqs-system', [
            pod('aqs-system', 'go-run-controller-1', FUTURE),
            pod('aqs-system', 'ui-api-old', PAST),
        ])
        result = self.run_refresh()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('aqs-system/ui-api-old', result.stderr)
        self.set_pods('aqs-system', [
            pod('aqs-system', 'go-run-controller-1', FUTURE),
            pod('aqs-system', 'ui-api-1', FUTURE),
        ])
        result = self.run_refresh()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_wrong_context_touches_nothing(self):
        (self.dir / 'context').write_text('kind-cks2\n')
        result = self.run_refresh()
        self.assertEqual(result.returncode, 3, result.stdout + result.stderr)
        self.assertEqual(self.calls('podman'), [])
        self.assertEqual(self.calls('kind'), [])
        self.assertEqual(self.calls('kubectl'), ['config current-context'])

    def test_aqs_images_subset(self):
        result = self.run_refresh(AQS_IMAGES='ui-api')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        podman = self.calls('podman')
        self.assertTrue([line for line in podman if line.startswith('build ') and '/ui-api:0.0.0' in line], podman)
        self.assertFalse([line for line in podman if 'go-run-controller' in line], podman)
        kind = self.calls('kind')
        self.assertTrue([line for line in kind if 'ui-api' in line], kind)
        self.assertFalse([line for line in kind if 'go-run-controller' in line], kind)
        kubectl = self.calls('kubectl')
        self.assertTrue([line for line in kubectl if 'rollout restart deployment/ui-api' in line], kubectl)
        self.assertFalse([line for line in kubectl if 'go-run-controller' in line], kubectl)

    def test_no_platform_deployments_is_a_noop(self):
        self.set_deployments([
            deployment('aqs-test', 'warm-app',
                       'ghcr.io/ogaston/demo-target-app-qa-swarm:abc', {'app': 'warm-app'}),
        ])
        result = self.run_refresh()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('refresh-images: OK (0 deployments reiniciados)', result.stdout)
        self.assertFalse([line for line in self.calls('kubectl') if 'rollout' in line])

    def test_up_refreshes_existing_cluster(self):
        start = UP.index('bash "$root/scripts/kind/kind-up.sh"')
        end = UP.index('bash "$root/scripts/kind/dashboard-up.sh"')
        block = UP[start:end]

        def run(fresh):
            work = self.dir / ('up-' + fresh)
            work.mkdir()
            script = ('set -euo pipefail\nroot=$TEST_ROOT\nfresh=' + fresh + '\n'
                      'bash() { printf "%s\\n" "$*" >>"$TEST_WORK/args"; }\n' + block)
            env = dict(os.environ, TEST_ROOT=str(ROOT), TEST_WORK=str(work))
            result = subprocess.run(['bash', '-c', script], env=env, text=True,
                                    capture_output=True, timeout=10)
            lines = [line.replace(str(ROOT) + os.sep, '')
                     for line in (work / 'args').read_text().splitlines()]
            return result, lines

        result, lines = run('0')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('scripts/kind/refresh-images.sh', lines, lines)
        self.assertLess(lines.index('scripts/kind/demo-preflight.sh --deploy'),
                        lines.index('scripts/kind/refresh-images.sh'), lines)

        result, lines = run('1')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('scripts/kind/build-images.sh', lines, lines)
        self.assertIn('scripts/kind/load-images.sh', lines, lines)
        preflight = lines.index('scripts/kind/demo-preflight.sh --deploy')
        self.assertLess(lines.index('scripts/kind/build-images.sh'), preflight, lines)
        self.assertLess(lines.index('scripts/kind/load-images.sh'), preflight, lines)
        self.assertNotIn('scripts/kind/refresh-images.sh', lines, lines)


if __name__ == '__main__':
    unittest.main(verbosity=2)
