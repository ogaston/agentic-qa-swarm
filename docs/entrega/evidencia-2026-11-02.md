# Evidencia capturada — 2026-11-02 (clúster kind `kind-aqs`, Kubernetes v1.37.0)

Salidas reales, sin editar salvo recortes marcados. Regenerables con los comandos de cada bloque.

## 1. RBAC: `kubectl auth can-i --list` de los ServiceAccounts de los agentes LLM

```console
$ kubectl auth can-i --list -n aqs-test --as=system:serviceaccount:aqs-system:agent-planner
Resources                                       Non-Resource URLs                      Resource Names   Verbs
selfsubjectreviews.authentication.k8s.io        []                                     []               [create]
selfsubjectaccessreviews.authorization.k8s.io   []                                     []               [create]
selfsubjectrulesreviews.authorization.k8s.io    []                                     []               [create]
clustertrustbundles.certificates.k8s.io         []                                     []               [get list watch]
                                                [/.well-known/openid-configuration/]   []               [get]
                                                [/.well-known/openid-configuration]    []               [get]
                                                [/api/*]                               []               [get]
                                                [/api]                                 []               [get]
                                                [/apis/*]                              []               [get]
                                                [/apis]                                []               [get]
                                                [/healthz]                             []               [get]
                                                [/healthz]                             []               [get]
                                                [/livez]                               []               [get]
                                                [/livez]                               []               [get]
                                                [/openapi/*]                           []               [get]
                                                [/openapi]                             []               [get]
                                                [/openid/v1/jwks/]                     []               [get]
                                                [/openid/v1/jwks]                      []               [get]
                                                [/readyz]                              []               [get]
                                                [/readyz]                              []               [get]
                                                [/version/]                            []               [get]
                                                [/version/]                            []               [get]
                                                [/version]                             []               [get]
                                                [/version]                             []               [get]
```

Solo los permisos de descubrimiento que Kubernetes concede a todo usuario autenticado (`system:basic-user`, `system:discovery`). Ningún recurso. `agent-reporter` da la misma salida.

## 2. RBAC: el ServiceAccount que sí toca Kubernetes (`go-run-controller`, Go determinista, sin LLM)

```console
$ kubectl auth can-i --list -n aqs-test --as=system:serviceaccount:aqs-system:go-run-controller | head -12
Resources                                       Non-Resource URLs                      Resource Names   Verbs
selfsubjectreviews.authentication.k8s.io        []                                     []               [create]
selfsubjectaccessreviews.authorization.k8s.io   []                                     []               [create]
selfsubjectrulesreviews.authorization.k8s.io    []                                     []               [create]
configmaps                                      []                                     []               [get list watch create update patch delete]
services                                        []                                     []               [get list watch create update patch delete]
deployments.apps                                []                                     []               [get list watch create update patch delete]
statefulsets.apps                               []                                     []               [get list watch create update patch delete]
jobs.batch                                      []                                     []               [get list watch create update patch delete]
pods/log                                        []                                     []               [get list watch delete]
pods                                            []                                     []               [get list watch delete]
clustertrustbundles.certificates.k8s.io         []                                     []               [get list watch]
```

## 3. Preguntas puntuales

```console
$ kubectl auth can-i create pods -n aqs-test --as=system:serviceaccount:aqs-system:agent-planner
no
$ kubectl auth can-i get secrets -n aqs-system --as=system:serviceaccount:aqs-system:agent-reporter
no
$ kubectl auth can-i patch kustomizations.kustomize.toolkit.fluxcd.io -n flux-system --as=system:serviceaccount:aqs-system:agent-planner
no
$ kubectl auth can-i create jobs -n default --as=system:serviceaccount:aqs-system:go-run-controller
no
$ kubectl auth can-i create jobs -n aqs-test --as=system:serviceaccount:aqs-system:go-run-controller
yes
$ kubectl auth can-i create pods/exec -n aqs-test --as=system:serviceaccount:aqs-system:go-reset
no
$ kubectl auth can-i get pods -n aqs-test --as=system:serviceaccount:aqs-test:aqs-runner
no
```

## 4. Pod Security Admission

```console
$ kubectl get ns aqs-system aqs-test --show-labels   # (recortado a las etiquetas pod-security)
aqs-system  baseline (enforce) / restricted (warn)
aqs-test  baseline (enforce) / restricted (warn)
$ kubectl run psa-probe -n aqs-test --image=busybox:1.37.0 --dry-run=server --overrides=<privileged:true>
Error from server (Forbidden): pods "psa-probe" is forbidden: violates PodSecurity "baseline:latest": privileged (container "p" must not set securityContext.privileged=true)
```

## 5. Bucle GitOps: un cambio en Git que se reconcilia en el clúster

Flux v2.4.0 instalado con `AQS_FLUX_BRANCH=entrega/modulo8 bash scripts/kind/flux-up.sh`:

```console
OK rama entrega/modulo8 en GitHub: 3b22147e3ec659b983a3094d1c2d47dd02b554a6
OK secrets
OK controladores de Flux (v2.4.0)
OK sync aplicado (rama entrega/modulo8)
OK aqs-kind Ready en entrega/modulo8@sha1:3b22147e3ec659b983a3094d1c2d47dd02b554a6
```

