# Ronda 1 — U8-T04

VEREDICTO: NO-VERDE

(No pude escribir `revisiones/U8-T04/ronda-1.md`: mis herramientas son de solo lectura. El orquestador debe guardar este texto en esa ruta.)

Base: tarea en blob 7f376b3 y worktree `agentic-qa-swarm-wt-U8-T04`, rama `tarea/U8-T04`, sha f02c097. El worktree quedó limpio tras mis ejecuciones (`git status --short` da 0 líneas). Usé el clúster `aqs` solo bajo `flock`, de kind-up a kind-down. Una vez `kind create` falló por un problema del entorno ("Reached target Multi-User System") y reintenté. Los comandos de clúster los corrí en dos bloques `flock` completos. Las transcripciones están en la sección final.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas de la app | `bash scripts/test/u8-target-app-local.sh` | **pasa**: 10 OK (`openapi-valido`, `invariante-stock-sin-defecto`, `defecto-oversell-sembrado`, `baseline-clean-verify-0`, `baseline-sucio-verify-mayor-que-0`, `baseline-sin-token-401`, entre otros), `rc=0`, 9,6 s. El tiempo es creíble: arranca Postgres por podman. |
| 2 | `baseline.sh` en la imagen de go-reset | `bash scripts/test/u8-target-app-local.sh --baseline-script` | **pasa**: `clean` rc 0, `verify` entero, `version` = `ref-v1`, `rc=0`. |
| 3 | Kind: deploy, superficie y reset | `deploy.sh`, lectura de la etiqueta `kind-stub` y `u8-target-app-kind.sh` bajo `flock` | **falla**. `deploy rc=1` (FALLA `warm-app`), etiqueta vacía (pasa), `u8-target-app-kind.sh` rc=1 (7 min). Ver F-01, F-02 y F-03. |
| 4 | Políticas y alcance | `policies.sh \| grep -c '^FALLA'`; `git status --short \| wc -l`; fuera de alcance | **pasa**: 0, 0 y 0. `deploy/flux/base/security/` y `policy/` sin cambios en el diff. |

## Hallazgos

### F-01 · ROJO · CA-3 / `deploy/flux/base/warm.yaml` (Service `warm-app`) · Causa raíz del cuelgue de `POST /resets`
El Service `warm-app` solo expone el puerto 80. El diff añade `containerPort: 9090` al pod, pero no añade el puerto 9090 al Service. `baseline.sh` llama a `http://warm-app.aqs-test.svc:9090`, un puerto que el Service no define, así que la conexión no recibe ni respuesta ni rechazo. `wget` de busybox no tiene timeout por defecto, de modo que `clean` se queda colgado hasta el `RESET_SCRIPT_TIMEOUT` (60 s). Dos intentos suman unos 120 s, que es el HTTP 000 del codificador. El warm queda en cuarentena o dirty y `ensure` y `deploys` devuelven 409.

Evidencia, ejecutada dentro del pod go-reset de mi clúster:
```
service/warm-app ports: [{"name":"http","port":80,...}]          (solo 80)
wget -T 8 http://warm-app.aqs-test.svc:9090/__aqs/baseline/version  -> "download timed out"
wget -T 8 http://<podIP>:9090/__aqs/baseline/version               -> ref-v1  (rc=0)
wget -T 8 http://warm-app.aqs-test.svc:80/health                   -> {"status":"ok"}
```
- **NetworkPolicy descartada.** El ingreso de aqs-system a aqs-test por la IP del pod en :9090 funciona. `allow-from-control-plane` permite aqs-system a aqs-test en cualquier puerto. aqs-system no tiene egress restringido: `networkpolicies-system.yaml` es solo de ingreso. Con una NetworkPolicy no se arregla.
- **Experimento sobre el clúster vivo, no en el código.** Añadí con `kubectl patch` el puerto `baseline` 9090 (`targetPort: baseline`) al Service. Resultado: `OK reset-previo-warm-ready`, `OK deploy-target-app-ensure`, `OK deploy-target-app-aceptado`, `OK deploy-target-app` y `OK reset-verificado`. Todo el script terminó en 31 s. Repetí lo mismo en la segunda ejecución.

