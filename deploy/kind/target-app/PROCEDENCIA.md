# Procedencia del baseline vendorizado

- Repositorio de origen: https://github.com/ogaston/demo-target-app-qa-swarm
- SHA fijado: `b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`
- Ruta en el origen: `baseline/baseline.sh`
- Copia: byte a byte (sin modificar). `scripts/kind/secrets.sh` la lee por defecto desde
  `deploy/kind/target-app/baseline.sh` (sobreescribible con `TARGET_APP_BASELINE_SCRIPT`).

Verificar la copia contra el origen en ese SHA:

```bash
diff <(curl -fsS https://raw.githubusercontent.com/ogaston/demo-target-app-qa-swarm/b97a719199d5ccbc2ff24f9cfc408ec7ee31114d/baseline/baseline.sh) deploy/kind/target-app/baseline.sh && echo BASELINE_IGUAL
```

Si el origen cambia de SHA, actualizar esta copia y este documento en el mismo commit.
