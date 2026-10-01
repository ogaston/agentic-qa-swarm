# U0X-T0N — <título corto de la tarea>

> **Plantilla.** Copia este archivo a `tareas/U0X-T0N-<slug>.md` y llénalo. Cada tarea de tu
> `unidades-y-tareas.md` se convierte en uno de estos antes de poder despacharse.
>
> **Este archivo es el mundo entero del codificador.** No va a leer tu PRD. Si algo no está
> aquí, para él no existe.

**Unidad:** U0X — `<nombre de la unidad>`
**Historias que implementa:** US-X.Y
**Depende de:** `<U0X-T0M, o ninguna>`

---

## Alcance

**Dentro** (una línea, concreta):

> <qué se construye en esta tarea>

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- <cosa que parece relacionada y no lo es>
- <refactor tentador que pertenece a otra tarea>

> La lista de «fuera» no es burocracia: es lo que le permite al codificador **empujar de
> vuelta** cuando el revisor o su propio criterio le sugieren ampliarse. Sin esa lista, toda
> tarea crece hasta el tamaño del proyecto.

---

## Archivos de contexto

Rutas, no contenido pegado:

- `unidades-y-tareas.md`
- `aidlc-docs/construction/plans/U0X-*-code-generation-plan.md`
- `aidlc-docs/inception/application-design/unit-of-work-story-map.md`
- `<cualquier otro archivo que el codificador necesite leer>`

---

## Criterios de aceptación

Cada uno con **su comando**. El revisor los va a correr él mismo, uno por uno.

- [ ] **CA-1** — <qué debe ser cierto>
  ```bash
  <comando exacto>
  ```
  Esperado: <qué salida demuestra que pasa>

- [ ] **CA-2** — <qué debe ser cierto>
  ```bash
  <comando exacto>
  ```
  Esperado: <qué salida demuestra que pasa>

> **Si un criterio no tiene comando, la tarea no está lista y no se despacha.** «Funciona
> correctamente» no es un criterio: es una opinión con forma de criterio.
>
> **Si el comportamiento cambia estado**, el criterio nombra la **lectura de vuelta con la
> herramienta real** (`kubectl get`, `helm status`, `git log`), no el código de salida del
> programa. Un programa que dice que funcionó no es que funcionó.

---

## Plan de pruebas

Qué pruebas se escriben **en esta tarea**, no después:

- <prueba unitaria de ...>
- <prueba negativa de ...>

**Rojo primero:** antes de cualquier arreglo, el codificador reproduce el fallo y lo registra.
El par falla-pasa es la prueba. Una prueba que nunca se vio fallar no demuestra nada.

---

## Notas

- Archivos que se **modifican en su sitio**: `<lista>`. Nada de duplicados con sufijo.
- <cualquier trampa conocida, decisión ya tomada, o límite que el codificador debe respetar>

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
