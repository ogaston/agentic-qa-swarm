# Kimi Code — reglas de este proyecto

Este archivo solo agrega reglas de eficiencia de tokens para Kimi Code.
Para cualquier tarea de desarrollo de software, el flujo obligatorio es el de
`../AGENTS.md` (AI-DLC): léelo y síguelo; no lo reemplaza esta nota.

## Lectura de código (ahorro de contexto)

- Empieza con `Grep` y `Glob` para ubicar símbolos y rutas. No leas archivos
  completos si solo necesitas una función.
- Usa `Read` con `offset` y `limit` para rangos concretos. Evita leer archivos
  de más de 300 líneas sin necesidad.
- No leas completos `unidades-y-tareas.md`, `specs/prd.md`, `guide.md` ni
  `aidlc-docs/audit.md`: `grep -n '^#'` para ubicar la sección (o el ID de la
  unidad/tarea) y luego `Read` con `offset`/`limit`. `entradas/prd.md` es solo
  un puntero a `specs/prd.md`.
- No vuelvas a leer un archivo que ya está en la conversación; usa lo que ya tienes.
- El servidor MCP `codebase-memory-mcp` está configurado en
  `.kimi-code/mcp.json`. Úsalo primero para buscar símbolos (`search_graph`),
  trazar llamadas (`trace_path`) y leer fragmentos exactos (`get_code_snippet`);
  luego lee solo el código fuente relevante. Si el grafo no tiene cobertura de
  una ruta, usa `check_index_coverage` y vuelve a `Grep`.

## Salida de comandos

- Pasa `-q` / `--quiet` o filtra la salida (`| tail -n 50`, `| grep ...`) en
  pruebas, builds y `git log`. Nunca vuelques logs completos al contexto.
- Si la salida es grande, resume los fallos y cita solo las líneas relevantes.

## Respuestas

- Modo caveman en el chat: `.agents/skills/caveman/SKILL.md`, nivel por defecto.
  Respuesta primero, sin relleno, todos los hechos técnicos.
- Prosa normal para lo que se escribe en archivos (`aidlc-docs/`, `audit.md`,
  commits, PRs), advertencias de seguridad, confirmaciones de acciones
  irreversibles y los mensajes estandarizados de aprobación de AI-DLC (textuales).
- Código, comandos, rutas y errores exactos. Responde en el idioma del usuario.
- No repitas el contenido de archivos que ya editaste; indica la ruta y el cambio.
- `aidlc-docs/audit.md` es grande (100+ KB): no lo leas completo. Añade con
  `cat >> aidlc-docs/audit.md <<'EOF'` y usa `tail -n 40` para ver lo reciente.

## Estabilidad del prefijo (caché)

- No cambies el orden ni el contenido de este archivo entre sesiones sin motivo.
- No pongas fechas, IDs ni datos dinámicos al inicio de los prompts.
