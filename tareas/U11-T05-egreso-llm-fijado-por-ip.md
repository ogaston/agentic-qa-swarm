# U11-T05 — El planner y el reporter siempre salen por una IP que la NetworkPolicy permite

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M8.2 (egreso acotado), US-M3/M4/M9 con el modelo real.
**Depende de:** U11-T02 **fusionada** (comparte `scripts/kind/secrets.sh` y `deploy/flux/kind/kustomization.yaml`). **Tope: 3 rondas**.

---

## Origen (evidencia real de U11-T02, ronda 1)

Con el proveedor directo (`LITELLM_UPSTREAM_URL=https://<host>` en `litellm.env`, sin proxy), `kind-smoke.sh` falló con
`FALLA llm-egress-acotado: el planner no alcanza el gateway (control positivo del egreso)`. En `scripts/kind/secrets.sh` (líneas 76-86) la
NetworkPolicy se abre a las IPv4 que `getent ahostsv4 <host>` devuelve **en ese momento** (un `/32` por IP). Un host detrás de un CDN rota de IP:
el pod resuelve el nombre por su cuenta (DNS del clúster), puede obtener una IP que no estaba en la lista y la política la bloquea. Es un riesgo
real para la demo (falla de forma intermitente), no un error del humano. **La causa es una hipótesis hasta reproducirla** (el primer paso de la tarea).

## Alcance

**Dentro** (una línea, concreta):

> Reproducir y demostrar la causa, y hacer que `planner` y `reporter` resuelvan el host del proveedor exactamente a una de las IP permitidas (p. ej. `hostAliases` en sus pods con la primera IP resuelta por `secrets.sh`, manteniendo el nombre del host para TLS/SNI), con el control positivo `llm-egress-acotado` en verde y estable.

Detalle:

1. **Diagnóstico con evidencia:** en el clúster, comparar las IP a las que resuelve el pod del planner (`getent`/`nslookup` desde un pod con la misma política) contra los `ipBlock` renderizados de su NetworkPolicy, y registrar ambas en la bitácora (sin imprimir la clave ni valores sensibles; las IP de un proveedor público pueden quedar en la bitácora, pero **no** la IP del gateway privado del humano: ocúltala como `<GATEWAY_IP>` si `litellm.env` usa `LITELLM_UPSTREAM_IP`). Si la hipótesis es falsa, el codificador lo dice y propone la causa real antes de arreglar nada.
2. **Fijación:** `secrets.sh` (o el paso que renderiza la política y los Deployments de los agentes en `deploy/flux/kind`) añade `hostAliases` al planner y al reporter con la primera IP resuelta para el host del proveedor; el resto de IP siguen permitidas por si el pod usa otra ruta. Si el modo es `LITELLM_UPSTREAM_IP` (IP literal), no cambia nada.
3. **Guarda:** si `getent` devuelve cero IPv4 el script ya falla; se añade que, si la lista cambia entre `secrets.sh` y `deploy.sh`, se detecta (re-resolver en `deploy.sh` y avisar con instrucciones; sin cambiar de IP en silencio).
4. **Documentar** en `docs/operaciones/kind-local.md` por qué existe el alias y cómo se vuelve a ejecutar `secrets.sh` si el proveedor cambia de IP.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Proxy LiteLLM/forward-proxy dentro del clúster, egress por FQDN (requiere otro CNI), cambiar el modelo, claves o `litellm.env`.
- Cambios en agentes (Python), controlador, contratos; `deploy/flux/base` (el cambio es solo del overlay kind).
- Relajar la política de egreso (ningún `0.0.0.0/0`, ningún puerto extra).

---

## Archivos de contexto

- `scripts/kind/secrets.sh` (`litellm_var`, `LITELLM_UPSTREAM_IPS`), `deploy.sh`, `kind-smoke.sh` (comprobación `llm-egress-acotado`), `lib.sh`
- `deploy/flux/kind/kustomization.yaml`, `deploy/flux/kind/agents/`, `deploy/flux/base/agents.yaml`
- `docs/operaciones/kind-local.md`, `tareas/U8-T09-llm-app-y-github-reales.md`, `bitacoras/U11-T02.md` (CA-6)