Cambio: commit `4d93c97` (etiquetas Pod Security en `deploy/flux/base/namespaces.yaml`), `git push` a las
13:25:06 (UTC-5). Sin `kubectl apply`. Antes del push los namespaces no tenían etiquetas `pod-security.*`.

```console
$ kubectl get ns aqs-system aqs-test --show-labels        # 13:26:10, 64 s después del push
aqs-system  ...,pod-security.kubernetes.io/audit=restricted,pod-security.kubernetes.io/enforce=baseline,pod-security.kubernetes.io/warn=restricted
aqs-test    ...,pod-security.kubernetes.io/audit=restricted,pod-security.kubernetes.io/enforce=baseline,pod-security.kubernetes.io/warn=restricted

$ bash scripts/kind/flux-status.sh
rama:            entrega/modulo8
GitHub:          4d93c97df9ed1621c7e2956b541662b54cd955ff
fuente (Flux):   entrega/modulo8@sha1:4d93c97df9ed1621c7e2956b541662b54cd955ff
aplicado (Flux): entrega/modulo8@sha1:4d93c97df9ed1621c7e2956b541662b54cd955ff  Ready=True
mensaje:         Applied revision: entrega/modulo8@sha1:4d93c97df9ed1621c7e2956b541662b54cd955ff
OK clúster en la revisión de Git
```

Efecto secundario observado al instalar Flux: en la primera reconciliación los 9 Deployments del control plane se
reiniciaron una vez (Flux toma la propiedad de los campos que había aplicado `kubectl`). Después, 0 reinicios en
más de 5 reconciliaciones. `kind-smoke.sh --preflight` tras el cambio: todo OK salvo `warm-estado` (el warm ya
estaba en cuarentena antes de instalar Flux; resuelto en la sección 7).

## 6. Evaluación por modelo (`agents/eval`)

```console
$ cd agents/eval && .venv/bin/python -m pytest -q
111 passed
$ .venv/bin/python -m pytest -q tests/test_live_model.py
9 passed      # gateway OpenAI en loopback; «modelo-bueno» 12/12 casos, «modelo-roto» 2/12 y rc=1
```

Pendiente antes de presentar: `agents/eval/run-models.sh <modelo-A> <modelo-B>` contra el gateway real y pegar
aquí `out/comparacion.md`.

## 7. Cuarentena real y recuperación por GitOps

El reset se negó a declarar listo un warm roto: el Deployment `aqs-test/warm-app` apuntaba a la imagen
`demo-target-app-qa-swarm:81b5ea9…`, que no existe en GHCR (404) ni como commit del repo objetivo.

```console
$ bash scripts/demo/up.sh --reset
reset: not verified (HTTP 409)
{"attempts":2,"checks":null,"reset_verified":false,"state":"cuarentena"}
# go-reset: "alert":"warm.quarantined","reason":"verificación fallida: [app-ready: pod no Ready]"
```

Recuperación sin `kubectl apply`: se borró el Deployment y Flux lo recreó desde Git (`ssa: IfNotPresent` crea lo
que falta) con la imagen de referencia del overlay.

```console
$ kubectl -n aqs-test delete deployment warm-app          # 15:30:45
$ kubectl -n aqs-test get deploy warm-app -o jsonpath=...  # 15:30:48
ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d
$ bash scripts/demo/up.sh --reset
reset: OK (state=ready, reset_verified=true)
$ bash scripts/kind/kind-smoke.sh --preflight
smoke: OK (sin FALLA; pendientes declarados arriba)          # 24 OK, 0 FALLA
```

## 8. Primera corrida contra modelos reales (inválida por configuración)

`agents/eval/run-models.sh Deepseek-V4-Flash Qwen-3.8-Max-Quantized`, 2026-11-02 ~15:45, con los topes por
defecto de las bibliotecas (planner 25 s / 4000 tokens, reporter 30 s / 1024 tokens):

| llm | factualidad | precision | ruido | adherencia | casos ok | umbrales |
|---|---|---|---|---|---|---|
| http:Deepseek-V4-Flash | 1.00 | 0.80 | 0.00 | 0.68 | 2/12 | FALLA |
| http:Qwen-3.8-Max-Quantized | 1.00 | 0.80 | 0.00 | 0.68 | 2/12 | FALLA |

Diagnóstico por caso (`report.json`): el planner falló en los 10 casos con superficie, con `PlanRejected` en
Deepseek y `BudgetExceeded` en Qwen. Los dos `no-arranca` fallaron cerrado, como se esperaba. Que dos modelos
distintos den números idénticos indica un fallo del sistema, no del modelo. El clúster corre los agentes con
120 s y 8000 tokens de salida, porque un modelo de razonamiento con `max_tokens` bajo gasta la salida pensando y
devuelve `content` vacío (comentario en `deploy/flux/kind/kustomization.yaml`). El eval usaba los topes del LLM
falso.

Corrección: `run-models.sh` usa por defecto los topes del clúster, y el informe guarda el código fijo del error
(`PlanRejected:json_invalido`, `BudgetExceeded:time`…) para que un fallo sistemático se diagnostique sin adivinar.
Hay que repetir la corrida.

Lectura para la sustentación: la prueba de cambio de modelo cumplió su función. Detectó que la configuración del
arnés no era la del despliegue antes de que llegara a una demo.
