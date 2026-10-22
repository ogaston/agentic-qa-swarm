#!/usr/bin/env bash
# U9-T05 (C-127): pruebas de los insumos persistentes de scripts/kind/ SIN clúster.
# Trabaja en un HOME y un XDG_RUNTIME_DIR temporales, con kubectl y podman falsos en PATH que
# registran sus llamadas. Nunca toca los insumos reales del humano. Salida: «OK <nombre>» o «FALLA <nombre>».
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/u9-kind-state.XXXXXX") || exit 1
trap 'rm -rf "$T"' EXIT

FIXTURE_SECRET_VALUE="VALOR-SECRETO-NO-IMPRIMIR-9f3a"
fails=0
ok()   { echo "OK $1"; }
falla() { echo "FALLA $1: $2"; fails=$((fails + 1)); }

# Entorno aislado por caso: HOME, XDG_RUNTIME_DIR y bin falso propios.
new_env() {
  rm -rf "$T/case" && mkdir -p "$T/case/home" "$T/case/run" "$T/case/bin"
  : >"$T/case/calls.log"
  export HOME="$T/case/home" XDG_RUNTIME_DIR="$T/case/run"
  unset XDG_STATE_HOME AQS_KIND_STATE_DIR
  export PATH="$T/case/bin:$PATH"
  export FAKE_HASH_EMPTY="" FAKE_IMAGE_MISSING=""
  cat >"$T/case/kustomize.yaml" <<YAML
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      volumes:
      - secret:
          secretName: go-identity-users
YAML
  cat >"$T/case/bin/kubectl" <<FAKE
#!/usr/bin/env bash
echo "kubectl \$*" >>"$T/case/calls.log"
case "\$1" in
  config) echo kind-aqs ;;
  kustomize) cat "$T/case/kustomize.yaml" ;;
  apply) cat >/dev/null ;;
  get) exit 1 ;;
  *) : ;;
esac
exit 0
FAKE
  cat >"$T/case/bin/podman" <<FAKE
#!/usr/bin/env bash
echo "podman \$*" >>"$T/case/calls.log"
case "\$1" in
  image) [ -z "\${FAKE_IMAGE_MISSING:-}" ] ;;
  run) cat >/dev/null; [ -z "\${FAKE_HASH_EMPTY:-}" ] && echo '\$argon2id\$v=19\$fake' ;;
  *) : ;;
esac
exit 0
FAKE
  chmod +x "$T/case/bin/kubectl" "$T/case/bin/podman"
}

secret_mutations() { grep -E '^kubectl (apply|create|delete|replace|patch)' "$T/case/calls.log" | wc -l; }

# 1. dir-700-archivos-600: rotate-demo con hashes válidos deja el directorio 700 y los ficheros 600.
new_env
out=$(bash "$root/scripts/kind/secrets.sh" --rotate-demo 2>&1); rc=$?
sd="$HOME/.local/state/aqs-kind"
if [ $rc -eq 0 ] && [ "$(stat -c %a "$sd" 2>/dev/null)" = 700 ] \
   && [ "$(stat -c %a "$sd/demo.txt" 2>/dev/null)" = 600 ] \
   && [ "$(stat -c %a "$sd/admin.txt" 2>/dev/null)" = 600 ] \
   && [ "$(stat -c %a "$sd/admin-mfa-secret.txt" 2>/dev/null)" = 600 ]; then
  ok dir-700-archivos-600
else falla dir-700-archivos-600 "rc=$rc salida: $out"; fi

# 2. permisos-abiertos-falla: un fichero 644 hace fallar y se nombra el archivo, no su contenido.
new_env
mkdir -p "$HOME/.local/state/aqs-kind" && chmod 700 "$HOME/.local/state/aqs-kind"
printf '%s\n' "$FIXTURE_SECRET_VALUE" >"$HOME/.local/state/aqs-kind/demo.txt"
chmod 644 "$HOME/.local/state/aqs-kind/demo.txt"
out=$(bash -c ". '$root/scripts/kind/lib.sh'; kind_state_check" 2>&1); rc=$?
if [ $rc -ne 0 ] && grep -q 'demo.txt' <<<"$out" && ! grep -q "$FIXTURE_SECRET_VALUE" <<<"$out"; then
  ok permisos-abiertos-falla
else falla permisos-abiertos-falla "rc=$rc salida: $out"; fi

# 3. migracion-desde-runtime: un fichero antiguo de tmpfs se mueve una vez al directorio nuevo.
new_env
mkdir -p "$XDG_RUNTIME_DIR/aqs-kind" && printf 'x\n' >"$XDG_RUNTIME_DIR/aqs-kind/admin.txt"
out=$(bash -c ". '$root/scripts/kind/lib.sh'; kind_state_migrate" 2>&1); rc=$?
if [ $rc -eq 0 ] && [ -f "$HOME/.local/state/aqs-kind/admin.txt" ] \
   && [ ! -e "$XDG_RUNTIME_DIR/aqs-kind/admin.txt" ] \
   && [ "$(stat -c %a "$HOME/.local/state/aqs-kind/admin.txt")" = 600 ] \
   && grep -q 'admin.txt' <<<"$out"; then
  ok migracion-desde-runtime
else falla migracion-desde-runtime "rc=$rc salida: $out"; fi

# 4. rotate-demo-sin-insumos-llm: no exige litellm.env ni deepseek.key.
new_env
out=$(bash "$root/scripts/kind/secrets.sh" --rotate-demo 2>&1); rc=$?
if [ $rc -eq 0 ] && ! grep -q 'litellm' <<<"$out"; then ok rotate-demo-sin-insumos-llm
else falla rotate-demo-sin-insumos-llm "rc=$rc salida: $out"; fi

# 5. rotate-demo-hash-vacio-no-toca-secret: hash vacío => fallo y ninguna mutación de Secret.
new_env
export FAKE_HASH_EMPTY=1
out=$(bash "$root/scripts/kind/secrets.sh" --rotate-demo 2>&1); rc=$?
if [ $rc -ne 0 ] && grep -qi "hash" <<<"$out" && [ "$(secret_mutations)" = 0 ]; then ok rotate-demo-hash-vacio-no-toca-secret
else falla rotate-demo-hash-vacio-no-toca-secret "rc=$rc mutaciones=$(secret_mutations) salida: $out"; fi

# 6. falta-litellm-mensaje-ruta: sin --rotate-demo, falta litellm.env => mensaje con la ruta nueva y sin tocar el clúster.
new_env
out=$(bash "$root/scripts/kind/secrets.sh" 2>&1); rc=$?
if [ $rc -ne 0 ] && grep -q "$HOME/.local/state/aqs-kind/litellm.env" <<<"$out" \
   && grep -qi 'no se tocan' <<<"$out" && [ "$(secret_mutations)" = 0 ]; then
  ok falta-litellm-mensaje-ruta
else falla falta-litellm-mensaje-ruta "rc=$rc salida: $out"; fi

echo "resumen: $fails fallo(s)"
[ "$fails" -eq 0 ]
