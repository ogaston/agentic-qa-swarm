#!/usr/bin/env bash
# CA-2 de U8-T05: usuario MinIO aqs-evidence con permisos mínimos, leído de vuelta en el clúster kind `aqs`.
# Usa las credenciales del Secret aqs-evidence-s3 (nunca impresas) desde un pod con la imagen de MinIO fijada
# y su mc. Comprobaciones: put y get en evidence; «sin-admin» (mc admin info falla); «otro-bucket-denegado».
# Imprime «OK|FALLA <comprobacion>» y sale 0 solo si todas pasan. Limpia su pod y sus objetos siempre (trap).
# Solo actúa sobre el clúster kind `aqs` (guarda require_kind_context, sale 3 en otro contexto).
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck disable=SC1091  # lib.sh se carga por ruta dinámica (relativa a root)
. "$root/scripts/kind/lib.sh"

require_kind_context # sale 3 si el contexto no es kind-aqs

NS=aqs-system
POD=u8-t05-mc
STAMP=$(date +%s)
RUN="u8t05-acceso-$STAMP"
OBJ="runs/$RUN/probe.txt"
OTRO="u8t05-otro-$STAMP"
probe="u8t05-$(date +%s%N)"
fail=0

cleanup() {
  mc_as root "$POD" rm --recursive --force "m/evidence/runs/$RUN" >/dev/null 2>&1
  mc_as root "$POD" rb --force "m/$OTRO" >/dev/null 2>&1
  pod_down "$NS" "$POD"
}
trap cleanup EXIT

check() { # <comprobacion> <comando...>: OK si el comando sale 0
  local name=$1 out
  shift
  if out=$("$@" 2>&1); then
    echo "OK $name"
  else
    echo "FALLA $name"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

denied() { # <comprobacion> <comando...>: OK si el comando falla por permisos (AccessDenied o «Insufficient permissions»)
  local name=$1 out
  shift
  if out=$("$@" 2>&1); then
    echo "FALLA $name (el comando funcionó y debía ser denegado)"
    fail=1
    return
  fi
  # MinIO responde «Access Denied» en sin-admin y «Insufficient permissions» en el bucket ajeno (AccessDenied).
  # Un usuario inexistente («Access Key Id ... does not exist») no cuenta como denegación: sale FALLA.
  if grep -qiE 'denied|insufficient permissions' <<<"$out"; then
    echo "OK $name"
  else
    echo "FALLA $name (fallo sin denegación explícita)"
    printf '%s\n' "$out" | sed 's/^/    /' >&2
    fail=1
  fi
}

put_probe() { # <destino m/...>: escribe el valor de prueba con el usuario de evidencia
  printf '%s' "$probe" | mc_as aqs "$POD" pipe "$1"
}

get_probe() { # <origen m/...>: lee de vuelta y compara con el valor escrito
  [ "$(mc_as aqs "$POD" cat "$1")" = "$probe" ]
}

mc_pod_up "$NS" "$POD" || { echo "FALLA pod-mc (no arranca: ¿existe minio?)"; exit 1; }

check put-evidence put_probe "m/evidence/$OBJ"
check get-evidence get_probe "m/evidence/$OBJ"
denied sin-admin mc_as aqs "$POD" admin info m
mc_as root "$POD" mb "m/$OTRO" >/dev/null 2>&1 || { echo "FALLA preparar-otro-bucket"; fail=1; }
denied otro-bucket-denegado put_probe "m/$OTRO/probe.txt"

if [ "$fail" -eq 0 ]; then echo "rc=0"; else echo "rc=1"; fi
exit "$fail"
