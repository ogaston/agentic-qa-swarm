# Ronda 1 — U5-T10

VEREDICTO: VERDE

No encontré ninguna evasión real de la regla, y los seis criterios pasan con mi propia ejecución. Hay tres hallazgos AMARILLO que no bloquean.

## Criterios de aceptación, verificados por mí
Todo corrió en `/home/omarjayg/Javeriana/topicos-especiales/wt-U5-T10`, con los alias `K` y `C` literales. El hash de la tarea coincide: f2e0bce5.

| # | Criterio | Resultado |
|---|---|---|
| 1 | CA-1: `egress: [{}]` concatenado al build de prod | `rc=1`, pasa. El rojo inicial (`rc=0` sobre la base) lo cita la bitácora y no lo reproduje porque el árbol ya tiene la regla. |
| 2 | CA-2: `conftest verify` | `rc=0`, `36 tests, 36 passed, 0 skipped`. `egress_test.rego` aporta 16 pruebas (mínimo 9), con permitidas (a) y (b) y una denegada por cada caso del alcance. |
| 3 | CA-3: build de dev y de prod | `rc=0` dos veces, `328 tests, 328 passed, 0 failures`. |
| 4 | CA-4: matriz completa | `rc=1` en los 8 denegados (abierta, solo-puertos, todos-ns, otro-ns, expr, ipblock, dns-443, dns-sin-puertos). `rc=0` en `ok-intra` y `ok-dns`. |
| 5 | CA-5: archivos cambiados | Salida exacta: `bitacoras/U5-T10.md`, `policy/egress.rego`, `policy/egress_test.rego`. |
| 6 | CA-6: árbol limpio | `git status --short | wc -l` da `0`. Tras mi revisión el worktree sigue limpio. |

La suite no es una señal de saltos: `0 skipped` y las 36 pruebas corrieron. Mis casos adicionales usaron el build real de prod más una política añadida, sin `--combine`.

## Evasiones intentadas
Todas son NetworkPolicy en `aqs-test` concatenadas al build de prod. «Esperado» es mi expectativa antes de correr. Para el caso «sin namespace», `rc=0` es lo que la regla produce, no lo que yo esperaba.

| Intento | Esperado | Devolvió |
|---|---|---|
| kube-system más otra etiqueta en `matchLabels` | deny | deny |
| kube-system en `matchLabels` más `matchExpressions` | deny | deny |
| `namespaceSelector` kube-system más `ipBlock` | deny | deny |
| `ipBlock: {}` y `ipBlock: null` junto a kube-system | deny | deny |
| `endPort: 65535` y `endPort: 0` | deny | deny |
| puerto nombrado `dns` y puerto `"53"` como cadena | deny | deny |
| protocolo `SCTP` | deny | deny |
| `protocol: null` | deny | deny |
| `port` ausente (solo protocolo) | deny | deny |
| `ports: []` | deny | deny |
| `ports` con 53 y además 443 | deny | deny |
| `podSelector` más `namespaceSelector: {}` en el mismo peer | deny | deny |
| `podSelector` más kube-system, sin puertos 53 | deny | deny |
| peers mezclados (intra más kube-system) | deny | deny |
| intra más un segundo peer `ipBlock` | deny | deny |
| `to: null` y `to: []` | deny | deny |
| `podSelector: null` | deny | deny |
| clave extra en `namespaceSelector` | deny | deny |
| `Kube-System` con otra capitalización | deny | deny |
| egress sin `policyTypes` | deny | deny, y es correcto porque Kubernetes lo aplica |
| `policyTypes: [Ingress]` con bloque `egress` | deny | deny. Kubernetes lo ignoraría, pero denegar es lo conservador. |
| `egress: null` | sin deny | sin deny. Para Kubernetes equivale a que no haya egress, así que es correcto. |
| `ipBlock: false` junto a kube-system y 53 | deny | **`rc=0`**. Ver F-01. |
| sin `metadata.namespace` | — | `rc=0`. Ver F-02. |
| kube-system con `podSelector` (kube-dns) y 53 UDP | permitido | permitido |
| intra con puertos 443 | permitido | permitido |
| protocolo ausente con puerto 53 | permitido (TCP) | permitido |

