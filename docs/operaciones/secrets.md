# Secrets de la plataforma (SOPS + age)

El repo es **publico**: ningun secret entra en claro. Los Secrets de la app se cifran con SOPS + age y Flux los descifra
(`decryption: {provider: sops, secretRef: {name: sops-age}}` en `deploy/flux/clusters/<env>/aqs.yaml`).
La clave privada age nunca entra al repo. Quien ejecuta estos pasos es un humano, no el agente.

Secrets cubiertos (6): `minio-root`, `minio-kms`, `minio-tls`, `backup-target` (ns `aqs-system`),
`warm-db-credentials` (`aqs-test`) y `grafana-admin` (`aqs-observability`).

## 1. Generar la clave age

```bash
age-keygen -o age.key          # imprime la clave publica (age1...); age.key es la privada
```

Guarda `age.key` en un gestor de secretos fuera del repo. Nunca la commitees.

## 2. Crear `sops-age` en el clúster (a mano, una vez por clúster)

```bash
kubectl -n flux-system create secret generic sops-age --from-file=age.agekey=age.key
```

El Secret `sops-age` vive solo en el clúster. Despues borra la copia local de `age.key` si ya esta en el gestor.

## 3. Generar y cifrar los Secrets

```bash
BACKUP_ENDPOINT=https://... BACKUP_BUCKET=... BACKUP_ACCESS_KEY_ID=... BACKUP_SECRET_ACCESS_KEY=... \
  scripts/secrets/generate.sh <dev|prod> <age1-publica>
```

El script crea contraseñas aleatorias, la CA y el certificado de MinIO (SAN `minio.aqs-system.svc` y
`minio.aqs-system.svc.cluster.local`) y `MINIO_KMS_SECRET_KEY` (`aqs-kms:<base64 de 32 bytes>`); falla si falta
alguna variable `BACKUP_*`; escribe `deploy/flux/<env>/secrets/*.sops.yaml` cifrados y actualiza `secrets/kustomization.yaml`.
Los temporales van en `mktemp -d` y se borran al salir. Valores en `stringData`; SOPS cifra solo `data`/`stringData`.
Commitea los `*.sops.yaml` solo con una clave real y bajo revision humana. `scripts/ci/check-secrets.sh` (parte de
`scripts/ci/policies.sh`) falla si algun Secret bajo `deploy/` no esta cifrado. No hay `.sops.yaml` en el repo: el
destinatario siempre se pasa como argumento.

## 4. Rotar

1. Genera una clave age nueva (paso 1) y actualiza `sops-age` (paso 2, `--dry-run=client -o yaml | kubectl apply -f -`).
2. Vuelve a ejecutar `generate.sh` con la nueva clave publica: regenera todos los valores y los cifra con la clave nueva.
3. Commit, PR y revision; Flux reconcilia. Reinicia los pods que consumen los Secrets si no recargan solos.
4. Destruye la clave antigua tras confirmar la reconciliacion.

Nota: regenerar cambia las credenciales de MinIO y de Grafana; planifica el cambio (gestion-de-cambios.md).

## 5. Si se filtra la clave age

1. Trata **todos** los valores cifrados con esa clave como comprometidos (el historial git es publico).
2. Rota segun la seccion 4 de inmediato: clave nueva y valores nuevos (no basta recifrar los viejos).
3. Cambia tambien las credenciales externas de `backup-target` en el proveedor del bucket.
4. Registra el incidente (respuesta-a-incidentes.md).