**Arreglo para la ronda 2:**
- Añadir en el Service `warm-app` de `warm.yaml` `{name: baseline, port: 9090, targetPort: baseline}`.
- Poner `-T <seg>` en los `wget` de `baseline.sh`, para que falle rápido en vez de colgarse.

### F-02 · ROJO · CA-3 (`deploy rc=0`) / `demo/target-app/store.go` `openStore` · La app sale a los 30 s si la base aún no responde
`openStore` reintenta el ping solo 30 s y luego la app hace `os.Exit(1)`. En un clúster nuevo `warm-db` tarda más. El contenedor `warm-app` reinicia (`RESTARTS 1`) y `deploy.sh` marca `FALLA aqs-test/deployment.apps/warm-app` por su regla de reinicios cero. Reproduje esto en las dos ejecuciones, así que `deploy rc=1`, no `0` como pide CA-3.

```
warm-app previous logs: {"level":"ERROR","msg":"no se pudo abrir la base","error":"context canceled"}
lastState: terminated exitCode 1, started 15:32:49, finished 15:33:19 (30 s exactos)
```
- **Afirmación de la bitácora desmentida.** Dice «`deploy.sh`: rc=0 en la mayoría de ejecuciones». En mis dos ejecuciones dio 0 de 2.
- **Arreglo:** la app no debe morir por una base lenta. Debe reintentar mientras viva el contexto y escuchar ya, con `/health` en 503 (`db_down`) hasta que haya base. También se puede pasar la conexión a perezosa. La sonda de readiness actual usa `/`.

### F-03 · ROJO (bloqueo de especificación) · CA-3 `OK superficie-con-orders` · `POST /surface` da 502 por un usuario MinIO sin aprovisionar
Con F-01 aplicado en caliente, `deploy-target-app` pasa y la app es alcanzable. Aun así `/surface` responde 502:
```
go-warm-manager: "guardar la superficie fallo ... The Access Key Id you provided does not exist in our records."
```
- **Causa.** La clave del Secret `aqs-evidence-s3` no existe como usuario en MinIO. El propio `kind-smoke.sh:270` lo declara: «usuario de aqs-evidence-s3 sin aprovisionar, C-107». El aprovisionamiento es el alcance de U8-T05, que depende de U8-T04 fusionada.
- **Efecto.** CA-3 de U8-T04 no puede pasar al 100 % con el alcance actual, y el codificador no puede arreglarlo sin salirse de alcance.
- **Decisión del orquestador:** mover `superficie-con-orders` a U8-T05 o a U8-T08, o ampliar el alcance de U8-T04 con el aprovisionamiento mínimo del usuario `aqs-evidence`. La superficie que el warm-manager lee del app sí funciona: desde el pod de go-warm-manager, `wget http://warm-app.aqs-test.svc/openapi.json` devuelve el OpenAPI de la app.
- **Para la ronda 2.** El codificador debe corregir F-01 y F-02 y dejar F-03 documentado como bloqueado. No debe inventar un workaround.

### F-04 · NARANJA · `scripts/test/u8-target-app-kind.sh` y bitácora · La prueba no distingue el baseline real del relleno
`reset-verificado` parte de un warm sin datos sucios y lee solo el campo `reset_verified` de la respuesta de `POST /resets`, que es lo que go-reset afirma de sí mismo. Con el relleno `kind-stub` también daría `true`.
- **Falta lo que pedía el plan de pruebas.** Pide pegar dos lecturas: `POST /resets` con el relleno da `true` sin verificar nada, y con el baseline real y datos sucios `verify` da >0. La bitácora admite que no capturó la primera. La segunda solo existe localmente (CA-1), no en el clúster.
- **Arreglo.** En el script de kind, ensuciar la base (`POST /orders` contra la app), correr el reset y leer de vuelta que `GET /orders` queda vacío y `verify` da 0. Pegar el rojo con el relleno o con `clean` desactivado.

