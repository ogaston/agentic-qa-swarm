# Guion de la grabación de respaldo (demo de 3 minutos)

La guía de sustentación pide una demo en vivo de 3 minutos **y una grabación de respaldo**, que incluya un caso en
que el agente falla o lo frenan sus límites. Este es el procedimiento exacto. Todo se corre desde la raíz del
repositorio, con el contexto de kubectl `kind-aqs`.

## Antes de grabar (unos 20 minutos, sin cámara)

Cómo encender, apagar y reparar cada pieza del entorno (dashboard, túnel, webhook, warm):
[docs/operaciones/demo-subir-bajar.md](../operaciones/demo-subir-bajar.md). `bash scripts/demo/status.sh` dice en
segundos si todo está listo y, si no, qué comando lo arregla.

1. **Actualiza `main` y deja Flux en `main`.** Así la grabación muestra `rama: main`. El `git pull` va
   **después** de fusionar el último PR y **antes** de `up.sh`, porque `up.sh` construye las imágenes desde tu
   copia local. Si `main` está atrasado, los pods corren código viejo aunque Flux muestre la revisión nueva:

   ```bash
   git switch main && git pull --ff-only
   git log --oneline -1 origin/main      # debe coincidir con tu HEAD
   bash scripts/kind/flux-up.sh          # debe terminar en «aqs-kind Ready en main@sha1:…»
   bash scripts/kind/flux-status.sh      # «rama: main» y «OK clúster en la revisión de Git»
   ```

2. **Levanta el dashboard, el túnel y el webhook de GitHub:**

   ```bash
   bash scripts/demo/up.sh
   ```

   Al final imprime la URL del dashboard y el archivo con las credenciales. `up.sh` reconstruye y vuelve a
   cargar las imágenes de la plataforma, y recrea sus pods (`scripts/kind/refresh-images.sh` los borra en vez
   de usar `rollout restart`, que Flux revierte). Si termina con
   `refresh-images: el pod … arrancó antes del reinicio`, repite `bash scripts/kind/refresh-images.sh` y luego `bash scripts/demo/reconnect.sh`.
   Después hay que resetear el warm:

   ```bash
   bash scripts/demo/up.sh --reset       # «reset: OK (state=ready, reset_verified=true)»
   bash scripts/kind/kind-smoke.sh --preflight   # «smoke: OK (sin FALLA…)»
   bash scripts/demo/status.sh                   # «status: OK (listo para grabar)»
   ```

   El warm se apaga solo tras 30 minutos sin corridas. Graba dentro de ese rato o repite el reset.

   Si el reset devuelve 409 con `cuarentena`, mira la imagen del warm:
   `kubectl -n aqs-test get pods -l app.kubernetes.io/name=warm-app`. Si está en `ImagePullBackOff`, borra el
   Deployment (`kubectl -n aqs-test delete deployment warm-app`); Flux lo recrea desde Git en segundos con la
   imagen de referencia. Repite el reset.

3. **Ensayo completo sin grabar.** El caso con el defecto de sobreventa todavía no se ha verificado de punta a
   punta. Hazlo una vez entero (pasos 4 a 6) antes de grabar. Si algo falla, prefiero saberlo hoy y no el 21.

4. **Empuja el commit con el defecto antes de grabar.** La CI de la app objetivo tarda varios minutos en publicar
   la imagen:

   ```bash
   bash scripts/demo/10-push-commit.sh \
     --repo-dir ~/Javeriana/topicos-especiales/demo-target-app-qa-swarm --bug oversell
   ```

   Espera a `OK imagen-publicada`. La notificación queda pendiente en el Inbox del dashboard.

5. Prepara la pantalla con dos ventanas: una terminal con letra grande y el navegador en el dashboard, con la
   sesión ya iniciada y en el Inbox.

## Grabación

Graba con el grabador de GNOME (`Ctrl+Shift+Alt+R` para empezar y para parar; queda en `~/Videos/Screencasts`) o
con OBS (1920×1080, MP4). Lo que dices va entre comillas.

| Tiempo | Pantalla | Qué hacer y decir |
|---|---|---|
| 0:00 | Terminal | `bash scripts/kind/flux-status.sh`. «El clúster está exactamente en el commit de `main`. Nada se aplicó a mano: lo reconcilia Flux.» |
| 0:20 | Terminal | `kubectl auth can-i create pods -n aqs-test --as=system:serviceaccount:aqs-system:agent-planner` responde `no`. «El agente que habla con el LLM no tiene ni un permiso en Kubernetes.» |
| 0:35 | Terminal | `kubectl auth can-i --list -n aqs-test --as=system:serviceaccount:aqs-system:agent-planner`. Señala que no hay recursos, solo descubrimiento. |
| 0:50 | Dashboard, Inbox | Abre la notificación del commit. «GitHub notificó; no corrió nada solo. Auto-notify sí, auto-run no.» Confirma. |
| 1:10 | Dashboard, corrida | Muestra las fases: warm listo, deploy, superficie inferida, ensayo. «Antes de la corrida completa, el plan se ensaya con un solo usuario.» |
| 1:30 | Dashboard | Revisa las familias de flujos y **aprueba la ejecución**. «Segunda compuerta humana.» |
| — | — | **Pausa la grabación** mientras corre (unos 3 a 4 minutos). Reanuda cuando la corrida llegue a `done`. |
| 2:00 | Dashboard, reporte | Post-mortem: el hallazgo de sobreventa (la invariante de stock), con la evidencia de los runners. «El LLM redactó esto con la evidencia, pero los runners que dispararon el tráfico no tienen acceso al modelo.» |
| 2:30 | Dashboard, Entorno | `reset_verified=true`. «El entorno quedó limpio y verificado. Si no, la siguiente corrida no arranca: cuarentena.» |
| 2:45 | Terminal | El límite: «Hoy la evaluación con `deepseek-flash` encontró que el planner rechaza planes con rutas `/orders/{id}` concretadas. Falla cerrado: prefiere no correr a correr algo que no observó.» (evidencia §9) |
| 3:00 | — | «El agente propone, un humano aprueba, GitOps reconcilia.» Para la grabación. |

## Después de grabar

1. Deja limpio el repo objetivo para la próxima demo:

   ```bash
   bash scripts/demo/10-push-commit.sh \
     --repo-dir ~/Javeriana/topicos-especiales/demo-target-app-qa-swarm --bug off
   ```

2. Sube el MP4 a Google Drive o a YouTube como **no listado**. No lo agregues al repositorio: pesa demasiado para Git.
3. Pon el enlace en el README §3.3, en lugar de «Pendiente antes de presentar: grabar el respaldo».
4. Vuelve a dejar el warm listo para el día de la presentación: `bash scripts/demo/up.sh --reset`.

## Si algo falla en vivo el 21

Pasa a la grabación sin intentar arreglarlo en el escenario: «Esto es lo que debía verse; aquí está grabado.»
Si el fallo es una cuarentena, también sirve: es el sistema negándose a correr sobre un entorno sucio.