---

## Criterios de aceptación

- [ ] **CA-1** — La causa está demostrada con evidencia (o refutada con otra).
  Comando: el que el codificador use para comparar, pegado en la bitácora, más
  ```bash
  grep -n -i 'causa\|hipótesis\|ipBlock' bitacoras/U11-T05.md | head -10
  ```
  Esperado: la bitácora contiene las IP que el pod resolvió y las permitidas por la política (con la del gateway privado ocultada), y una conclusión explícita.

- [ ] **CA-2** — El render lleva los alias y no relaja la política (sin clúster).
  ```bash
  kubectl kustomize deploy/flux/kind | python3 -c "import sys,yaml;docs=[d for d in yaml.safe_load_all(sys.stdin) if d];print(sum(1 for d in docs if d.get('kind')=='Deployment' and d['metadata']['name'] in ('agent-planner','agent-reporter') and d['spec']['template']['spec'].get('hostAliases')))"; bash scripts/ci/policies.sh 2>&1 | tail -2; echo "policies rc=${PIPESTATUS[0]}"
  ```
  Esperado: `2` (planner y reporter con `hostAliases`; si el render de los alias lo hace `secrets.sh`/`deploy.sh` en lugar del kustomize, el codificador ajusta el comando y lo anota) y `policies rc=0`. Prueba sin clúster (script de prueba con `getent`/`kubectl` simulados) de que la IP del alias es una de las `ipBlock` de la política renderizada y de que con cero IP se aborta.

- [ ] **CA-3** — Lectura de vuelta en un clúster real (`kind-aqs`): el control positivo pasa y es estable.
  ```bash
  bash scripts/kind/kind-up.sh && bash scripts/kind/build-images.sh && bash scripts/kind/load-images.sh && bash scripts/kind/secrets.sh && bash scripts/kind/deploy.sh
  for i in 1 2 3; do bash scripts/kind/kind-smoke.sh 2>&1 | grep -E '^(OK|FALLA) llm-egress-acotado'; done
  kubectl --context kind-aqs -n aqs-system get deploy agent-planner -o jsonpath='{.spec.template.spec.hostAliases}'; echo
  ```
  Esperado: tres `OK llm-egress-acotado` seguidos y los `hostAliases` visibles en el Deployment real (sin imprimir la IP del gateway privado en la salida pegada: `<GATEWAY_IP>`). Requiere red al proveedor; si el entorno no lo permite, se declara y lo corre el revisor o el humano.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T02); git diff --name-only $b | grep -v -E '^(scripts/kind/|scripts/test/|deploy/flux/kind/|docs/operaciones/kind-local\.md|tareas/U11-T05-|bitacoras/U11-T05\.md|revisiones/U11-T05/)' | wc -l; git diff --name-only $b -- deploy/flux/base services agents | wc -l
  ```
  Esperado: `0`, `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** reproducir `FALLA llm-egress-acotado` (o, si es intermitente, un bucle de N=10 resoluciones desde el pod que muestre una IP fuera de la política) y pegar la salida. Después el cambio y el mismo comando en verde.
- Negativas del script: host que no resuelve, lista vacía, `LITELLM_UPSTREAM_IP` literal (sin alias), URL mal formada.
- Par falla-pasa: quitar `hostAliases` y comprobar que la prueba de CA-2 falla.

---

## Notas

- Archivos que se **modifican en su sitio**; nada de duplicados con sufijo. Rama `tarea/U11-T05`, worktree `../agentic-qa-swarm-wt-U11-T05`, desde `tarea/U11-T02`.
- Los insumos LLM del humano viven en `${XDG_STATE_HOME:-~/.local/state}/aqs-kind/` (no en `XDG_RUNTIME_DIR`). Nunca se imprimen ni se escriben en bitácoras, informes o logs.
- Si el diagnóstico apunta a otra causa (p. ej. DNS, MTU, reloj), el codificador la documenta, propone la tarea y se detiene: no improvisa.
- Candidatas que surjan van a `tareas/candidatas.md`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
