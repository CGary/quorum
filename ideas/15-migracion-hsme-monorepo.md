# 15 — Migración de HSME al monorepo Quorum (relocación pura, ambos funcionales)

> **Alcance:** mover el repositorio HSME (`CGary/mcp-semantic-memory`, ~310 commits) dentro de
> este repositorio bajo `semantic/`, preservando TODA la historia git, **sin mover ni cambiar
> funcionalidad**. Al terminar, ambos proyectos compilan, se instalan y operan exactamente igual
> que hoy. La integración funcional (importador, advisor de dedup) es trabajo POSTERIOR y está
> definida en `docs/adr/0008-fusion-monorepo-capa-semantica-hsme.md` — NO forma parte de esta
> migración.
>
> Fecha: 2026-06-09. Hechos verificados contra ambos repos en esa fecha.

---

## 0. Hechos verificados que condicionan la migración

| Hecho | Evidencia | Implicación |
|---|---|---|
| Quorum es Go puro, sin CGO | `go.mod` raíz: `modernc.org/sqlite` | El build de Quorum NO cambia en nada |
| HSME exige CGO + build tags | `semantic/justfile`: `CGO_ENABLED=1`, tags `sqlite_fts5 sqlite_vec` | HSME se compila SIEMPRE desde su justfile, nunca desde la raíz |
| Los módulos no se importan entre sí | Frontera de compilación (ADR 0008) | El merge es de árboles disjuntos: cero conflictos esperados |
| La DB de HSME está gitignoreada | `.gitignore` de HSME: `data/`, `*.db` ("PROTECCIÓN TOTAL") | **git NO mueve `data/engram.db`** — copia manual obligatoria |
| La DB se resuelve relativa al cwd | `justfile`: `SQLITE_DB_PATH := invocation_directory() + "/data/engram.db"` | Tras mover el repo, los recetas `just` buscan la DB en la RUTA NUEVA |
| El MCP usa rutas absolutas al repo viejo | `~/.claude.json:2114-2117`: command `/home/gary/go/bin/hsme`, env `SQLITE_DB_PATH=/home/gary/dev/hsme/data/engram.db` | El binario instalado sobrevive; el env var hay que actualizarlo |
| Module path actual: `github.com/hsme/core` | `go.mod` de HSME | Funciona sin renombrar; el rename es opcional y separado (§6) |
| go raíz 1.26.3, go HSME 1.26.2 | ambos `go.mod` | `go.work` válido (toma el mayor) |

---

## 1. Precondiciones (no empezar sin esto)

```bash
# 1.1 Backup atómico de la DB de HSME (compatible WAL)
cd /home/gary/dev/hsme && just backup
# o a mano: cp data/engram.db data/engram.db.pre-monorepo.bak

# 1.2 Ambos working trees LIMPIOS y pusheados
cd /home/gary/dev/hsme   && git status --short   # vacío
cd /home/gary/dev/quorum && git status --short   # vacío

# 1.3 Parar procesos de HSME (worker/ops escriben en la DB)
cd /home/gary/dev/hsme && just stop-all
```

Además: cerrar las sesiones de Claude Code que tengan el MCP `hsme` activo (el server stdio
mantiene la DB abierta).

---

## 2. Fase A — Commit de relocación en el repo HSME

El truco que preserva la historia navegable: mover TODO a `semantic/` **dentro del repo HSME,
antes del merge**. Así `git log semantic/...` funciona en el monorepo sin `--follow`.

```bash
cd /home/gary/dev/hsme
git switch -c move-to-monorepo
mkdir semantic

# Mover todo lo TRACKEADO (incluye dotfiles como .gitignore) excepto .git y semantic/
git ls-tree -z --name-only HEAD | xargs -0 -I{} git mv {} semantic/

git commit -m "chore: relocate tree under semantic/ for quorum monorepo merge"
```

