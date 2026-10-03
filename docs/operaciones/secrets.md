# Secrets de la plataforma (SOPS + age)

El repo es **publico**: ningun secret entra en claro. Los Secrets de la app se cifran con SOPS + age y Flux los descifra
(`decryption: {provider: sops, secretRef: {name: sops-age}}` en `deploy/flux/clusters/<env>/aqs.yaml`).
La clave privada age nunca entra al repo. Quien ejecuta estos pasos es un humano, no el agente.

Secrets cubiertos (8): `minio-root`, `minio-kms`, `minio-tls`, `backup-target` (ns `aqs-system`),
`warm-db-credentials` (`aqs-test`) y `grafana-admin` (`aqs-observability`) los genera `generate.sh` (seccion 3);
`go-governance-service-token` y `go-identity-users` (ns `aqs-system`) se crean con el procedimiento de la seccion 6.

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

## 6. Secrets de go-governance y go-identity (a mano, no los genera `generate.sh`)

Los Deployments `go-governance` y `go-identity` referencian estos dos Secrets por nombre. **Los pods de `go-governance` y
`go-identity` no pasan `/readyz` hasta que existan los dos Secrets en el clúster**: es el comportamiento fail-closed
esperado, no un defecto. Los pasos los ejecuta un humano con la clave age; nunca pegues valores reales en el repo, en
issues ni en el historial del shell compartido.

### 6.1 `go-governance-service-token` (clave `token`)

Token de servicio de al menos 32 caracteres:

```bash
umask 077; d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
openssl rand -hex 32 > "$d/token"      # 64 caracteres hex
{
  printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: go-governance-service-token\n  namespace: aqs-system\n'
  printf 'type: Opaque\nstringData:\n  token: "%s"\n' "$(cat "$d/token")"
} > "$d/go-governance-service-token.yaml"
```

### 6.2 `go-identity-users` (clave `users.json`)

Formato: `[{"username": "<usuario>", "password_hash": "<hash PHC>", "role": "user|admin", "mfa_secret": "<base32>"}]`.
Roles `user` o `admin`; `mfa_secret` es obligatorio para cada `admin` (base32, al menos 160 bits, es decir 32 caracteres
base32) y opcional para `user`.

- `password_hash`: una vez por usuario, con la contraseña por stdin (8 a 128 caracteres), sin dejarla en argumentos:
  `printf '%s' "<contraseña>" | go-identity hash-password`.
- `mfa_secret` de un admin: `head -c 20 /dev/urandom | base32` (20 bytes = 160 bits). Entrégalo al admin por un canal seguro
  para su app TOTP.

```bash
umask 077; d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
cat > "$d/users.json" <<'JSON'
[
  {"username": "<usuario>", "password_hash": "<hash PHC>", "role": "user"},
  {"username": "<admin>", "password_hash": "<hash PHC>", "role": "admin", "mfa_secret": "<base32 de 160 bits o mas>"}
]
JSON
{
  printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: go-identity-users\n  namespace: aqs-system\n'
  printf 'type: Opaque\nstringData:\n  users.json: |\n'
  sed 's/^/    /' "$d/users.json"
} > "$d/go-identity-users.yaml"
```

### 6.3 Cifrar y registrar (ambos Secrets)

Mismo patrón que `generate.sh` (SOPS cifra solo `data`/`stringData`; el destinatario age se pasa como argumento):

```bash
for n in go-governance-service-token go-identity-users; do
  docker run --rm --security-opt label=disable -v "$d":/in:ro ghcr.io/getsops/sops:v3.9.1-alpine \
    sops --encrypt --age <age1-publica> --encrypted-regex '^(data|stringData)$' "/in/$n.yaml" \
    > deploy/flux/<env>/secrets/$n.sops.yaml
  grep -q 'ENC\[' deploy/flux/<env>/secrets/$n.sops.yaml || { echo "cifrado fallido: $n" >&2; exit 1; }
done
```

La ruta de destino es `deploy/flux/<env>/secrets/<nombre>.sops.yaml` (`<env>` es `dev` o `prod`). Añade a
`deploy/flux/<env>/secrets/kustomization.yaml`, bajo `resources:`, una línea por Secret:

```yaml
  - go-governance-service-token.sops.yaml
  - go-identity-users.sops.yaml
```

Commitea solo los `*.sops.yaml` cifrados, con una clave real y bajo revisión humana. Para rotar estos dos Secrets repite
esta sección con valores nuevos (y la clave age nueva si procede, sección 4) y reinicia los pods.

### 6.4 Comprobación en dev (la hace el humano; no la ejecuta el loop)

1. Tras la reconciliación de Flux: `kubectl -n aqs-system get pods -l aqs.io/tier=control-plane` muestra `go-governance` y
   `go-identity` en `Ready` (`/readyz` responde 200). Si siguen sin `Ready`, revisa que los dos Secrets existan.
2. El volumen de auditoría es escribible por el usuario 65532 (`fsGroup: 65532`): `kubectl -n aqs-system logs deploy/go-governance`
   no muestra `permission denied` sobre `/data/audit.jsonl`.
3. El archivo de usuarios es legible por el usuario no root: `kubectl -n aqs-system logs deploy/go-identity` no muestra errores
   de `IDENTITY_USERS_FILE`.
