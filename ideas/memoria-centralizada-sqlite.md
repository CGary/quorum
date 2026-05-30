# 🧠 Propuesta Técnica: Memoria Centralizada Multi-Proyecto con SQLite

**Estado:** Propuesta para convertir en feature mediante el flujo Quorum.  
**Fecha:** 2026-05-30.  
**Contexto:** Evolución de Quorum hacia memoria operativa multi-proyecto.  
**Origen:** Necesidad de usar Quorum en varios proyectos sin fragmentar la experiencia acumulada en múltiples carpetas `memory/` locales.  
**Veredicto:** La memoria actual existe como corpus curado, validable y versionado, pero casi ningún proceso runtime la consume automáticamente. Para que deje de ser almacenamiento pasivo, Quorum necesita una base central SQLite, multi-proyecto, administrada por el propio Quorum.

---

## 1. Problema

Quorum puede usarse en varios proyectos. Hoy cada proyecto conserva su propia carpeta:

```text
memory/
  decisions/*.json
  patterns/*.json
  lessons/*.json
```

Esto genera fragmentación:

1. La experiencia queda dispersa por repositorio.
2. No existe una base común de decisiones, patrones y lecciones.
3. Los JSON locales no son consumidos automáticamente por `q-blueprint`, `q-analyze`, `q-implement`, `q-review` ni routing.
4. La futura evolución hacia RAG necesita una estructura central, consultable e indexable.
5. Mantener carpetas `memory/` por proyecto añade duplicación y ruido operacional.

Observación técnica:

> Si ningún proceso consume activamente `memory/*.json`, la memoria funciona más como archivo histórico que como conocimiento operativo.

---

## 2. Objetivo

Crear una memoria centralizada multi-proyecto respaldada por SQLite.

La memoria local `memory/` debe migrarse a SQLite durante `quorum init` y luego eliminarse del proyecto local. A partir de ese momento, la memoria durable se guarda mediante comandos de Quorum contra la base central.

El objetivo inicial NO incluye RAG, embeddings ni vector search. RAG es contexto futuro y no debe expandir el alcance de la primera feature.

---

## 3. Archivo de metaparámetros del proyecto

Cada proyecto debe declarar solamente su identidad local mediante un archivo de metaparámetros.

Nombre candidato:

```text
.quorumrc
```

Alternativa aceptable si el blueprint la prefiere:

```text
.quorum/config.yaml
```

Esta propuesta usa `.quorumrc` como nombre provisional.

### 3.1 Contenido mínimo propuesto

```yaml
project_id: quorum
project_name: Quorum
```

### 3.2 Campos

| Campo | Requerido | Descripción |
|---|---:|---|
| `project_id` | Sí | Identificador estable del proyecto dentro de la base central. Debe evitar colisiones entre proyectos. |
| `project_name` | Sí | Nombre humano del proyecto. |

La raíz del proyecto no es un metaparámetro versionado: se calcula en runtime. Si la DB necesita almacenar `root_path` como metadata local en `projects`, ese valor se deriva dinámicamente y puede actualizarse cuando el repo se mueve.

### 3.3 Lo que NO debe contener `.quorumrc`

`.quorumrc` NO debe contener rutas locales absolutas.

No debe contener la ruta de la base SQLite ni `project_root`. La ubicación de la base debe ser responsabilidad de Quorum, no del proyecto. La raíz del proyecto debe seguir resolviéndose dinámicamente con la lógica existente (`git rev-parse --show-toplevel` y fallback por búsqueda ascendente). Esto evita romper clones en otros equipos o CI y permite que el mismo proyecto use una base real, temporal o de pruebas sin modificar archivos versionados.

---

## 4. Ubicación de la base SQLite

La ruta de la base debe resolverse por una política central de Quorum.

### 4.1 Propuesta recomendada

Orden de resolución:

1. Variable de entorno explícita:

```bash
QUORUM_MEMORY_DB=/ruta/a/memory.db
```

2. Ruta por defecto administrada por Quorum:

```text
~/.quorum/memory.db
```

### 4.2 Justificación

La variable de entorno permite pruebas aisladas:

```bash
QUORUM_MEMORY_DB=$(mktemp -u)/test-memory.db go test ./...
```

O:

```bash
QUORUM_MEMORY_DB=/tmp/quorum-test-memory.db quorum init
```

Ventajas:

- No ensucia `.quorumrc` con rutas específicas de una máquina.
- Permite CI y tests con bases temporales.
- Permite al usuario cambiar de base sin tocar el repo.
- Mantiene la identidad del proyecto separada del storage físico.