> ⚠️ Usar `git ls-tree`, NO `ls -A`: mueve exactamente lo versionado (incluidos `.gitignore`,
> `.ai/`, etc.) y no arrastra `data/`, `backups/`, `logs/` ni binarios sueltos (ignorados).
> Esos quedan huérfanos en el repo viejo y se copian a mano en la Fase D.

Verificación: `git show --stat HEAD | head` debe listar solo renames `X → semantic/X`.

---

## 3. Fase B — Merge de historias en Quorum (en rama, nunca en main)

La constitución dice "el sistema commitea, nunca mergea a main": todo esto ocurre en una rama
de trabajo; el merge final a `main` lo hace el humano.

```bash
cd /home/gary/dev/quorum
git switch -c merge-hsme

git remote add hsme /home/gary/dev/hsme
git fetch hsme
git merge --allow-unrelated-histories hsme/move-to-monorepo \
  -m "feat: merge HSME history as semantic/ module (ADR 0008)"

git remote remove hsme
```

No se esperan conflictos: los árboles son disjuntos (todo HSME vive bajo `semantic/`). Si
apareciera alguno, es señal de un archivo en la raíz de HSME que colisiona — resolver
conservando la versión de Quorum en la raíz y la de HSME bajo `semantic/`.

Verificaciones inmediatas:

```bash
git log --oneline | wc -l                       # ≈ 139 + 310 + 2 (relocate + merge)
git log --oneline -5 -- semantic/justfile        # historia de HSME visible
ls semantic/                                     # cmd/ src/ tests/ justfile README.md ...
```

---

## 4. Fase C — Workspace Go y housekeeping del repo

### 4.1 `go.work` (solo conveniencia de desarrollo)

```bash
cd /home/gary/dev/quorum
go work init . ./semantic
git add go.work && git commit -m "chore: add go.work for two-module workspace"
```

Notas:
- Con `go.work`, `go build`/`go test ./...` desde la raíz siguen operando SOLO sobre el módulo
  raíz (los `./...` son por módulo). Nada del módulo `semantic` entra al build del core.
- `go.work.sum` puede aparecer; commitearlo también.
- Para builds de release del core donde se quiera aislamiento total: `GOWORK=off go build`.

### 4.2 Gitignore

El `.gitignore` de HSME viajó a `semantic/.gitignore` y git aplica gitignores anidados a su
subárbol — **no hay que duplicar reglas en la raíz**. Comprobar:

```bash
touch semantic/data/test.db 2>/dev/null; git status --short | grep -c data  # 0
```

Único añadido recomendable al `.gitignore` raíz (defensa en profundidad, opcional):

```
semantic/data/
semantic/backups/
semantic/logs/
```

### 4.3 Lo que NO se hace en esta migración

- NO renombrar el module path `github.com/hsme/core` (funciona tal cual; ver §6).
- NO reestructurar `src/` → `pkg/`.
- NO tocar `cmd/`, esquemas, skills ni nada del core de Quorum.
- NO construir el importador ni el advisor (fases 2-3 del ADR 0008).

---

## 5. Fase D — Migrar los DATOS y la config MCP (lo que git no mueve)

**Este es el paso que rompe HSME si se olvida.** La DB, backups y logs están gitignoreados.

```bash
# 5.1 Copiar datos operativos al nuevo hogar
cp -r /home/gary/dev/hsme/data     /home/gary/dev/quorum/semantic/data
cp -r /home/gary/dev/hsme/backups  /home/gary/dev/quorum/semantic/backups  # si existe
cp -r /home/gary/dev/hsme/logs     /home/gary/dev/quorum/semantic/logs     # si existe
```

```bash
# 5.2 Actualizar la config MCP (~/.claude.json, entrada del server hsme)
#     ANTES: "SQLITE_DB_PATH": "/home/gary/dev/hsme/data/engram.db"
#     AHORA: "SQLITE_DB_PATH": "/home/gary/dev/quorum/semantic/data/engram.db"
```

El `command` (`/home/gary/go/bin/hsme`) NO cambia: apunta al binario instalado, no al repo.

