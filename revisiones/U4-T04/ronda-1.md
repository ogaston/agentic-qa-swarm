# Ronda 1 — U4-T04

VEREDICTO: NO-VERDE

Los 13 criterios pasan con la ejecución propia del revisor (`GOTOOLCHAIN=go1.26.8`, `-count=1`, binario real). El único hallazgo que bloquea es NARANJA (F-01). Worktree limpio. Fail-closed, aislamiento del token de servicio, cadena de auditoría y namespace salieron limpios.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | 268 PASS, 0 FAIL; `Matrix` 20 PASS frente a `jq length`=17 (3 pruebas de nivel superior + 17 subpruebas de `TestMatrixReal`, que usan `RuleEvaluator`; el fake solo está en `TestFakeWiringReplay`); `-race -count=5` sin intermitencias |
| 2 | los seis valores esperados exactos |
| 3 | `false` ×6, `[false]` ×3 |
| 4 | `sin=401 persona=401 roto=400`, `sin-allow` |
| 5 | todos los códigos y versiones esperados |
| 6 | `true true false / false / true / false` |
| 7 | literal: `["gate.deny","gate.deny"]` (arbitrado); con `"workflow_allowed":"true"` en la segunda llamada: `["gate.deny","gate.allow"]`; `delete=405 put=405 post=405 sin_token=401`; `integra rc=0` |
| 8 | con `chattr +i` sobre `audit.jsonl` y una petición que sí debería permitirse: antes `allow=true`; después `allow=false` (503) y el log queda envenenado; `PUT /policies/events` 503 sin guardar; disco lleno (tmpfs 24k): 7 permitidos y luego `false` sin excepción; las 6 pruebas `FailClosed*` pasan |
| 9 | `sin=401 user=403 admin=200`, `identidad_caida=503`; ningún token en `data/` |
| 10 | cuatro `rc=1` |
| 11 | contrato válido, 0 líneas eliminadas, `serviceToken \| get,put \| GateDecision,GateRequest` |
| 12 | `docker build` literal construyó; `65532:65532`; sin `latest`; `list-services`=1; `detect-lang`=go; sin `k8s.io` |
| 13 | `ok`, gitleaks sin hallazgos, 0, 0 |

CI de GitHub sobre `18c55b9`: `ci` success (incluidos `vuln (go-governance)`/govulncheck), `contracts` y `policies` success.

## Auditoría, adversarial
Alterar acción/detalle, borrar, reordenar, duplicar, truncar al final y línea en blanco → `verify-audit` rc=1 con la línea. **Borrar la última línea (o las dos últimas) y archivo vacío → rc=0** (límite conocido). El servicio no arranca con línea alterada/truncada ni con `policies.jsonl` truncado o con versiones no consecutivas.
Mutantes en copia: detectados (ignorar error de auditoría, `unknown` como verdadero, `>`/`>=` en cuota, no exigir `workflow_allowed` en `warm_ready`, `TrimSpace`/`ToLower` en namespace, quitar `prev_hash`, quitar detección de truncado, aceptar cualquier token, desactivar aprobación, permitir workflow inexistente, `user` como admin); sobrevivió equivalente (quitar `d.Allow=false` en el handler); sobrevivió no equivalente (quitar `Sync`, F-06).
Namespace: `aqs-test\n`, `\u0000`, guión Unicode, `​`, `е` cirílica, `aqs-test.evil`, `AQS-test`, `" aqs-test"` se deniegan. `run_id` de 50 KB aceptado; cuerpo de 70 KB → 413; 50 MB sin token → 401.
Concurrencia con el binario: 40 gates con tope 5 → exactamente 5 `true`; 25 `PUT` → versiones 1..25 sin huecos; la cuota diaria sobrevive al reinicio.

## Hallazgos
### F-01 · NARANJA · `authz/rules.go` (`gatedWorkflow`) · La política de workflows no se evalúa en `warm_ready` ni en `inferring`
La tarea dice, sin distinguir estado: «Si `workflow` no está vacío: debe existir en la política `workflows`, si no → Deny». El evaluador solo lo comprueba en `deploying`, `rehearsing` y `running`. Con `workflow:"nada"` inexistente: `confirmed→warm_ready` y `deploying→inferring` dan `allow=true`; `warm_ready→deploying`, `inferring→rehearsing` y `rehearsing→running` dan `false`. La bitácora justifica el recorte con «solo en los destinos que ya exigen `workflow_allowed`», falso para `warm_ready`. Un workflow retirado de la política sigue avanzando hasta `inferring` y consume entorno warm. Ninguna prueba fija la decisión. Salida: evaluar existencia en todo destino salvo `resetting`, `reporting`, `done` y `failed` (para que el reset siempre se intente), o enmendar la especificación y fijar el comportamiento con una prueba.

### F-02 · AMARILLO · Coherencia de `workflow_allowed` en `warm_ready`
Coherente con la matriz y con las invariantes de U4-T06. Matices: en `warm_ready` el hecho es autodeclarado (no se contrasta con la política, ver F-01); y el diseño pasa el workflow en `deployToWarm(…, workflow)`, posterior a `warm_ready`, así que U2-T02 tendrá que reportar `workflow_allowed` antes de que exista la selección de workflow: dejarlo explícito en la especificación.

### F-03 · AMARILLO · La auditoría no liga el contenido de la política
`policy.set` registra solo nombre y versión; `policies.jsonl` no es verificable (editar `max_runs_per_day` 1000→999 a mano arranca sin quejas). Propuesta: sha256 del valor en `policy.set` y versión+hash de la política usada en cada gate.

### F-04 · AMARILLO · Decodificación laxa
`"CONFIRMED":"true"` se acepta; con claves duplicadas gana la última; `PUT /policies` acepta basura posterior; los 422 filtran texto interno de Go. El llamante es un servicio de confianza.

### F-05 · AMARILLO · Arranque con lista negra
`fake` se permite con `GOVERNANCE_ENV` vacío o distinto de `prod`/`production` (literal a la tarea; candidata del codificador). `GOVERNANCE_TEST_NAMESPACE` solo rechaza `staging`, `prod`, `production`, `default`, `kube-system`; `aqs-prod`, `prod-eu`, `*` y `" "` arrancan.

### F-06 · AMARILLO · Cobertura
Ninguna prueba cubre que se llame a `Sync()`; el mutante sobrevive y la tarea exige `fsync` por entrada.

## Tareas candidatas (fuera de alcance)
- Omitir `workflow` y declarar `workflow_allowed=true` salta cuota y aprobación (compatibilidad con la matriz): exigir `workflow` no vacío desde `deploying` cuando exista política.
- Cadena sin clave ni ancla externa; truncado de cola y archivo vacío no se detectan.
- Disco lleno deja el servicio sin arrancar (línea parcial) sin procedimiento de reparación: runbook o subcomando de diagnóstico.
- Un `user` autenticado puede crecer la auditoría con `policy.rejected` sin límite de tasa.
- El gate no verifica el estado real de la corrida (`from` lo reporta el llamante): U2-T02 y C-50.
- Estado por réplica (auditoría y cuota en un archivo local).

VEREDICTO: NO-VERDE
NARANJA|authz/rules.go (gatedWorkflow), regla "Workflow permitido por política"|La política de workflows no se evalúa en warm_ready ni en inferring: un workflow inexistente recibe allow=true en confirmed->warm_ready y deploying->inferring
INFORME: revisiones/U4-T04/ronda-1.md