### F-05 · NARANJA · `deploy/flux/base/control-plane.yaml` (go-reset) y `warm.yaml` · Modo 0444 en el token que autoriza `clean` (alerta 4f)
- **go-reset.** El pod ya tiene `fsGroup: 65532` y el proceso corre como uid 65532, así que `defaultMode: 0440` bastaba para go-reset. Ensanchar a 0444 ahí es innecesario.
- **warm-app.** El `CrashLoop` con 0440 pasa porque el pod no tiene `fsGroup`. La solución correcta es poner `securityContext.fsGroup: 65532` en el pod `warm-app` y dejar 0440 en ambos Secrets, no abrir a "otros".
- **Riesgo.** Es un token que permite truncar los datos de la app. El riesgo real es bajo: tmpfs privado del pod, sin otro contenedor ni acceso en el nodo.
- **Referencia.** Hay precedentes de 0444 en `control-plane.yaml` (líneas 249, 389, 529), pero ninguno es un token con ese poder.

### F-06 · AMARILLO · agrupado (barrido de clase)
- **Documentación y mensajes obsoletos.** `docs/operaciones/kind-local.md:58` dice «Pendiente … relleno kind-stub» y `kind-local.md:11` dice «9 imágenes». `scripts/kind/kind-smoke.sh:269` imprime `PENDIENTE reset verificado (… kind-stub …)`, que ya es falso. Ambos archivos están dentro de la cerca de CA-4.
- **Semilla duplicada.** Los valores `cafe/10` y `te/5` están escritos dos veces, en `seedProducts` y en el `VALUES` literal de `Diff` en `store.go`. Hay riesgo de deriva entre ambos.
- **Token en `secrets.sh`.** `copy_target_app_token` no compara los valores si el Secret ya existe en aqs-system. Si se regenera el de aqs-test, quedan tokens distintos, 401 y un cuelgue silencioso.
- **Comparación del token.** `constantTimeEq` filtra la longitud y reimplementa lo que ofrece `crypto/subtle`.
- **Validación de `AQS_IMAGES`.** En `all_images`, el `case` usa subcadena (`*"$n "*`), así que `AQS_IMAGES=app` pasa la validación sin seleccionar nada.
- **Toolchain en las pruebas.** `GOTOOLCHAIN=auto` descarga un toolchain de Go desde la red al probar.
- **Superficie de `:9090`.** `allow-same-namespace` de aqs-test permite que cualquier pod del namespace (el runner de caja negra) alcance `:9090`, protegido solo por el token. Opcional: una NetworkPolicy acotada al par go-reset → warm-app:9090.
- **dev/prod.** `base` ahora exige el Secret `target-app-baseline-token` en aqs-test y aqs-system, pero `scripts/secrets/generate.sh` y `docs/operaciones/secrets.md` aún no lo generan. En dev/prod go-reset y warm-app no arrancarían (`ContainerCreating`) hasta U8-T07, que ya lo cubre. No debe aplicarse a un clúster Flux real antes de U8-T07.
- **`images:` de kind.** `warm-app` ya arranca con target-app, así que el «deploy de target-app» sobre el warm es un cambio de imagen a la misma imagen.