### 4.3 Alternativa posible

Si se requiere más configuración global a futuro, usar un archivo global:

```text
~/.quorum/config.yaml
```

Ejemplo:

```yaml
memory_database: ~/.quorum/memory.db
```

Aun así, `QUORUM_MEMORY_DB` debe tener prioridad para tests y override temporal.

---

## 5. Comportamiento esperado de `quorum init`

`quorum init` debe convertirse en el punto de incorporación de memoria centralizada.

### 5.1 Proyecto nuevo sin `.quorumrc`

Cuando se ejecuta:

```bash
quorum init
```

Si no existe `.quorumrc`, el comando debe obtener o generar:

- `project_id`
- `project_name`
- raíz del proyecto resuelta dinámicamente en runtime

Luego debe:

1. Crear `.quorumrc` sin rutas absolutas locales.
2. Resolver la raíz del proyecto dinámicamente.
3. Resolver la ruta SQLite usando `QUORUM_MEMORY_DB` o `~/.quorum/memory.db`.
4. Crear la base si no existe.
5. Configurar SQLite con PRAGMAs obligatorios (`WAL`, `busy_timeout`, `foreign_keys`).
6. Registrar el proyecto en la tabla `projects`.
7. Continuar con el scaffolding normal de Quorum.

### 5.2 Proyecto existente con `memory/` local y sin `.quorumrc`

Si `quorum init` detecta archivos en:

```text
memory/decisions/*.json
memory/patterns/*.json
memory/lessons/*.json
```

y no existe `.quorumrc`, entonces debe exigir los metaparámetros mínimos antes de continuar.

Flujo deseado:

1. Detectar memorias locales existentes.
2. Informar que se requiere identidad del proyecto para migración central.
3. Crear `.quorumrc` con `project_id` y `project_name`, sin rutas absolutas.
4. Resolver la raíz del proyecto dinámicamente.
5. Resolver la base SQLite por variable de entorno o default global.
6. Configurar SQLite con PRAGMAs obligatorios (`WAL`, `busy_timeout`, `foreign_keys`).
7. Validar cada memoria local contra `memory.schema.json`.
8. Insertar todas las memorias en SQLite dentro de una transacción.
9. Registrar hashes de contenido y origen.
10. Si todo fue exitoso, eliminar los archivos locales de `memory/`.
11. Eliminar directorios `memory/decisions`, `memory/patterns`, `memory/lessons` si quedan vacíos.
12. Continuar el init normal.

### 5.3 Proyecto existente con `.quorumrc`

Si `.quorumrc` existe:

1. Validar que `project_id` y `project_name` existen.
2. Resolver la raíz del proyecto dinámicamente.
3. Resolver la base SQLite.
4. Configurar SQLite con PRAGMAs obligatorios (`WAL`, `busy_timeout`, `foreign_keys`).
5. Asegurar que el proyecto existe en `projects`.
6. Si todavía hay `memory/*.json`, migrarlos y eliminarlos.
7. Continuar init.

### 5.4 Requisito de seguridad para eliminar memoria local

Aunque la intención es eliminar `memory/` local después de migrar, la eliminación solo puede ocurrir si:

1. Todas las entradas fueron validadas.
2. La transacción SQLite completó exitosamente.
3. Cada entrada insertada puede verificarse por `(project_id, id)` y `content_hash`.
4. No hubo conflictos no resueltos.

Si algo falla, `quorum init` debe conservar `memory/` intacto y reportar el error.

---

## 6. Nuevo comando de guardado de memoria

`q-memory` debe dejar de escribir directamente en `memory/*.json` y persistir mediante un comando de Quorum.

Nombre recomendado:

```bash
quorum memory save
```

### 6.1 Responsabilidades de `quorum memory save`

1. Cargar `.quorumrc`.
2. Resolver la base SQLite mediante `QUORUM_MEMORY_DB` o default global.
3. Validar la entrada contra `memory.schema.json` o su evolución.
4. Insertar la memoria en SQLite.
5. Persistir `related`, `supersedes` y `anti_patterns` relacionalmente.
6. Calcular y guardar `content_hash`.
7. Rechazar duplicados conflictivos.
8. Emitir resultado machine-readable.

Ejemplos:

```bash
cat memory-entry.json | quorum memory save
```

```bash
quorum memory save --file /tmp/LES-2026-05-30-1.json
```

---

## 7. Análisis de comandos sugeridos

La propuesta inicial listaba muchos comandos:

```bash
quorum memory save
quorum memory sync
quorum memory migrate
quorum memory search
quorum memory list
quorum memory status
quorum memory export
quorum memory validate
```

