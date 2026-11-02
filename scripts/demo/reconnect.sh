#!/usr/bin/env bash
# Rehace las conexiones locales de la demo sin reconstruir nada:
#   1. port-forward de go-intake en 127.0.0.1:18622 (lo que el túnel cloudflared publica);
#   2. dashboard (vite en 18610 y sus port-forwards 18600/18601/18603), vía scripts/kind/dashboard-up.sh;
#   3. comprueba que la URL pública entrega el /healthz de go-intake.
# Un `kubectl port-forward` queda pegado al pod que eligió: si ese pod se recrea (refresh-images, up.sh,
# cambio de modelo), el proceso sigue escuchando pero ya no responde. El síntoma es un 500 sin cuerpo en el
# login del dashboard o un 502 en las entregas del webhook de GitHub. Este script lo arregla.
# No toca el clúster ni el servicio cloudflared. Nunca imprime secretos. Sale 0 solo si todo responde.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1
# shellcheck disable=SC1091
. "$root/scripts/kind/lib.sh"

require_kind_context

P_INTAKE=18622
PF_LOG=/tmp/aqs-demo-pf-intake.log
PF_PID=/tmp/aqs-demo-pf-intake.pid
PUBLIC_URL=${AQS_DEMO_PUBLIC_URL:-https://aqs.ogaston.com}
PUBLIC_URL=${PUBLIC_URL%/}

fail() { echo "reconnect: FALLA $*" >&2; exit 1; }

code_of() { curl -s -o /dev/null -w '%{http_code}' --max-time "${2:-3}" "$1" 2>/dev/null || true; }

# 1. go-intake: se mata solo un `kubectl port-forward … svc/go-intake 18622:8080` (el de up.sh, el del
# preflight o uno abierto a mano con el mismo comando). Candidatos: el pid guardado y el dueño del puerto.
ours() { [[ "$1" =~ ^[0-9]+$ ]] && [ -r "/proc/$1/cmdline" ] &&
  tr '\0' ' ' <"/proc/$1/cmdline" | grep -Eq "kubectl .*port-forward .*svc/go-intake $P_INTAKE:8080"; }
cands=$(cat "$PF_PID" 2>/dev/null || true)
cands="$cands $(ss -ltnpH "sport = :$P_INTAKE" 2>/dev/null | grep -o 'pid=[0-9]*' | cut -d= -f2 | sort -u | tr '\n' ' ')"
for pid in $cands; do
  ours "$pid" || continue
  kill "$pid" 2>/dev/null || true
  # Un kubectl pegado a un pod muerto puede tardar en salir: 10 s y luego SIGKILL.
  for _ in $(seq 1 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
  kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
done
rm -f "$PF_PID"
for _ in $(seq 1 25); do (exec 3<>"/dev/tcp/127.0.0.1/$P_INTAKE") 2>/dev/null || break; sleep 0.2; done
if (exec 3<>"/dev/tcp/127.0.0.1/$P_INTAKE") 2>/dev/null; then
  fail "127.0.0.1:$P_INTAKE sigue ocupado por un proceso que esta demo no arrancó (mira: ss -ltnp | grep $P_INTAKE)"
fi
nohup kubectl --context "$KIND_CONTEXT" -n aqs-system port-forward --address 127.0.0.1 \
  svc/go-intake "$P_INTAKE:8080" >"$PF_LOG" 2>&1 </dev/null &
echo $! >"$PF_PID"
ok=0
for _ in $(seq 1 30); do
  [ "$(code_of "http://127.0.0.1:$P_INTAKE/healthz")" = 200 ] && { ok=1; break; }
  sleep 1
done
[ "$ok" -eq 1 ] || fail "go-intake no responde en 127.0.0.1:$P_INTAKE (log en $PF_LOG)"
echo "OK go-intake en 127.0.0.1:$P_INTAKE"

# 2. Dashboard: dashboard-up.sh es idempotente (para lo anterior y arranca de nuevo).
bash "$root/scripts/kind/dashboard-up.sh" || fail "dashboard (scripts/kind/dashboard-up.sh)"

# 3. Túnel: la URL pública tiene que llegar al mismo go-intake.
code=000
for _ in $(seq 1 15); do
  code=$(code_of "$PUBLIC_URL/healthz" 5)
  [ "$code" = 200 ] && break
  sleep 2
done
[ "$code" = 200 ] || fail "$PUBLIC_URL/healthz respondió $code; revisa el conector: systemctl status cloudflared"
echo "OK túnel $PUBLIC_URL -> go-intake"
echo 'reconnect: OK'