## Alerta «Security Weaken»: punto por punto
| Punto | ¿Debilita? | Severidad |
|---|---|---|
| (a) `sslmode=disable` y el password interpolado en `DATABASE_URL` (heredado por dev y prod) | **No en efecto.** `warm-db` es `postgres:16.4` sin TLS: no se baja una protección que no existe. El tráfico queda dentro de aqs-test, con default-deny. El password viene de `secretKeyRef` y se compone con `$(PGPASSWORD)`, no es literal en el manifiesto; `secretrefs.rego` pasa. Es un literal heredado en `base` que habrá que cambiar si se activa TLS. Confirmar que el generador de dev/prod produce contraseñas aptas para URL (el de kind usa hex). | AMARILLO |
| (b) Sustitución global de `nginx` con `images:` en el overlay kind | **No.** Solo existe un contenedor `nginx` (`warm-app`), el overlay es solo kind y el tag está fijado, no es `latest`. Es una cuestión de validez de la prueba, no de seguridad. | AMARILLO |
| (c) `cap_net_bind_service` en el binario | **No.** Es una capability de archivo, solo `net_bind_service`, con usuario 65532. Se acopla a que no se active `allowPrivilegeEscalation:false` más adelante (`no_new_privs` anula las file caps). Alternativa: escuchar en 8080. | AMARILLO |
| (d) `--security-opt label=disable` (SELinux) en los comandos de prueba | **No.** Es solo para un contenedor local de prueba, con montajes `:ro` y un token aleatorio. Tiene el mismo precedente que `scripts/ci/policies.sh:24`. `:z` o `:Z` reetiquetaría archivos del repositorio, que es peor. Aceptable. | Sin hallazgo |
| (e) Imagen de postgres re-etiquetada desde ECR | **No hay cambio en el diff**: el re-etiquetado solo está en la bitácora. Yo hice pull de `docker.io/library/postgres:16.4` sin problema. El tag está fijado pero no por digest. | AMARILLO |
| (f) Cambios de permisos de Secrets | Sí, ensancha sin necesidad en go-reset. Ver F-05. | NARANJA |
| (g) `policy/*.rego` y NetworkPolicy relajadas | **No.** Ni `policy/` ni `deploy/flux/base/security/` aparecen en el diff, y `policies.sh` da 0 FALLA. Sí queda la exposición de `:9090` descrita en F-06. | Sin hallazgo |

## Desviaciones de CA-2
- `--entrypoint sh` es necesaria y aceptable. El `ENTRYPOINT` de la imagen es `go-reset`, así que `sh /baseline.sh clean` literal ejecutaría `go-reset sh …`. El comando literal del criterio no puede funcionar.
- `label=disable` es aceptable por lo dicho en el punto (d).

## Tareas candidatas (fuera de alcance)
- **go-reset.** Ejecutar el reset con un contexto desligado de la petición HTTP. Cuando el cliente corta a los 120 s, go-reset registra `no se pudo escribir la cuarentena: context canceled` y el estado se queda dirty o sin cuarentena. Hoy el `curl` con `--max-time` del script de kind lo desencadena.
- **go-reset.** No hay espera ni reintento entre `restart` y `clean`. En el único caso que pude ver tras F-01 funcionó, pero queda un posible carrera.
- **U8-T05, orden.** El aprovisionamiento del usuario MinIO `aqs-evidence` (F-03) es el bloqueo real de la superficie en kind.
- **Fusión, conflictos.** `origin/main` avanzó (U8-T06 fusionada y toca `control-plane.yaml`). Cuando U8-T04 vuelva, probablemente haya conflictos en ese archivo.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/u8t04-rev.out` (ejecución completa: CA-3 literal, diagnóstico y experimento)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/u8t04-rev2.out` (reinicio de warm-app y logs de go-warm-manager)
- `/tmp/claude-1000/pol.out` (`policies.sh`)

VEREDICTO: NO-VERDE
ROJO|deploy/flux/base/warm.yaml (Service warm-app)|F-01 El Service no expone :9090: baseline.sh se cuelga y go-reset deja el warm en cuarentena (causa raíz de CA-3)
ROJO|demo/target-app/store.go openStore, CA-3 deploy rc=0|F-02 La app sale a los 30 s si warm-db aún no responde: warm-app reinicia y deploy.sh da FALLA
ROJO|CA-3 superficie-con-orders|F-03 POST /surface da 502: usuario MinIO aqs-evidence sin aprovisionar (depende de U8-T05; decisión del orquestador)
NARANJA|scripts/test/u8-target-app-kind.sh, bitacoras/U8-T04.md|F-04 La prueba de reset no ensucia la base ni lee de vuelta; no distingue el baseline real del relleno; falta el rojo del plan
NARANJA|deploy/flux/base/control-plane.yaml y warm.yaml defaultMode 0444|F-05 El token que autoriza clean se abre innecesariamente a 0444; usar fsGroup 65532 y 0440
AMARILLO|varios|F-06 Documentación obsoleta, semilla duplicada, token sin sincronizar, dev/prod sin el Secret hasta U8-T07, exposición de :9090 en el namespace
INFORME: revisiones/U8-T04/ronda-1.md
