# Build y releases

## CI

El workflow de CI se ejecuta en:

- Pull requests.
- Pushes a `main`.
- Tags que comienzan con `v`.

CI ejecuta:

1. `golangci-lint run ./...`
2. `go test ./...`
3. `go build ./...`
4. Builds de validación cruzada para: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64, arm64).

La CI no recibe ni utiliza el secreto OAuth de Jira. Los builds de CI usan los valores por defecto de versión y no incluyen el secreto embebido.

## Publicar un release

Los releases se crean exclusivamente al hacer push de un tag SemVer estricto:

```bash
git tag v1.2.3
git push origin v1.2.3
```

El workflow rechaza tags que no cumplan SemVer estricto. Son válidos: `v1.2.3`, `v1.2.3-rc.1`, `v1.2.3+build.5`.

## Environment y secreto

El repositorio debe tener un GitHub Environment llamado `release`. El secreto OAuth se configura únicamente en ese Environment:

```text
JIRA_OAUTH_CLIENT_SECRET
```

El job de compilación del release usa el Environment `release`. El secreto se inyecta solo como variable de entorno en el paso que compila cada binario y se incorpora mediante `-ldflags`. No debe configurarse como repository secret ni exponerse a CI.

## Qué hace el pipeline de release

1. Valida el tag SemVer.
2. Obtiene versión sin `v`, commit corto y fecha UTC.
3. Compila los seis targets.
4. Embebe versión, commit, fecha y el secreto OAuth con `-ldflags`.
5. Empaqueta binarios Unix como `tar.gz` y Windows como `zip`.
6. Genera `SHA256SUMS`.
7. Crea el GitHub Release en el mismo repositorio y adjunta los assets.

## Matriz y assets

| Target | Asset |
| --- | --- |
| Linux amd64 | `nm-jira_linux_amd64.tar.gz` |
| Linux arm64 | `nm-jira_linux_arm64.tar.gz` |
| macOS amd64 | `nm-jira_darwin_amd64.tar.gz` |
| macOS arm64 | `nm-jira_darwin_arm64.tar.gz` |
| Windows amd64 | `nm-jira_windows_amd64.zip` |
| Windows arm64 | `nm-jira_windows_arm64.zip` |
| Checksums | `SHA256SUMS` |

Verificación:

```bash
sha256sum -c SHA256SUMS
```
