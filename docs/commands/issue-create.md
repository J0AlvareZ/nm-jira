# Crear un issue de Jira

## Sinopsis

```sh
nm:jira issue create <summary>
```

`summary` es el único argumento obligatorio.

## Comportamiento por defecto

- Proyecto: usa `--project` o el proyecto por defecto configurado.
- Labels: deduplica los valores; si no hay labels, usa `Support`.
- Assignee: se asigna por defecto solo si el proyecto es el proyecto predeterminado; en otro caso, no se asigna.
- Sin `--template`: no envía descripción, no lee templates ni abre un editor.

## Opciones

| Flag | Tipo/default | Comportamiento |
| --- | --- | --- |
| `--label, -l` | Repetible | Deduplica los valores; sin labels, usa `Support`. |
| `--component, -C` | String | Envía un componente por nombre. |
| `--parent, -P` | String | Crea un sub-task. |
| `--project, -p` | String | Si no se indica, usa el proyecto por defecto configurado. |
| `--assignee, -a` | String | Acepta accountId, email, `me`/`self`/`current` o una búsqueda de usuario. |
| `--template` | String; vacío | Lee `<config-dir>/nm-jira/templates/<nombre>.md`, abre el editor con el contenido precargado y envía el texto editado como descripción. |
| `--story-points` | String; `1` | Solo aplica con el proyecto `MRI`. |
| `--story-points-dev` | String; `1` | Solo aplica con el proyecto `MRI`. |
| `--type` | String; `Task` | Define el tipo de issue. |
| `--dry-run` | Bool | Imprime el curl POST a `/rest/api/3/issue` sin llamar a Jira. |

## Ejemplos

Creación mínima:

```sh
nm:jira issue create "Actualizar dependencias"
```

Labels repetibles, componente y tipo:

```sh
nm:jira issue create "Corregir autenticación" --label bug --label urgent --component API --type Bug
```

Sub-task con `--parent`:

```sh
nm:jira issue create "Implementar validación" --parent PROJ-123
```

Assignee con `me`:

```sh
nm:jira issue create "Revisar métricas" --assignee me
```

Proyecto `MRI` con story points:

```sh
nm:jira issue create "Planificar iteración" --project MRI --story-points 3 --story-points-dev 2
```

Dry-run:

```sh
nm:jira issue create "Validar solicitud" --dry-run
```

Crear desde un template:

```sh
nm:jira issue create "Agregar auditoría" --template feature
```

El nombre de template no puede estar vacío, incluir `/` o `\`, ni incluir la extensión `.md`. El comando busca el archivo exacto `<nombre>.md` dentro de `<config-dir>/nm-jira/templates/`. Si el directorio no existe, no es un directorio, no contiene templates `.md` o el archivo no existe, informa el nombre, la ruta y los templates disponibles; no crea archivos ni abre el editor.

## Salida y errores

En caso de éxito, el comando imprime:

```text
Created <key>: <summary>
```

Entre los errores conocidos se incluyen:

- `template "<nombre>" is unavailable: ...`
- `template "<nombre>" not found at "<path>"; available templates: ...`
- `editor exited with error: <causa>`
- `no Jira user found for "<referencia>"`
- `custom field "<nombre>" not found; story-related candidates: ...`

Los errores de Jira se propagan. El ejecutable escribe el prefijo `error:` en stderr y finaliza con exit code 1.
