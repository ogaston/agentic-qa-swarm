# U9-T05 — Los insumos de kind sobreviven al reinicio de la máquina (C-127)

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** US-M10 (plataforma operable en kind). Cierra **C-127**.
**Depende de:** U8 cerrada. **Ola 1**, en paralelo con U9-T01 y U9-T02. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Mover los insumos del humano y las credenciales demo de `${XDG_RUNTIME_DIR}/aqs-kind` (tmpfs) a un directorio persistente con permisos 700/600, y dar un camino de recuperación cuando se pierden.

Detalle:

- **Directorio.** `AQS_KIND_STATE_DIR`, por defecto `${XDG_STATE_HOME:-$HOME/.local/state}/aqs-kind` (fuera del repo). Una sola definición en `scripts/kind/lib.sh`; todos los scripts de `scripts/kind/` que hoy usan `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind` la usan. Los locks (`aqs-kind.lock`) y estados efímeros (pids de port-forward, `aqs-dashboard`) **se quedan** en `XDG_RUNTIME_DIR`.
- **Permisos.** El directorio se crea 700 y cada archivo 600; un script que encuentra permisos más abiertos falla con un mensaje que nombra el archivo (no su contenido).
- **Migración.** Si el directorio nuevo no tiene un archivo y el antiguo de `XDG_RUNTIME_DIR` sí, se mueve una vez y se avisa.
- **Recuperación.** `secrets.sh --rotate-demo`: regenera solo `go-identity-users` (y `demo.txt`, `admin.txt`, `admin-mfa-secret.txt`) sin exigir `litellm.env` ni `deepseek.key`, comprueba que los hashes no están vacíos **antes** de reemplazar el Secret (nunca borra el Secret viejo hasta tener el nuevo válido) y reinicia `go-identity` esperando el rollout. Si la imagen `go-identity` no está en podman local, la exporta del nodo kind (`ctr -n k8s.io images export`) o falla con un mensaje accionable; nunca usa un hash vacío.
- **Falta de insumos del LLM.** Si falta `litellm.env` o `deepseek.key`, `secrets.sh` (sin `--rotate-demo`) sigue fallando antes de tocar el clúster, pero el mensaje dice dónde ponerlos (ruta nueva) y que el resto de Secrets existentes no se tocan.
- **Documentación.** `docs/operaciones/kind-local.md`: sección «Tras reiniciar la máquina» (`podman start aqs-control-plane`, comprobación de pods, `--rotate-demo` si falta `demo.txt`).

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cifrar los archivos en disco (sops/age) o integrarlos con un gestor de contraseñas.
- Cambios en los manifiestos o en los servicios.
- Que `kind-up.sh` arranque solo el contenedor parado (puede ir a `candidatas.md`).

---

## Archivos de contexto

- `scripts/kind/lib.sh`, `scripts/kind/secrets.sh` (`gen_go_identity_users`, validación de insumos), `scripts/kind/github-webhook-check.sh`, `scripts/kind/kind-e2e.sh`, `scripts/kind/dashboard-*.sh`
- `docs/operaciones/kind-local.md`, `docs/operaciones/webhook-github-cloudflared.md`
- `tareas/candidatas.md` (C-127) y `aidlc-docs/audit.md` (entrada del cierre de U8: cómo se perdió `go-identity`)

---

## Criterios de aceptación

- [ ] **CA-1** — Sin rutas de estado en tmpfs.
  ```bash
  grep -rn 'XDG_RUNTIME_DIR:-/tmp}/aqs-kind\b' scripts/kind/ | grep -v -E 'aqs-kind\.lock|lib\.sh' | wc -l; shellcheck scripts/kind/*.sh; echo "rc=$?"
  ```
  Esperado: `0` y `rc=0` (la única referencia restante es la migración en `lib.sh`).

- [ ] **CA-2** — Pruebas de los scripts sin clúster.
  ```bash
  bash scripts/test/u9-kind-state.sh 2>&1 | grep -E '^(OK|FALLA) '; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: `rc=0` y, como mínimo, `OK` en: `dir-700-archivos-600`, `permisos-abiertos-falla`, `migracion-desde-runtime`, `rotate-demo-sin-insumos-llm`, `rotate-demo-hash-vacio-no-toca-secret` (con `kubectl` y `podman` falsos en `PATH` que registran llamadas: con hash vacío no hay ningún `delete`/`create`/`apply` de Secret), `falta-litellm-mensaje-ruta`.

- [ ] **CA-3** — En kind (dentro de `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`).
  ```bash
  bash scripts/kind/secrets.sh --rotate-demo; echo "rc=$?"; bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: `rc=0` y `0`; `demo.txt` existe en el directorio nuevo con modo 600.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(scripts/kind/|scripts/test/u9-kind-state\.sh|docs/operaciones/|tareas/U9-T05-insumos-persistentes-kind\.md|bitacoras/U9-T05\.md|revisiones/U9-T05/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- `scripts/test/u9-kind-state.sh` trabaja en un `HOME` y `XDG_RUNTIME_DIR` temporales; nunca toca los archivos reales del humano.
- **Rojo primero:** pegar la salida de `scripts/test/u9-kind-state.sh` antes de implementar.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
