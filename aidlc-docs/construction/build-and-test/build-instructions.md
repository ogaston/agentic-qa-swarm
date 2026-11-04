# Instrucciones de build

## Requisitos
- Go (módulos independientes en `services/*`), Node 20+ (`web/dashboard`), Python 3.11+ con venv (`agents/*`), `bash`.
- Entorno local con kind y podman solo para las pruebas de integración (`scripts/kind/`).

## Pasos (desde la raíz del repositorio)
1. Go: `for s in services/*/; do (cd $s && go vet ./... && go build ./...); done`
2. Dashboard: `cd web/dashboard && npm ci && npm run -s lint && npm run -s typecheck && npm run build`
3. Agentes: `cd agents/eval && python -m venv .venv && .venv/bin/pip install -r requirements-dev.txt`
4. Contratos: `bash contracts/validate.sh`

## Resultado esperado
Sin errores de `go vet`/lint/typecheck; `web/dashboard/dist` generado; `contrato-ok`.