## Decisiones no fijadas
- **Peers mezclados denegados:** cada regla es entera (a) o (b). Es lo más estricto y no rompe nada legítimo. Las políticas reales se escriben con una regla por tipo.
- **Protocolo ausente como TCP:** coincide con el valor por defecto de Kubernetes. No es permisivo.
- **`endPort` denegado:** estricto, y no rompe ningún caso legítimo de DNS.
- **`to: []` como abierto:** denegado. Para Kubernetes `to: []` equivale a `to` ausente, o sea abierto, así que es correcto.
- **`ports` sin restringir en (a):** es la única decisión algo permisiva. Está dentro de la letra de la tarea, porque (a) solo habla de peers, y el destino sigue limitado al propio namespace. No la objeto.
- **DNS sin `podSelector`:** el DNS abre el puerto 53 a cualquier pod de kube-system. Es lo que pide la tarea.

## Hallazgos
### F-01 · AMARILLO · `policy/egress.rego`, `peer_dns` · `not peer.ipBlock` deja pasar `ipBlock: false`
Con `ipBlock: false` junto a kube-system y 53 el resultado es `rc=0`. Un manifiesto así no es válido en Kubernetes: el esquema exige un objeto, y `null`, `{}` y los objetos reales sí se deniegan. No hay evasión práctica. Se arreglaría con `not "ipBlock" in object.keys(peer)`.

### F-02 · AMARILLO · `policy/egress.rego:8` · NetworkPolicy sin `metadata.namespace` no se evalúa
Una NetworkPolicy sin namespace con `egress: [{}]` da `rc=0`. En este repo no es un hueco real:
- ninguna `kustomization.yaml` bajo `deploy/` define `namespace:` ni `targetNamespace`;
- los manifiestos de `aqs-test` declaran su namespace explícito;
- `security.rego` usa la misma convención.

Si algún día se añade un transformador de namespace, el hueco se abre en las dos políticas a la vez. Lo anoto como tarea candidata.

### F-03 · AMARILLO · `policy/egress_test.rego` · decisiones sin prueba unitaria
Estas decisiones solo están cubiertas por mi matriz y por la bitácora:
- `endPort`;
- `SCTP`;
- puerto nombrado;
- peers mezclados;
- kube-system más etiqueta extra o `matchExpressions`;
- protocolo ausente.

Si alguien relajara `dns_port` o `peer_dns`, `conftest verify` seguiría en verde. No rompe ningún criterio.

## Tareas candidatas
- Endurecer las políticas de aqs-test para que exijan `metadata.namespace` en toda NetworkPolicy, o denegar las que no lo declaren, por si se introduce un transformador de namespace de kustomize (F-02).
- Con `--combine`, `egress.rego` no dispara porque `input` es un arreglo. Pasa lo mismo con `security.rego`, y `default_deny.rego` es el único que lo documenta. Tiene que verlo quien integre conftest en CI (C-20).

## Alcance
Solo cambiaron los tres archivos del alcance. `git diff 0a15da4..HEAD` sobre `deploy/`, `security*.rego` y `default_deny.rego` está vacío. No se tocó ninguna política existente ni se ejecutó nada contra un clúster. Las bitácoras no incluyen CA-5 ni CA-6, como prevé la tarea. Los resultados de ambos van arriba.

VEREDICTO: VERDE
AMARILLO|policy/egress.rego peer_dns|`ipBlock: false` pasa la regla (manifiesto inválido en Kubernetes)
AMARILLO|policy/egress.rego:8|NetworkPolicy sin namespace no se evalúa (sin hueco real hoy)
AMARILLO|policy/egress_test.rego|Decisiones sin prueba unitaria (endPort, SCTP, puerto nombrado, peers mezclados)
INFORME: revisiones/U5-T10/ronda-1.md
