# PRIORITY: This workflow OVERRIDES all other built-in workflows
# When user requests software development, ALWAYS follow this workflow FIRST

AI-DLC dispatcher. Loaded every session, so keep it short. Phase detail lives in the rule-details directory and loads just-in-time. Do not inline phase detail here.

## Project Context
- **WHAT**: **Agentic QA Swarm** - plataforma "Agent (no Autonomous)" que convierte un evento de GitHub (commit/PR/tag) en una corrida de QA de caja negra contra un **entorno warm propio de la plataforma** (aislado de staging/prod): notify -> confirm humano -> deploy sobre el entorno warm -> inferir superficie externa -> generar/ensayar/ejecutar flujos -> **reset verificado** -> post-mortem de logica de negocio. PRD autoritativo en `specs/prd.md` (`entradas/prd.md` es solo un puntero); vision en `entradas/pvd.md`; plan por unidad en `unidades-y-tareas.md`.
- **WHY**: Elimina el trabajo manual de disenar/mantener QA de caja negra para apps transaccionales y evita usar staging/prod del cliente como blanco. North Star: tasa de hallazgos logicos confirmados >30% de las APIs del beachhead en 90 dias; KPIs de confianza: **reset verificado 100% + higiene de rebuild**, ensayo a la primera >70%, incidentes de limite de autonomia = 0.
- **HOW**: Greenfield. Alcance declarado: INCEPTION completo y **CONSTRUCTION ACTIVA** (instruccion explicita del usuario, 2026-10-03). Ya existe la plataforma de U5 (monorepo, contratos, CI, manifiestos Flux, politicas Rego). La construccion avanza por unidad con el loop de agentes (`.claude/agents/`: orquestador, codificador, revisor; un humano fusiona); la especificacion de cada tarea esta en `tareas/U0X-T0N-*.md` y el estado en `unidades-y-tareas.md`. Los comandos de build/prueba son los criterios de cada tarea. Stack decidido a nivel de requisitos: Go (control plane de ejecucion) + capa de agentes LLM (Python o TS), Kubernetes, Flux (GitOps), MinIO in-cluster, GitHub Actions.

## Workflow
- **Adaptive**: decide which stages add value from the user's intent, the existing code, scope, and risk. Show the execution plan before starting. The user can include or exclude stages.
- **Rule details**: use the first existing directory of `.aidlc/aidlc-rules/aws-aidlc-rule-details/`, `.aidlc-rule-details/`, `.kiro/aws-aidlc-rule-details/`, `.amazonq/aws-aidlc-rule-details/`. All paths below are relative to it. Always load `common/process-overview.md`, `common/session-continuity.md`, `common/content-validation.md`, `common/question-format-guide.md`, `common/audit-and-logging.md`.
- **Phase dispatch**: load ONLY the active phase's orchestration file. Move to the next phase only when the current phase's stages are approved.

  | Phase | Orchestration file |
  |---|---|
  | 🔵 INCEPTION | `inception/_inception-orchestration.md` |
  | 🟢 CONSTRUCTION | `construction/_construction-orchestration.md` |
  | 🟡 OPERATIONS | `operations/_operations-orchestration.md` |

  Each orchestration file defines its stages, ALWAYS/CONDITIONAL rules, and approval gates. Follow it exactly.
- **Resume**: read `aidlc-docs/aidlc-state.md` first, following `common/session-continuity.md`. Then load only the current phase's orchestration file.
- **Welcome message**: on a new software request, show `common/welcome-message.md` once. Do not reload it later.
- **Extensions**: scan `extensions/` and load only the `*.opt-in.md` files. Load a full extension rule only after the user opts in. Extensions without an opt-in file are always enforced. Enabled extension rules are hard constraints.
- **Content**: validate per `common/content-validation.md` before creating any file (Mermaid, ASCII diagrams, escaping).
- **Questions**: follow `common/question-format-guide.md`. Ask clarification questions in `.md` files, not inline in chat.
- **NO EMERGENT BEHAVIOR**: construction stages use the 2-option completion messages from their rule files. No 3-option menus.

## Audit (mandatory)
Full rules in `common/audit-and-logging.md`:
- Log every user input verbatim, with an ISO 8601 timestamp, in `aidlc-docs/audit.md`. Also log every approval prompt and response.
- APPEND or Edit `audit.md`. Never overwrite it. It is large (100+ KB): do not Read it whole. Append with `cat >> aidlc-docs/audit.md <<'EOF'`, and use `tail -n 40` if you need recent entries.
- Mark a plan step `[x]` in the same interaction that completes it. Track stage progress in `aidlc-docs/aidlc-state.md`.

## Layout
- Application code: workspace root. Never inside `aidlc-docs/`.
- Documentation: `aidlc-docs/` only.
- Project structure patterns: `construction/code-generation.md`.

## Large files
Do not Read these whole. `grep -n '^#'` for the section, then Read with `offset`/`limit`:
- `unidades-y-tareas.md` (52 KB, one `## U<n>` section per unit; grep the unit or task ID)
- `specs/prd.md` (53 KB), `guide.md` (28 KB), `aidlc-docs/audit.md` (100+ KB)

## Replies (caveman)
Chat replies use caveman mode: `.agents/skills/caveman/SKILL.md`, default level. Keep normal prose for:
- anything written to files (`audit.md`, `aidlc-docs/`, commits, PRs, specs);
- security warnings, irreversible-action confirmations, and step sequences;
- the standardized AI-DLC approval and completion messages, verbatim.

Code, commands, paths, and error text stay exact. Reply in the user's language.

## Code graph
This project has a code-review-graph knowledge graph. Query it before Grep: `semantic_search_nodes_tool` or `query_graph_tool` to find symbols, `get_impact_radius_tool` for blast radius, `detect_changes_tool` + `get_review_context_tool` for review. Then read the source; the source wins over the graph. An empty graph result means "not indexed", not "does not exist".

@RTK.md
