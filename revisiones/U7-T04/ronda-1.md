# Ronda 1 — U7-T04

VEREDICTO: NO-VERDE

Los 5 CA pasan en mi ejecución fresca. Hay dos hallazgos NARANJA en pie y ningún ROJO.

Corrí `kind-up`, `build-images`, `load-images` y `deploy` sobre `kind-aqs`. `kind-up` funcionó al primer intento, sin el fallo transitorio. Al terminar, `kind-down` borró el clúster y el contexto quedó en `kind-ckad`. `aqs-poc` y `ckad` siguen intactos. El worktree quedó limpio (`git status --short | wc -l` da 0). No quedan port-forwards en 186xx ni directorios `aqs-smoke.*` en `/run/user/1000`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Humo completo | El de la tarea, literal | pasa: `22 OK`, ninguna `FALLA`, `rc=0` |
| 2 | Sensibilidad de red | El de la tarea, literal | pasa: `1` y `1` (con las 4 NetworkPolicy borradas sale `FALLA netpol-sin-egress-test` / «1.1.1.1:443 alcanzable»; repuestas, `OK`) |
| 3 | Nada colgado | El de la tarea, literal | pasa: `0` y `0` |
| 4 | Pendientes y runbook | El de la tarea, literal | pasa: `2` (≥2) y `8` (≥6) |
| 5 | Alcance | El de la tarea con `git merge-base HEAD origin/main` | pasa: `0` y `0` |

Más datos de la ejecución:
- **CA-5:** el merge-base es `acd980a`. El diff toca 4 archivos: `bitacoras/U7-T04.md`, `deploy/flux/kind/kustomization.yaml`, `docs/operaciones/kind-local.md` y `scripts/kind/kind-smoke.sh`. `base`, `dev`, `prod` y `policy/` no cambian.
- **`policies.sh`:** `rc=0`, 0 `FALLA`, 16 `OK` (incluye los de `kind`).
- **E-3:** `warm-estado` da `OK` con el overlay de kind. La NetworkPolicy no hizo falta: `ui-api` llega a `go-warm-manager` por `allow-same-namespace`.
- **Reglas del orquestador:** ninguna comprobación se apoya en `kind-stub`. El reset verificado sale como `PENDIENTE` (C-104).

## Pruebas de sensibilidad propias
- **RBAC:** creé un Role y RoleBinding que dan `get secrets` en `aqs-system` a la SA `go-reset`. La comprobación pasó a `FALLA rbac-test-ns-only` con «difiere go-reset aqs-system get secrets: esperado=no obtenido=yes». Después borré el binding. Lee de vuelta con `kubectl auth can-i` sobre 118 filas, no lee manifiestos.
- **Logs:** inyecté `password_hash` en `go-identity` y un token de servicio en `go-governance`, escribiendo en `/proc/1/fd/1`. Resultado: `FALLA secrets-no-en-logs` con «coincidencias: 2». La comprobación es sensible.
- **Egress:** probé aparte, con un pod mío, el ClusterIP del API server (`kubernetes.default`, 10.96.0.1:443). Con las políticas puestas no se alcanza (`rc=1`); con las políticas borradas sí (`rc=0`).
- **Señal TERM a mitad de ejecución:** `rc=130`, sin port-forwards, sin pod de prueba y sin directorio temporal.

## Hallazgos
### F-01 · NARANJA · `scripts/kind/kind-smoke.sh:217-224` · `netpol-sin-egress-test` da `OK` sin Internet y su segundo testigo no mide la política de `aqs-test`
- El testigo de egress hacia fuera es `1.1.1.1:443`. Si el nodo no tiene salida a Internet, `nc` falla con o sin política y la comprobación da `OK` sin probar nada. El script no lo detecta y el runbook solo lo avisa en prosa.
- El otro testigo, `go-identity.aqs-system:8080`, no sirve. `aqs-system` tiene `default-deny-ingress` y `allow-same-namespace`, así que lo bloquea su ingress. En mi corrida con las políticas de `aqs-test` borradas (`smoke-nonp.out`), solo falló el testigo `1.1.1.1`; `go-identity` seguía sin alcanzarse.
- Por eso, sin Internet, la comprobación no distingue «política aplicada» de «sin red». El runbook dice que `go-identity` demuestra el aislamiento, y eso es impreciso.
- Arreglo propuesto: añadir un testigo que no dependa de Internet y que sí se abra sin políticas. El ClusterIP de `kubernetes.default` (`kubectl get svc kubernetes -n default`, puerto 443) cumple. Lo comprobé arriba.
- Alternativa mínima: que el script emita `PENDIENTE` o `FALLA` si el testigo externo no se pudo validar.