Después de fijar que `quorum init` migra automáticamente y elimina memoria local, no todos siguen teniendo el mismo valor.

### 7.1 Comandos que sí valen para la primera feature

| Comando | Mantener | Razón |
|---|---:|---|
| `quorum memory save` | Sí | Es el backend de persistencia para `q-memory`. |
| `quorum memory status` | Sí | Permite diagnosticar `.quorumrc`, DB resuelta, proyecto registrado y cantidad de memorias. |
| `quorum memory search` | Sí, si el costo es bajo | Es el primer consumidor real de la memoria central. Sin búsqueda, SQLite sería solo almacenamiento. |

### 7.2 Comandos que pueden eliminarse o diferirse

| Comando | Decisión propuesta | Razón |
|---|---|---|
| `quorum memory sync` | No necesario en v1 | Si `init` migra y borra locales, no hay modo espejo que sincronizar. |
| `quorum memory migrate` | Diferir o hacerlo interno | La migración ocurre durante `quorum init`; un comando público solo se justifica para reparación manual. |
| `quorum memory list` | Diferir | `search` puede cubrir listados básicos con filtros vacíos o `--all`. |
| `quorum memory export` | Diferir | Útil para backup/auditoría, pero no imprescindible para la primera feature. |
| `quorum memory validate` | No necesario | Ya existe `quorum validate`; `save` e `init` deben validar internamente. |

### 7.3 Set mínimo recomendado

Primera feature:

```bash
quorum memory save
quorum memory status
quorum memory search
```

Internamente, `quorum init` debe tener funciones de migración, pero no necesariamente exponerlas como comando público.

### 7.4 Comando de reparación opcional

Puede considerarse un comando administrativo en una segunda etapa:

```bash
quorum memory repair
```

Responsabilidades futuras:

- reintentar migración interrumpida;
- verificar hashes;
- detectar entradas faltantes;
- reconstruir FTS;
- exportar backup si se agrega esa capacidad.

---

## 8. Concurrencia e inicialización SQLite

SQLite será una base central compartida por varios procesos de Quorum, potencialmente ejecutados desde distintos repositorios o worktrees. Por eso la conexión debe configurarse de forma uniforme desde un helper central.

PRAGMAs obligatorios al abrir/inicializar la base:

```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;
```

Justificación:

| PRAGMA | Razón |
|---|---|
| `journal_mode=WAL` | Mejora concurrencia entre lectores y escritores y evita bloqueos innecesarios. |
| `busy_timeout=5000` | Evita fallos inmediatos `database is locked` durante escrituras concurrentes razonables. |
| `foreign_keys=ON` | SQLite no aplica claves foráneas por defecto; es obligatorio para evitar datos huérfanos. |

La configuración debe vivir en un único helper de apertura, por ejemplo `OpenMemoryDB`, para evitar conexiones sin PRAGMAs.

---

## 9. Esquema SQLite inicial

El diseño debe favorecer migraciones futuras y escalabilidad.

### 9.1 Tabla `projects`

