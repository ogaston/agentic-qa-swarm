# Ronda 1 — U7-T05

VEREDICTO: VERDE

Base: worktree /home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U7-T05, rama tarea/U7-T05, sha b9b31a9. Blob de la tarea según el despacho: 90020b52. No tengo Write, así que el orquestador debe guardar este texto en revisiones/U7-T05/ronda-1.md.

El diff contra el merge-base toca 5 archivos: bitacoras/U7-T05.md, docs/operaciones/kind-local.md y scripts/kind/dashboard-{up,down,smoke}.sh. lib.sh, kind-smoke.sh, vite.config.ts y web/dashboard/src no cambian.

## Cómo corrí los criterios
Usé un único bloque `flock /run/user/1000/aqs-kind.lock bash block.sh`. Esperó en la cola unos 25 minutos detrás de otras sesiones (U8-T01, U8-T04). Dentro del bloque hice: kind-down (el clúster no existía), kind-up, build-images, load-images y deploy (`deploy: OK (overlay deploy/flux/kind en kind-aqs)`). Después corrí los criterios y terminé con kind-down. Fijé el contexto `kind-aqs` y al final lo restauré a `kind-ckad`. La salida completa está en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/out.log`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Recorrido | `bash scripts/kind/dashboard-smoke.sh 2>&1 \| grep -E '^(OK\|FALLA\|PENDIENTE)'; echo "rc=${PIPESTATUS[0]}"` | pasa: 7 OK (index-servido, csp-estricta, login, inbox, warm-estado, sin-token-401, logout-invalida-token), `PENDIENTE confirmar-y-seguir-corrida (P1)`, ninguna FALLA, rc=0 |
| 2 | Limpieza | `bash scripts/kind/dashboard-smoke.sh >/dev/null 2>&1; ss -ltn \| grep -c -E '127\.0\.0\.1:(186(0[0-6])\|18610)'` | pasa: `0`. Tampoco quedó el directorio de estado `aqs-dashboard` ni `aqs-dash-smoke.*` en `$XDG_RUNTIME_DIR` |
| 3 | Sin secretos | `bash scripts/kind/dashboard-smoke.sh > /tmp/u7t05.out 2>&1; grep -c -E 'password_hash\|\$argon2id\$\|Bearer [A-Za-z0-9_-]{20,}' /tmp/u7t05.out` | pasa: `0`. Comprobación adicional: la contraseña demo real (`demo.txt` línea 2) aparece 0 veces en la salida de 10 líneas |
| 4 | Humo de U7-T04 | `bash scripts/kind/kind-smoke.sh 2>&1 \| grep -c '^FALLA'` | pasa: `0` |
| 5 | Alcance | `git status --short` en el worktree después de todas las corridas, más el filtro del merge-base | pasa: `git status --short` vacío, 0 archivos fuera de alcance (lista de archivos arriba) |

La verificación del estado de vuelta es real. login, inbox y warm-estado pasan por el origen :18610 y su proxy /api hacia los port-forward. La sesión se lee con `GET /api/auth/session` (rol `user`). Tras el logout, el mismo token da 401 en esa ruta. Las llamadas curl de las comprobaciones solo van a `$ORIGIN` (:18610).

## Pruebas adicionales mías (sensibilidad)
- **CSP:** extraje el fragmento Python de `csp_estricta` y lo corrí contra cabeceras falsas. Con una CSP `default-src *` sale rc=1. Con la CSP ausente también sale rc=1. En ambos casos imprime `cabeceras distintas de la política: content-security-policy`. La comprobación sí discrimina.
- **Puerto ocupado:** con 127.0.0.1:18610 ocupado, el smoke imprime `FALLA dashboard-up`, sale rc=1 y no deja listeners (0 en el rango).
- **`bash -n`:** pasa en los tres scripts. `shellcheck` no está instalado, así que no lo pude correr.
- **SIGTERM a mitad del smoke:** mi intento no sirvió. El smoke terminó antes de los 12 s, así que el kill llegó tarde. No lo verifiqué. Por lectura, `trap 'exit 130' INT TERM` más `trap cleanup EXIT` lo cubre.

## Hallazgos
### F-01 · AMARILLO · bitacoras/U7-T05.md · Sin prueba de sensibilidad del lado del smoke
El codificador lo declaró. El rojo solo mostró la ausencia del script. Yo cubrí la CSP offline (arriba), pero `sin-token-401`, `inbox` y `warm-estado` no tienen mutación que demuestre que pasarían a FALLA. No bloquea: cada una compara un código HTTP o un valor concreto, no un rc.

### F-02 · AMARILLO · scripts/kind/dashboard-smoke.sh:60-70 · Política CSP duplicada en tres sitios
La cadena vive en `vite.config.ts`, en `scripts/test/u6-demo-local.sh` y ahora en el smoke. El criterio exige «la misma política que U6-T06», y hoy coincide. Si alguien cambia una de las copias, el smoke falla de forma ruidosa pero no hay una fuente única.

### F-03 · AMARILLO · scripts/kind/dashboard-smoke.sh:98-137 · Token y contraseña en argv
`jq --arg p "$pw"` y `-H "Authorization: Bearer $(cat token)"` dejan secretos visibles en `ps` mientras corre el proceso. Es local, en loopback, y de corta duración, y es el mismo patrón que usa U7-T04. No se imprimen en la salida, que es lo que exige CA-3.

### F-04 · AMARILLO · scripts/kind/dashboard-up.sh:46-52 · Los port-forward no se extrajeron a lib.sh
La tarea decía «reutiliza los port-forward de U7-T04 (extraídos a lib.sh si hace falta)». El codificador replicó el patrón (`pf`, `wait_url`) en vez de extraerlo. La cláusula «si hace falta» lo permite y no cambió kind-smoke, así que CA-4 sigue en 0. Es solo una duplicación menor.

## Tareas candidatas
- Mover a `lib.sh` las funciones `pf` y `wait_url` (kind-smoke y dashboard-up las duplican), con una prueba que demuestre que el comportamiento de kind-smoke no cambia.
- Una fuente única de la CSP, por ejemplo exportada desde `vite.config.ts` o leída de un archivo de contrato, para que el smoke y u6-demo-local no hardcodeen la cadena.

## Notas del bucle
- El worktree quedó limpio después de la revisión (`git status --short` vacío).
- El scratchpad de la sesión es compartido con otras sesiones; mis logs de despliegue pudieron mezclarse con los de ellas, pero solo afectó a la impresión del `tail` de deploy.log, no a ningún criterio.
- No hay ROJO ni NARANJA en pie, y no hubo desborde de alcance. Los cuatro hallazgos son AMARILLOS y no bloquean.
- El clúster `aqs` quedó destruido al terminar (`kind-down: clúster aqs eliminado`).

VEREDICTO: VERDE
AMARILLO|bitacoras/U7-T05.md|Sin prueba de sensibilidad del lado del smoke (CSP verificada por mí offline)
AMARILLO|scripts/kind/dashboard-smoke.sh:60-70|Política CSP duplicada en tres sitios
AMARILLO|scripts/kind/dashboard-smoke.sh:98-137|Token y contraseña en argv
AMARILLO|scripts/kind/dashboard-up.sh:46-52|Port-forward no extraídos a lib.sh
INFORME: revisiones/U7-T05/ronda-1.md