### F-02 · NARANJA · `docs/operaciones/kind-local.md:~48` y `:~54` · El runbook sigue diciendo que `warm-estado` está bloqueado
- La tabla «Qué funciona» dice: «Estado del warm desde ui-api (`GET /warm`) | Bloqueado: el Deployment de ui-api no define `WARM_URL`… | `warm-estado` (FALLA, defecto de manifiesto…)».
- Tras E-3 esa comprobación da `OK`: en mi corrida `GET /warm` devolvió 200 con un `state` válido. La documentación contradice el estado real.
- Entregar «qué funciona y qué queda pendiente» es parte del alcance (CA-4). Hay que cambiar la fila a «Funciona» (`warm-estado`, con la nota E-3 de solo kind).
- La bitácora no menciona que el runbook quedó desactualizado.

### F-03 · AMARILLO · `scripts/kind/kind-smoke.sh:40-52` · Una segunda señal durante `cleanup` deja el directorio temporal
- Pasa si llega otro `TERM` o `INT` mientras `cleanup` espera el borrado del pod: `trap 'exit 130'` interrumpe la limpieza antes de `rm -rf "$work"`.
- Me ocurrió sin buscarlo: quedó `/run/user/1000/aqs-smoke.klkDBG` con `login.json` (contraseña demo), `patterns` (tokens de servicio) y `logs.txt`. Tiene modo 700 en tmpfs de usuario, por eso es amarillo. Lo borré a mano.
- Mejora: ignorar señales dentro de `cleanup` (`trap '' INT TERM` al entrar) o borrar `$work` antes de esperar el pod.

### F-04 · AMARILLO · `scripts/kind/kind-smoke.sh:229-248` · Logs vacíos dan `OK` en `secrets-no-en-logs`
`go-reset` tiene 0 líneas de log y `go-warm-manager` tiene 1. Es una prueba de ausencia legítima. Pero si `kubectl logs` devolviera vacío por otra causa, el resultado sería igual. Basta con exigir que el total de `logs.txt` sea mayor que 0.

## Respuestas a lo que pediste juzgar
- **Red.** El positivo del control (`warm-app:80`) sí prueba que el pod tiene red. Con Internet, el negativo `1.1.1.1:443` es sensible: el CA-2 y mi corrida lo demuestran. Sin Internet se vuelve vacuo (F-01).
- **RBAC.** Prueba lo que dice. Lee de vuelta del clúster y falla al introducir un permiso extra.
- **Comprobaciones que dan `OK` sin leer de vuelta.** No encontré ninguna en la parte de estado. `inbox-con-token` solo exige un array JSON, sin comprobar contenido; es aceptable porque la bandeja está vacía en kind. Los `OK` por ausencia (red, logs) son los de F-01 y F-04.

## Tareas candidatas
- Los 4 testigos de red deberían ser comunes a `kind-smoke` y a futuras pruebas de aislamiento. Candidata: una función compartida en `scripts/kind/lib.sh`.

## Rutas de transcripciones largas
Salidas de mi corrida, en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/0ecca73d-838d-45a3-8b36-868c95cd55c0/scratchpad/`:
- `smoke1.out`
- `smoke-nonp.out`
- `smoke-rbac.out`
- `pol.out`

VEREDICTO: NO-VERDE
NARANJA|scripts/kind/kind-smoke.sh:217-224|netpol-sin-egress-test da OK sin Internet y el testigo go-identity no mide la política de aqs-test
NARANJA|docs/operaciones/kind-local.md:~48,~54|El runbook declara warm-estado bloqueado tras E-3 (ahora OK)
INFORME: revisiones/U7-T04/ronda-1.md
