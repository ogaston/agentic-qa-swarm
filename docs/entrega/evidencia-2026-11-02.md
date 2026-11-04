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

La comparación contra el gateway real está medida en las secciones 10 y 11 (Deepseek-V4-Flash 10/12, Qwen-3.8-Flash 6/12).

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

## 9. `deepseek-flash` con los topes del clúster: hallazgo de rutas con plantilla

Gateway LiteLLM del clúster (`LITELLM_UPSTREAM_URL` de `litellm.env`, clave `deepseek.key`), topes de
`deploy/flux/kind` (120 s, 16000/8000 tokens). Duración: 5 min 10 s. El gateway solo sirve `deepseek-flash` y
`deepseek-v4-pro`; los nombres `Qwen-3.8-*` responden `400 The supported API model names are deepseek-flash,
deepseek-v4-pro`.

```console
$ LLM_BASE_URL=<gateway> LLM_API_KEY_FILE=~/.local/state/aqs-kind/deepseek.key ./run-models.sh deepseek-flash
topes: planner 120s/8000 tok, reporter 120s/8000 tok
OK factualidad 1.0000 (umbral > 0.8)
FALLA precision 0.8000 (umbral > 0.8)
OK ruido 0.0000 (umbral < 0.2)
FALLA adherencia 0.6774 (umbral == 1)
| http:deepseek-flash | 1.00 | 0.80 | 0.00 | 0.68 | 2/12 | FALLA |
```

Por caso: el planner da `PlanRejected:endpoint_no_observado:<flujo>/0` en los 10 casos con superficie. El reporter
da el veredicto esperado en los 7 golden y bug-sembrado. En trampa-fintech-01 da `inconcluso`, como se esperaba.
En trampa-ecom-01 y trampa-logis-01 el validador da `ReportRejected:sin_hallazgos_con_flujo_no_pasado`, que es
fallo cerrado. Los dos `no-arranca` dan `NoSurface`/`NoEvidence`, como se esperaba.

Confirmación con una llamada aislada del planner sobre `golden-ecom-01` (superficie: `POST /orders`,
`GET /orders/{id}`, `POST /orders/{id}/cancel`):

```text
create-order-happy          [('POST', '/orders')]
get-order-happy             [('GET', '/orders/1')]
cancel-order-happy          [('POST', '/orders/1/cancel')]
get-missing-order-probe     [('GET', '/orders/999999999')]
cancel-missing-order-probe  [('POST', '/orders/999999999/cancel')]
```

El modelo concreta los parámetros de ruta, y `validate.py` compara `(method, path)` contra la plantilla literal.
Es un defecto del contrato del planner, no del modelo: un plan razonable se rechaza entero. Corrección propuesta
como tarea: aceptar instancias de una plantilla observada (cada segmento `{x}` casa con un segmento no vacío, sin
`/`, `..` ni host) y conservar la plantilla en el `FlowPlan` para A1 y para el runner.

## 10. Deepseek-V4-Flash contra Qwen-3.8-Flash en el gateway LiteLLM del humano

Gateway `http://chalas-lab:4000` (`100.84.211.20`, Tailscale; excepción http acotada a esa IP), clave
`litellm.key`, topes del clúster. `run-models.sh Deepseek-V4-Flash Qwen-3.8-Flash`, 5 min 4 s:

| llm | factualidad | precision | ruido | adherencia | casos ok | umbrales |
|---|---|---|---|---|---|---|
| http:Deepseek-V4-Flash | 1.00 | 0.80 | 0.00 | 0.68 | 2/12 | FALLA |
| http:Qwen-3.8-Flash | 0.00 | 0.00 | 0.00 | 0.63 | 2/12 | FALLA |

- **Deepseek-V4-Flash** repite el patrón de §9: `PlanRejected:endpoint_no_observado` en los 10 casos con superficie
  (rutas con plantilla concretadas) y el reporter acierta en los 7 golden y bug-sembrado.