> ⚠️ Si se arranca el MCP con el env var viejo apuntando a una ruta inexistente, HSME puede
> crear una DB VACÍA nueva y parecerá que "se perdieron" las memorias. No es pérdida: es la
> ruta. Por eso la config se actualiza en el mismo momento que la copia.

---

## 6. Fase E — Reinstalar y verificar AMBOS proyectos

### 6.1 Quorum (debe ser indistinguible de hoy)

```bash
cd /home/gary/dev/quorum
go test ./...                            # verde, sin CGO, sin tocar semantic/
go build -o quorum . && mv quorum ~/go/bin/   # (o la ruta de PATH habitual)
quorum task list && quorum memory status      # humo
```

Prueba ácida del ADR 0008 (la frontera de compilación):

```bash
CGO_ENABLED=0 go build -o /tmp/quorum-acid . && echo "CORE INDEPENDIENTE ✅"
```

### 6.2 HSME (mismo flujo de siempre, nueva ubicación)

```bash
cd /home/gary/dev/quorum/semantic
just test          # go test -tags "sqlite_fts5 sqlite_vec" ./...
just install       # instala hsme, hsme-worker, hsme-ops, hsme-cli en ~/go/bin
just work-bg       # relanzar el worker (embeddings async)
just ops-bg        # relanzar observabilidad (si se usa)
```

Reiniciar la sesión de Claude Code (relanza el MCP server con el env var nuevo) y verificar
continuidad de datos: una búsqueda `mcp__hsme__search_fuzzy` debe devolver memorias ANTIGUAS
(pre-migración). Si devuelve corpus vacío → revisar §5.2.

### 6.3 Checklist final

- [ ] `go test ./...` verde en la raíz sin compilador C disponible
- [ ] `just test` verde en `semantic/`
- [ ] `quorum` reinstalado responde (`task list`, `memory status`)
- [ ] Binarios `hsme*` reinstalados desde `semantic/` responden
- [ ] `search_fuzzy` vía MCP devuelve memorias pre-migración (continuidad de `engram.db`)
- [ ] `git log semantic/<archivo>` muestra historia HSME completa
- [ ] `git status` limpio (nada de `data/`/`logs/` colándose)
- [ ] Worker y ops corriendo si se usan (`just start-all`)

---

## 7. Fase F — Cierre del repo viejo (solo tras checklist verde)

1. En `CGary/mcp-semantic-memory`: último commit en `main` dejando un README que apunte a
   `CGary/quorum` (sección `semantic/`).
2. Archivar: `gh repo archive CGary/mcp-semantic-memory` (queda read-only; los enlaces siguen
   resolviendo).
3. Actualizar la descripción de `CGary/quorum` para mencionar la capa de memoria semántica.
4. Conservar `/home/gary/dev/hsme` local unas semanas como red de seguridad de los datos
   no versionados; borrarlo solo cuando la DB nueva tenga backups propios (`just backup`).

---

## 8. Rollback

Todo es reversible hasta el merge a `main`:

- La rama `merge-hsme` de Quorum se descarta con `git branch -D merge-hsme` — `main` intacto.
- El repo HSME viejo no se toca (la relocación vive en su rama `move-to-monorepo`).
- La DB original sigue en `/home/gary/dev/hsme/data/` + backup de §1.1.
- Si el MCP ya se apuntó a la ruta nueva, revertir el env var en `~/.claude.json`.

---

## 9. Trabajo posterior (fuera de alcance, en orden — ADR 0008 §6)

1. Rename del module path (`github.com/hsme/core` → p. ej. `quorum/semantic`) + opcional
   `src/` → `pkg/`. Mecánico: sed de imports + `just test`.
2. Importador delta semantic←Quorum (`mode=ro` sobre `~/.quorum/memory.db`).
3. Advisor de duplicados semánticos en `q-memory`/`q-session` vía `search_fuzzy`
   (solo después del importador; sin corpus es ciego).
4. CI en dos carriles (core sin CGO en cada PR; semantic con CGO + Ollama mockeado).
5. Renumerar la colisión de ADRs 0007 (dos archivos comparten número).
