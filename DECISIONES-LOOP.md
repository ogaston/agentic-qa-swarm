# Decisiones del loop de agentes — Módulo 6

> Media página al final (guía §13). Cuatro puntos, honestos. Si el punto 3 queda vacío, el
> revisor es complaciente y eso es un problema, no una virtud.

## 1. Un empujón de vuelta que arbitré

Pendiente — cuando el codificador empuje de vuelta un hallazgo («no es un defecto» con comando, o
«fuera de alcance»), registrar aquí qué decidió el orquestador y por qué.

## 2. La tarea que más rondas costó, y qué estaba mal en su especificación

Pendiente — tarea, número de rondas, y qué criterio estaba mal escrito (el bucle largo casi nunca
es un codificador torpe).

## 3. Un hallazgo del revisor que yo no habría visto

Pendiente — un hallazgo concreto con su severidad. Si no hay ninguno, el revisor es complaciente.

## 4. Coste de la ola en paralelo frente a la serie

Pendiente — qué costó en tokens la ola en paralelo frente a haberla corrido en serie.

---

## Anexo — Decisiones de la ola 2 de U2 (2026-10-07)

Registro de lo decidido durante el loop de U2-T02, U2-T03 y U2-T06 (tres codificadores en paralelo, un revisor por ronda).

- **Tope duro de 3 rondas por tarea** (decisión del humano). Codificado en `.claude/agents/orquestador.md`. Las tres tareas lo agotaron sin VERDE.
- **Cierre «dividir»** (decisión del humano, a propuesta del orquestador): se fusionan U2-T02/T03/T06 tal cual (PR #40, #41, #42), con sus hallazgos abiertos en tareas de seguimiento U2-T02b, U2-T03b y U2-T06b (3 rondas propias cada una). No hubo VERDE; el cierre es por decisión del humano, como en U1-T05.
- **Patrón de los hallazgos:** en las tres tareas los defectos de cada ronda fueron de **una sola familia** (caminos optimistas o incompletos alrededor de un efecto externo o de una verificación: relanzamientos sin tope con el gate o el disco caídos, `fake`/semillas por defecto sin barrera, operaciones no atómicas, errores tragados, pruebas que no ejercen la invariante). Cada ronda destapaba una variante nueva de la misma familia; el barrido de clase obligatorio desde la ronda 2 la fue acotando pero no la agotó. Lección: una tarea que mezcla estado persistido, efectos externos y concurrencia necesita acotar el espacio de fallos **en su especificación** (tabla de fallos × estados) y no solo en los criterios.
- **Fallos de mi especificación que el loop expuso:** (1) el CA-6 de U2-T02 pasaba por variables faltantes y no discriminaba la valla (corregido en el archivo de la tarea); (2) la contradicción de los Jobs sin token de ServiceAccount (U2-T03/T06 vs `policy/isolation.rego`), que se resuelve en U2-T07 con decisión del humano; (3) U2-T06 pedía un Job `reset-{run}` «sin credenciales» que ejecutara pasos contra la API de Kubernetes: el codificador lo resolvió ejecutando el reset en proceso y renderizando el Job aparte, y el revisor lo aceptó.
- **Ratificado por el orquestador:** eliminación del puerto `ContainerRuntime` de U2-T03 (código muerto; el artefacto viaja por argumentos del Job); imagen `aqs-warm-deployer:0.1.0` como **placeholder** (no existe; su construcción o su sustitución es de U2-T07); `InferSurface` exige el deploy de la corrida `done` y el warm `dirty` (no `ready`, que tras un deploy no se da); `PUT /sessions` de `go-reset` y la clave `updated_at` del `warm-state` (aceptables, a llevar a contrato).
- **Un hallazgo que el orquestador no habría visto:** el reset sin tope en U2-T02 (28 lanzamientos y 26 handoffs duplicados con el gate caído al agotar los reintentos), un camino que ninguno de los 9 criterios ejercía (revisor, ronda 1, ROJO). Y en U2-T06, que un warm idle (0 réplicas) hacía que el rebuild semanal terminara siempre en cuarentena (ronda 2, NARANJA).
- **Incidencias de entorno:** los tres codificadores se cortaron una vez por el límite de uso de la API a mitad de ronda; se reanudaron en la misma ronda (un corte no cuenta como ronda). La CLI `docker` local es podman: sin `unqualified-search-registries` en `~/.config/containers/registries.conf` y sin `/etc/containers/nodocker` fallan los criterios que usan imágenes de nombre corto o capturan `2>&1`.
- **Coste en tokens de la ola en paralelo frente a la serie:** no medido (queda pendiente el punto 4 de la plantilla).
- **Decisión A sobre los Jobs sin token (humano, 2026-10-07):** deploy y reset **en proceso**, con la ServiceAccount de `go-warm-manager`/`go-reset` y el Role `aqs-test-operator` existente. Se descartó relajar `policy/isolation.rego` (opción B). Efecto: U2-T03b deja de ser «candados de prueba» y pasa a ser «deploy en proceso» (versión mínima, por indicación del humano de buscar la solución fácil: parche en proceso de la imagen de `warm-app`, solo `published-image`, estado en memoria; tras un reinicio el warm queda `dirty` y se resetea, sin resolución de huérfanos ni anotaciones); `build-from-repo` queda fuera hasta que exista un mecanismo de build (candidata). U2-T07 ya no construye imagen de deployer ni ServiceAccounts de Job.
