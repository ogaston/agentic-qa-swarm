#!/usr/bin/env bash
# scripts/demo/10-push-commit.sh — Paso 1 y Paso 2 de docs/demo-real.md, automatizados.
#
# Prepara y empuja el commit de la demo en el clon local del repo objetivo
# (ogaston/demo-target-app-qa-swarm) y espera a que la CI publique la imagen
# ghcr.io/ogaston/demo-target-app-qa-swarm:<sha40> (solo push a main publica imagen).
#
# Modos:
#   --bug off      commit sano: quita ENV TARGET_APP_BUG=oversell si existe; commit
#                  --allow-empty si el Dockerfile ya está limpio (idempotente).
#   --bug oversell commit con el defecto: añade ENV TARGET_APP_BUG=oversell junto al
#                  ENV LISTEN_ADDR existente si falta; si ya existe no lo duplica.
#
# Orden estricto del guion: editar Dockerfile -> git add -> commit -> push.
# Nunca `git commit -am` antes de editar.
#
# Uso:
#   bash scripts/demo/10-push-commit.sh --repo-dir <ruta-al-clon> [--bug off|oversell]
#
# Variables opcionales:
#   ESPERA_IMAGEN_S     segundos máximos de espera a la imagen (defecto 900).
#   ESPERA_INTERVALO_S  segundos entre sondeos a GHCR (defecto 20).
#
# Salida (al final):
#   SHA=<sha40>
#   OK imagen-publicada          si la imagen existe en GHCR
#   FALLA ...                    con la causa, en cualquier otro caso
#
# Códigos de salida: 0 imagen publicada; 1 error de uso/validación/push;
# 2 la imagen no existe tras el timeout (404 en GHCR);
# 3 no se pudo comprobar la imagen (token anónimo, red o límite de GHCR).
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# shellcheck source=scripts/kind/lib.sh
source "$root/scripts/kind/lib.sh"   # TARGET_REPO_FULL/URL/IMAGE_REPO, target_image_exists

REPO_DIR=""
BUG=off
ESPERA_IMAGEN_S=${ESPERA_IMAGEN_S:-900}
ESPERA_INTERVALO_S=${ESPERA_INTERVALO_S:-20}

LINEA_BUG='ENV TARGET_APP_BUG=oversell'
ANCLA='ENV LISTEN_ADDR=:80 BASELINE_LISTEN_ADDR=:9090'

uso() {
  echo "uso: bash scripts/demo/10-push-commit.sh --repo-dir <ruta-al-clon> [--bug off|oversell]" >&2
}

falla() { # falla <mensaje> [rc]
  echo "FALLA $1" >&2
  exit "${2:-1}"
}

# --- argumentos ---------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --repo-dir) [ $# -ge 2 ] || { uso; exit 1; }; REPO_DIR=$2; shift 2 ;;
    --bug)      [ $# -ge 2 ] || { uso; exit 1; }; BUG=$2; shift 2 ;;
    -h|--help)  uso; exit 0 ;;
    *) uso; echo "FALLA argumento desconocido: $1" >&2; exit 1 ;;
  esac
done
[ -n "$REPO_DIR" ] || { uso; falla "falta --repo-dir"; }
case "$BUG" in off|oversell) ;; *) falla "--bug debe ser off u oversell (recibido: '$BUG')" ;; esac
[[ "$ESPERA_IMAGEN_S" =~ ^[0-9]+$ && "$ESPERA_INTERVALO_S" =~ ^[0-9]+$ ]] \
  || falla "ESPERA_IMAGEN_S y ESPERA_INTERVALO_S deben ser enteros de segundos"

for cmd in git curl jq awk; do
  command -v "$cmd" >/dev/null 2>&1 || falla "falta el comando $cmd"
done

# --- validar el clon ----------------------------------------------------------
[ -d "$REPO_DIR" ] || falla "--repo-dir '$REPO_DIR' no es un directorio"
git -C "$REPO_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
  || falla "'$REPO_DIR' no es un clon git"

remote=$(git -C "$REPO_DIR" config --get remote.origin.url 2>/dev/null) \
  || falla "el clon no tiene remote 'origin' (git remote add origin $TARGET_REPO_URL.git)"
remoto_norm=${remote%.git}
case "$remoto_norm" in
  "https://github.com/$TARGET_REPO_FULL"|"git@github.com:$TARGET_REPO_FULL"|"ssh://git@github.com/$TARGET_REPO_FULL") ;;
  *) falla "origin apunta a '$remote', no al repo objetivo $TARGET_REPO_URL" ;;
esac

rama=$(git -C "$REPO_DIR" symbolic-ref --short HEAD 2>/dev/null) \
  || falla "HEAD desacoplado en '$REPO_DIR': haz checkout de main"
[ "$rama" = main ] || falla "estás en la rama '$rama'; la demo empuja a main (git checkout main)"

