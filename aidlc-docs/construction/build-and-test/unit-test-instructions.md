# Ejecución de pruebas unitarias y de contrato

- Go: `for s in services/*/; do (cd $s && go test -count=1 ./...); done`
- Dashboard: `cd web/dashboard && npx vitest run`
- Evaluación offline de agentes: `cd agents/eval && .venv/bin/python -m pytest -q`
- Agentes (planner/reporter): `.venv/bin/python -m pytest -q` en cada directorio con venv.
- Contratos: `bash contracts/validate.sh`

## Integración y extremo a extremo (requieren clúster kind)
`bash scripts/kind/kind-smoke.sh`, `bash scripts/ci/rbac-matrix.sh`, `bash scripts/test/u12-self-service-kind.sh`.
Guion de la demo real: `docs/demo-real.md`.

## Rendimiento
No aplica al alcance de la entrega: los tiempos de la corrida real se registran en `docs/demo-real.md`.