- **Qwen-3.8-Flash** da `LLMUnavailable` en todas las llamadas. Prueba directa al gateway:
  `Together_aiException … "This model only supports streaming. Set \"stream\": true." (code: streaming_required)`.
  Nuestro adaptador (`agents/*/llm_http.py`) solo hace llamadas sin streaming. Es una incompatibilidad del
  adaptador, no una medida del modelo. `Qwen3.7-Max` da el mismo error; `Qwen-3.8-Max-Quantized` responde sin
  streaming (HTTP 200).
- Los dos `no-arranca` fallan cerrado en ambos modelos, sin llamar al LLM.

Lectura: los 12 casos detectaron dos incompatibilidades antes de llegar a producción. Sin ellos, cambiar a
`Qwen-3.8-Flash` en el Secret `llm-upstream` habría dejado toda corrida en `llm_unavailable`, y los planes con
rutas `{id}` se habrían rechazado con cualquier modelo que concrete parámetros.

## 11. Deepseek-V4-Flash contra Qwen-3.8-Flash, con streaming y rutas con plantilla

Mismo gateway y topes que en §10, después del commit `df58b7d` (`LLM_STREAM` opcional en los dos agentes;
`match_endpoint` acepta `/orders/1` como instancia de `/orders/{id}`). Comando:
`LLM_STREAM=1 ./run-models.sh Deepseek-V4-Flash Qwen-3.8-Flash`, 23 min 14 s, rc 1.

| llm | factualidad | precision | ruido | adherencia | casos ok | umbrales |
|---|---|---|---|---|---|---|
| http:Deepseek-V4-Flash | 1.00 | 0.90 | 0.00 | 0.97 | 10/12 | FALLA |
| http:Qwen-3.8-Flash | 1.00 | 0.70 | 0.00 | 0.84 | 6/12 | FALLA |

Casos que fallan (`out/<modelo>/report.json`, campos `planner.error` y `reporter.error`):

| Caso | Deepseek-V4-Flash | Qwen-3.8-Flash |
|---|---|---|
| golden-fintech-01 | ok | planner `BudgetExceeded:time` |
| bug-logis-01 | ok | planner `BudgetExceeded:time` |
| bug-fintech-02 | planner `PlanRejected:json_invalido` | planner `BudgetExceeded:time` |
| trampa-ecom-01 | ok | planner `BudgetExceeded:time`; reporter `ReportRejected:sin_hallazgos_con_flujo_no_pasado` |
| trampa-fintech-01 | ok | planner `BudgetExceeded:time`; reporter `ReportRejected:sin_hallazgos_con_flujo_no_pasado` |
| trampa-logis-01 | reporter `ReportRejected:sin_hallazgos_con_flujo_no_pasado` | reporter `ReportRejected:sin_hallazgos_con_flujo_no_pasado` |

En los demás casos (golden, bug-sembrado y los dos `no-arranca`) el plan es válido y el veredicto es el esperado.

**Por qué Qwen tarda.** Una llamada aislada con el prompt real del planner de `bug-fintech-01` (streaming, 8000
tokens de salida):

| Petición | Tiempo total | reasoning_tokens | completion_tokens |
|---|---|---|---|
| Por defecto | 150,9 s | 6665 | 7161 |
| `chat_template_kwargs: {"enable_thinking": false}` | 8,8 s | 0 | 545 |

`reasoning_effort: "low"` no está disponible: LiteLLM responde HTTP 400
`together_ai does not support parameters: ['reasoning_effort']`. Con el tope de 120 s del clúster, el planner
corta la llamada y falla cerrado (`BudgetExceeded:time`). Es el límite actuando, no un plan aceptado tarde.

Lectura: con el arnés arreglado, la diferencia entre modelos es real y medible. Con esta configuración,
Deepseek-V4-Flash es el modelo del clúster. Qwen-3.8-Flash necesitaría el razonamiento apagado o un tope mayor, y
los dos fallan las trampas del esquema en el reporter, que el validador detiene.
