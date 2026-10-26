# U11-T05 — El control positivo de egreso del humo prueba el puerto correcto del proveedor LLM

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M8.2 (egreso acotado: el control debe poder demostrar que la política permite lo que debe y bloquea el resto).
**Depende de:** U11-T02 (PR #135; esta rama parte de `tarea/U11-T02`). **Tope: 3 rondas** (la ronda 1 fue el diagnóstico).

---

## Origen y re-especificación (2026-10-26)

La versión original de esta tarea suponía que `llm-egress-acotado` fallaba porque la IP del proveedor rota y la NetworkPolicy (construida con las IP resueltas una vez) no la cubría, y proponía fijar el host con `hostAliases`. **El diagnóstico del codificador (ronda 1, `bitacoras/U11-T05.md`) refutó esa hipótesis**:

- La NetworkPolicy permite `<IP pública del CDN>/32` en el puerto **443**; dentro del pod del planner 10 de 10 resoluciones dan esa IP, y en el host también.
- El fallo es del **propio control** de `scripts/kind/kind-smoke.sh` (línea ~347): calcula el puerto con `u.port or 80` y no tiene en cuenta el esquema. Con `LLM_BASE_URL=https://<host>` sin puerto explícito prueba el **80**, que la política (correctamente) no permite. Desde el pod, el 443 conecta y el 80 da timeout. Es determinista.

Es decir, la política es correcta y el humo mentía. Por eso la tarea pasa a **arreglar el control**; el alias de host se descarta (no hay evidencia de rotación; queda como candidata si algún día se observa).

## Alcance

**Dentro** (una línea, concreta):

> El control positivo `llm-egress-acotado` de `kind-smoke.sh` deriva el puerto de `LLM_BASE_URL` con la regla del esquema (`https` → 443, `http` → 80, puerto explícito → ese), mediante una función con prueba propia, sin debilitar el control negativo (`1.1.1.1:443` sigue bloqueado) ni la discriminación del control (un puerto no permitido sigue fallando).

Detalle:

1. **Función pura** (en `scripts/kind/lib.sh` o en el propio script, donde sea más simple y testeable sin clúster) que, dada una URL, devuelva `host` y `port` con la regla anterior; esquema distinto de `http`/`https` o URL sin host → error claro.
2. `llm_egress_acotado` usa esa función dentro del `kubectl exec` (puede pasar el puerto calculado como argumento o replicar la regla en el Python del pod, pero **la regla debe estar en un solo sitio con una prueba que la cubra**).
3. **Prueba sin clúster** (`scripts/test/u11-egress-port-test.sh`, o ampliar `u11-scripts-test.sh`): `https://h` → 443; `http://h` → 80; `https://h:8443/v1` → 8443; `http://h:4000/v1` → 4000; `https://h/v1` → 443; `ftp://h`, vacío y sin host → error.
4. **Sensibilidad**: con la regla antigua (`u.port or 80`) la prueba debe fallar en `https://h` (par falla-pasa).
5. Documentar en la bitácora el diagnóstico ya hecho (no repetirlo) y las candidatas.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- `hostAliases`, cambios en la NetworkPolicy, en `secrets.sh`, en los manifiestos de `deploy/` o en los agentes: **la política es correcta y no se toca**.
- Cambiar el control negativo (`1.1.1.1:443`) o relajarlo.
- Otros controles del humo (`llm-real-configurado`, etc.).

---

## Archivos de contexto

- `scripts/kind/kind-smoke.sh` (`llm_egress_acotado`, ~línea 340), `scripts/kind/lib.sh`
- `scripts/test/u11-scripts-test.sh` (estilo de las pruebas sin clúster)
- `bitacoras/U11-T05.md` (diagnóstico de la ronda 1), `tareas/U8-T09-llm-app-y-github-reales.md`

---

## Criterios de aceptación

- [ ] **CA-1** — La regla del puerto está en una función con prueba, y la prueba detecta la regla vieja.
  ```bash
  bash scripts/test/u11-egress-port-test.sh; echo "rc=$?"
  ```
  Esperado: termina en `OK` y `rc=0`, con los casos de la lista (443, 80, 8443, 4000, `https://h/v1`→443, errores para `ftp://`, vacío y sin host). Par falla-pasa pegado en la bitácora: con `u.port or 80` el caso `https://h` falla.

- [ ] **CA-2** — `kind-smoke.sh` usa esa regla y no tocó nada más del humo.
  ```bash
  bash -n scripts/kind/kind-smoke.sh && echo SINTAXIS_OK; grep -c 'u.port or 80' scripts/kind/kind-smoke.sh; git diff --stat $(git merge-base HEAD origin/tarea/U11-T02) -- scripts/kind/kind-smoke.sh | tail -1
  ```
  Esperado: `SINTAXIS_OK`, `0` y un diff acotado a `llm_egress_acotado` (más la función si vive en el mismo archivo).

- [ ] **CA-3** — Lectura de vuelta en un clúster real (`kind-aqs`): el control positivo pasa de forma estable y sigue discriminando. Requiere red al proveedor y los insumos del humano (`${XDG_STATE_HOME:-~/.local/state}/aqs-kind/`); no se imprimen valores sensibles.
  ```bash
  bash scripts/kind/kind-up.sh && bash scripts/kind/build-images.sh && bash scripts/kind/load-images.sh && bash scripts/kind/secrets.sh && bash scripts/kind/deploy.sh
  for i in 1 2 3; do bash scripts/kind/kind-smoke.sh 2>&1 | grep -E '^(OK|FALLA) llm-egress-acotado'; done
  kubectl --context kind-aqs -n aqs-system exec deploy/agent-planner -- python3 -c 'import os,socket;from urllib.parse import urlsplit;u=urlsplit(os.environ["LLM_BASE_URL"]);
try: socket.create_connection((u.hostname,80),timeout=5);print("PUERTO80_ALCANZABLE")
except Exception: print("PUERTO80_BLOQUEADO")'
  bash scripts/kind/kind-smoke.sh 2>&1 | grep -c '^FALLA'
  ```
  Esperado: tres `OK llm-egress-acotado`, `PUERTO80_BLOQUEADO` (la política sigue sin permitir el 80: el control sigue discriminando) y `0` líneas `FALLA` en todo el humo. Si el entorno no permite llegar al proveedor o a podman, se declara en la bitácora y lo corre el revisor o el humano; no se da por cumplido. Bajar el clúster al terminar.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T02); git diff --name-only $b | grep -v -E '^(scripts/kind/(kind-smoke|lib)\.sh|scripts/test/|tareas/U11-T05-|bitacoras/U11-T05\.md|revisiones/U11-T05/)' | wc -l; git diff --name-only $b -- deploy services agents | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** la prueba de CA-1 contra la regla vieja falla en `https://h`; y en el clúster el humo da `FALLA llm-egress-acotado` (ya reproducido en la ronda 1: cítalo, no lo repitas si ya está en la bitácora).
- Negativas: esquema raro, URL vacía, puerto no numérico, URL con credenciales (`https://u:p@h/` → host `h`, y no se imprimen las credenciales).
- Sensibilidad en clúster: el puerto 80 sigue bloqueado y `1.1.1.1:443` sigue bloqueado.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T05`, worktree `../agentic-qa-swarm-wt-U11-T05`, desde `tarea/U11-T02`.
- Los insumos LLM del humano viven en `${XDG_STATE_HOME:-~/.local/state}/aqs-kind/`. Nunca se imprimen ni se escriben. La IP pública del CDN del proveedor puede figurar en la bitácora; la del gateway privado, nunca.
- Candidatas que surjan (p. ej. alias de host solo si se observa rotación real) van a la bitácora; `tareas/candidatas.md` queda fuera del alcance de esta tarea.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