[ -z "$(git -C "$REPO_DIR" status --porcelain)" ] \
  || falla "el clon tiene cambios sin confirmar; déjalo limpio antes de la demo"

git -C "$REPO_DIR" fetch origin main \
  || falla "git fetch origin main falló (¿red o credenciales?)"

head_sha=$(git -C "$REPO_DIR" rev-parse HEAD)
main_sha=$(git -C "$REPO_DIR" rev-parse origin/main)
if [ "$head_sha" != "$main_sha" ]; then
  if git -C "$REPO_DIR" merge-base --is-ancestor origin/main HEAD; then
    falla "tienes commits locales sin empujar a origin/main; la demo parte de main en sincronía"
  elif git -C "$REPO_DIR" merge-base --is-ancestor HEAD origin/main; then
    falla "origin/main va por delante de tu clon: git pull --ff-only y reintenta"
  else
    falla "HEAD y origin/main divergen; reconcilia el clon (fetch + rebase/reset) antes de la demo"
  fi
fi
echo "clon validado: $REPO_DIR (main = origin/main = ${head_sha:0:12}…)"

# --- editar el Dockerfile (antes de cualquier commit) --------------------------
DOCKERFILE="$REPO_DIR/Dockerfile"
[ -f "$DOCKERFILE" ] || falla "no existe $DOCKERFILE"

if [ "$BUG" = oversell ]; then
  if grep -qxF "$LINEA_BUG" "$DOCKERFILE"; then
    echo "Dockerfile ya tiene '$LINEA_BUG'; no se duplica"
  else
    grep -qxF "$ANCLA" "$DOCKERFILE" \
      || falla "no encuentro la línea ancla '$ANCLA' en $DOCKERFILE; añade el ENV a mano"
    tmp=$(mktemp)
    awk -v ancla="$ANCLA" -v linea="$LINEA_BUG" '{ print } $0 == ancla { print linea }' \
      "$DOCKERFILE" >"$tmp" && mv "$tmp" "$DOCKERFILE"
    echo "Dockerfile: añadida '$LINEA_BUG' tras el ENV existente"
  fi
else
  if grep -qxF "$LINEA_BUG" "$DOCKERFILE"; then
    tmp=$(mktemp)
    grep -vxF "$LINEA_BUG" "$DOCKERFILE" >"$tmp" && mv "$tmp" "$DOCKERFILE"
    echo "Dockerfile: quitada '$LINEA_BUG' (modo sano)"
  else
    echo "Dockerfile ya está limpio (sin '$LINEA_BUG')"
  fi
fi

# --- add -> commit -> push -----------------------------------------------------
if [ "$BUG" = oversell ]; then MSG="demo: fija el defecto oversell"; else MSG="demo: commit sano"; fi

if [ -n "$(git -C "$REPO_DIR" status --porcelain -- Dockerfile)" ]; then
  git -C "$REPO_DIR" add Dockerfile
  git -C "$REPO_DIR" commit -m "$MSG"
else
  git -C "$REPO_DIR" commit --allow-empty -m "$MSG"
fi
git -C "$REPO_DIR" push origin HEAD:refs/heads/main \
  || falla "git push origin HEAD:refs/heads/main falló (¿permiso de push a main?)"

SHA=$(git -C "$REPO_DIR" rev-parse HEAD)
[[ "$SHA" =~ ^[0-9a-f]{40}$ ]] || falla "git rev-parse HEAD devolvió un SHA mal formado: '$SHA'"
echo "commit empujado a main; esperando la imagen $TARGET_IMAGE_REPO:$SHA (máx. ${ESPERA_IMAGEN_S}s)"

# --- esperar a que la CI publique la imagen ------------------------------------
# target_image_exists (scripts/kind/lib.sh): 0 existe, 1 no existe (404), 2 no se pudo comprobar.
limite=$((SECONDS + ESPERA_IMAGEN_S))
ultimo_rc=1
while [ "$SECONDS" -lt "$limite" ]; do
  ultimo_rc=0
  target_image_exists "$SHA" || ultimo_rc=$?
  [ "$ultimo_rc" = 0 ] && break
  sleep "$ESPERA_INTERVALO_S"
done

echo "SHA=$SHA"
if [ "$ultimo_rc" = 0 ]; then
  echo "OK imagen-publicada"
  exit 0
elif [ "$ultimo_rc" = 1 ]; then
  echo "FALLA imagen-no-publicada: tras ${ESPERA_IMAGEN_S}s GHCR sigue respondiendo 404 para $TARGET_IMAGE_REPO:$SHA; revisa la pestaña Actions de $TARGET_REPO_URL (solo push a main publica imagen)" >&2
  exit 2
else
  echo "FALLA imagen-no-comprobable: tras ${ESPERA_IMAGEN_S}s no pude comprobar $TARGET_IMAGE_REPO:$SHA (token anónimo, red o límite de GHCR); no es que falte: reintenta más tarde" >&2
  exit 3
fi