```sql
CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT,
  git_remote TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

### 9.2 Tabla `memory_entries`

```sql
CREATE TABLE memory_entries (
  project_id TEXT NOT NULL,
  id TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('pattern', 'decision', 'lesson')),
  source_task TEXT NOT NULL,
  title TEXT NOT NULL,
  context TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  supersedes TEXT,
  source_path TEXT,
  content_hash TEXT NOT NULL,
  imported_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  PRIMARY KEY (project_id, id),
  FOREIGN KEY (project_id) REFERENCES projects(id)
);
```

### 9.3 Tabla `memory_related`

```sql
CREATE TABLE memory_related (
  project_id TEXT NOT NULL,
  memory_id TEXT NOT NULL,
  related_ref TEXT NOT NULL,
  PRIMARY KEY (project_id, memory_id, related_ref),
  FOREIGN KEY (project_id, memory_id)
    REFERENCES memory_entries(project_id, id)
    ON DELETE CASCADE
);
```

### 9.4 Tabla `memory_anti_patterns`

```sql
CREATE TABLE memory_anti_patterns (
  project_id TEXT NOT NULL,
  memory_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  content TEXT NOT NULL,
  PRIMARY KEY (project_id, memory_id, ordinal),
  FOREIGN KEY (project_id, memory_id)
    REFERENCES memory_entries(project_id, id)
    ON DELETE CASCADE
);
```

### 9.5 Tabla `memory_supersession_edges`

```sql
CREATE TABLE memory_supersession_edges (
  project_id TEXT NOT NULL,
  from_id TEXT NOT NULL,
  to_id TEXT NOT NULL,
  PRIMARY KEY (project_id, from_id, to_id),
  FOREIGN KEY (project_id, from_id)
    REFERENCES memory_entries(project_id, id)
    ON DELETE CASCADE
);
```

Nota: `to_id` puede referenciar una memoria aún no importada o de otro contexto histórico. Si se decide exigir que siempre exista localmente, agregar una segunda FK compuesta para `(project_id, to_id)`.

### 9.6 Tabla `init_migrations`

```sql
CREATE TABLE init_migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  entries_seen INTEGER NOT NULL DEFAULT 0,
  entries_inserted INTEGER NOT NULL DEFAULT 0,
  entries_failed INTEGER NOT NULL DEFAULT 0,
  removed_local_files INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error TEXT
);
```

### 9.7 Índices

```sql
CREATE INDEX idx_memory_entries_type ON memory_entries(type);
CREATE INDEX idx_memory_entries_project_type ON memory_entries(project_id, type);
CREATE INDEX idx_memory_entries_source_task ON memory_entries(project_id, source_task);
CREATE INDEX idx_memory_entries_created_at ON memory_entries(created_at);
CREATE INDEX idx_memory_entries_hash ON memory_entries(content_hash);
```

### 9.8 FTS5 para búsqueda textual

Si SQLite tiene FTS5 disponible:

```sql
CREATE VIRTUAL TABLE memory_fts USING fts5(
  project_id,
  memory_id,
  type,
  title,
  context,
  content,
  anti_patterns
);
```

Si FTS5 no está disponible, `search` puede comenzar con `LIKE` simple y dejar FTS como mejora posterior.

---

## 10. Preparación para RAG futuro

RAG queda fuera del alcance inicial, pero el schema no debe bloquearlo.

Tablas futuras posibles:

```sql
CREATE TABLE memory_chunks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  memory_id TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  text TEXT NOT NULL,
  token_count INTEGER,
  content_hash TEXT NOT NULL
);
```

```sql
CREATE TABLE memory_embeddings (
  chunk_id INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  embedding BLOB NOT NULL,
  created_at TEXT NOT NULL
);
```

Estas tablas no deben implementarse en la primera feature salvo decisión explícita del blueprint.

---

## 11. Integración con skills

### 10.1 `q-memory`

Debe evolucionar de escribir JSON local a persistir mediante:

```bash
quorum memory save
```

El skill sigue siendo la compuerta humana de curación. No debe convertirse en ingesta automática.

### 10.2 `q-blueprint` y `q-analyze`

Una segunda feature debe hacer que estos procesos consulten memoria relevante mediante SQLite, por ejemplo:

```bash
quorum memory search --project <project_id> --type lesson "schema validation"
```

O mediante un comando especializado futuro:

```bash
quorum memory context --task <TASK_ID>
```

Esto es lo que convierte la memoria de pasiva a operativa.

---

## 12. Cambios esperados en código

Módulos probables:

```text
cmd/memory.go
cmd/memory_save.go
cmd/memory_status.go
cmd/memory_search.go
internal/core/quorum_config.go
internal/core/memory_store.go
internal/core/memory_sqlite.go
internal/core/memory_init_migration.go
internal/core/memory_search.go
```

Cambios existentes probables:

```text
cmd/init.go
internal/core/task_manager.go
internal/core/schema.go
.agents/skills/q-memory/SKILL.md
.agents/schemas/memory.schema.json
README.md
quorum.md
```

Si SQLite pasa a ser fuente canónica, debe agregarse ADR en:

```text
docs/adr/
```

---

## 13. Requisitos funcionales candidatos

1. `quorum init` crea `.quorumrc` si falta.
2. `.quorumrc` contiene identidad del proyecto, sin ruta de DB ni `project_root`.
3. La DB se resuelve con `QUORUM_MEMORY_DB` o `~/.quorum/memory.db`.
4. `quorum init` registra el proyecto en SQLite con la raíz resuelta dinámicamente.
5. `quorum init` detecta `memory/*.json` locales.
6. `quorum init` valida e importa memorias locales dentro de una transacción.
7. `quorum init` elimina memorias locales solo después de confirmar inserción por hash.
8. `quorum init` conserva memorias locales si la migración falla.
9. `quorum memory save` persiste nuevas memorias en SQLite.
10. `quorum memory status` diagnostica config, DB y conteos.
11. `quorum memory search` consulta memorias por texto, proyecto y tipo.
12. La base soporta múltiples proyectos sin colisiones de IDs.
13. `related`, `supersedes` y `anti_patterns` se preservan relacionalmente.
14. La conexión SQLite usa `WAL`, `busy_timeout` y `foreign_keys=ON`.
15. Las tablas satélite tienen claves foráneas con `ON DELETE CASCADE` donde corresponda.

---

## 14. Requisitos no funcionales

1. Migración idempotente.
2. Escrituras transaccionales.
3. Sin dependencia de servicios externos.
4. Sin RAG ni embeddings en primera implementación.
5. Tests aislables mediante `QUORUM_MEMORY_DB`.
6. Compatibilidad con proyectos legacy que ya tienen `memory/` local.
7. Errores claros cuando `.quorumrc` es inválido o hay conflicto de `project_id`.
8. No borrar archivos locales si la migración no está totalmente verificada.


---

## Recomendación arquitectónica final

La decisión recomendada para esta propuesta es:

> SQLite será la fuente operativa principal de memoria durable multi-proyecto. Los JSON locales `memory/*.json` se migran durante `quorum init` y se eliminan después de verificación exitosa. Para no convertir SQLite en una caja negra irreversible, debe planificarse `quorum memory export` como capacidad futura de backup/auditoría, aunque no sea parte obligatoria de la primera feature.

Esto adopta la intención de centralizar y eliminar duplicación local, pero conserva una salida técnica para auditoría, portabilidad y recuperación.

---

## 15. Riesgos abiertos

| Riesgo | Mitigación |
|---|---|
| Pérdida de datos al borrar `memory/` local. | Transacción + verificación por hash antes de eliminar. |
| Colisión de `project_id` entre proyectos. | Detectar `root_path`/remote divergente y pedir resolución. |
| `.quorumrc` se vuelve no portable. | No guardar ruta de DB; solo identidad del proyecto. |
| SQLite se vuelve opaco. | Guardar `raw_json`, hashes y metadata de origen. |
| FTS5 no disponible. | Fallback a búsqueda simple y tests condicionales. |
| `quorum init` se vuelve demasiado interactivo. | Soportar flags para CI/no interactivo. |

---

## 16. Preguntas para convertir en feature

1. ¿El archivo final será `.quorumrc` o `.quorum/config.yaml`?
2. ¿Cómo generar `project_id` por defecto: slug del repo, remote Git o prompt humano?
3. ¿Qué hacer si el `project_id` ya existe con otro `root_path` o remote?
4. ¿`quorum init` será interactivo o aceptará flags como `--project-id` y `--project-name`?
5. ¿`quorum memory search` entra en la primera feature o queda para una segunda?
6. ¿Debe existir un comando administrativo `quorum memory repair` para recuperar migraciones fallidas?
7. ¿Cómo documentar constitucionalmente que SQLite reemplaza a `memory/` local como fuente de memoria durable?

---

## 17. Alcance recomendado para primera feature

Implementar:

1. `.quorumrc` con identidad del proyecto, sin rutas absolutas.
2. Resolución dinámica de raíz del proyecto.
3. Resolución de DB por `QUORUM_MEMORY_DB` y fallback `~/.quorum/memory.db`.
4. Inicialización de schema SQLite con PRAGMAs obligatorios.
5. Migración automática desde `memory/*.json` durante `quorum init`.
6. Eliminación segura de memorias locales después de migración verificada.
7. `quorum memory save`.
8. `quorum memory status`.
9. Tests de migración, eliminación segura, concurrencia básica y DB temporal.

Opcional si el costo es bajo:

1. `quorum memory search` con búsqueda simple o FTS5.

Dejar para segunda feature:

1. `q-blueprint` consumiendo memoria automáticamente.
2. `q-analyze` consumiendo memoria automáticamente.
3. RAG, chunks y embeddings.
4. Comandos administrativos de repair/export avanzados.

---

## 18. Criterio de éxito

La propuesta se considera exitosa cuando un usuario puede:

1. Ejecutar `quorum init` en un proyecto con `memory/` local.
2. Obtener `.quorumrc` sin ruta de DB.
3. Migrar automáticamente las memorias locales a SQLite.
4. Ver que los archivos locales fueron eliminados solo después de una migración exitosa.
5. Repetir el proceso en otro proyecto y consultar ambos desde la misma DB.
6. Usar `QUORUM_MEMORY_DB` para correr pruebas contra una base temporal.
7. Guardar nuevas memorias con `quorum memory save` sin recrear `memory/` local.
8. Ejecutar escrituras concurrentes razonables sin errores inmediatos `database is locked`.
9. Eliminar o desregistrar datos sin dejar filas satélite huérfanas.
