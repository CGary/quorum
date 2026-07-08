# 09 — `quorum analyze contract-check`: gate determinista de `touch`/`forbid`

**Tipo:** función pura en core + shim CLI (patrón `analyze`).
**Depende de:** 01 (G0). Independiente de todo lo demás — puede implementarse en cualquier momento temprano.
**Riesgo sugerido:** low.

## Contexto

La constitución #3 ("no patches outside the contract") hoy se cumple por disciplina: `q-implement` instruye revisar `git diff --name-only`, y `q-review`/`q-accept` miran después. No existe un checker determinista centralizado. Con delegados externos que no conocen Quorum, la disciplina no existe: el gate tiene que ser código. Además sirve a TODO el sistema, no solo a la flota — `q-review` y `q-accept` pueden invocarlo en vez de re-derivar la lógica en prompts.

## Objetivo

`quorum analyze contract-check` — stdin: `{contract_path, changed_files: [...], diff_stat: {files, insertions, deletions}}` → `{ok: bool, violations: [{type, detail}...]}`.

## Reglas (todas las del contrato vigente)

1. `changed_files ⊆ 02-contract.yaml.touch` (glob-aware, mismas semánticas de glob que `risk.go` usa para `sensitive_paths`).
2. `changed_files ∩ forbid.files = ∅`.
3. `diff lines ≤ limits.max_diff_lines` (si el contrato lo define).
4. `len(changed_files) ≤ limits.max_files_changed` (ídem).
5. `forbid.behaviors` queda FUERA (es semántico, no determinista; sigue siendo trabajo de `q-review`) — el output lo recuerda en un campo `not_checked: ["forbid.behaviors"]` para que nadie asuma cobertura total.

Consumidores: el dispatch (tarea 06) lo corre post-ejecución — violación ⇒ `attempt_failed` con evidencia estructurada; los skills `q-review`/`q-accept` pueden adoptarlo después (mejora separada, no incluida acá).

## Criterios de aceptación

- [ ] `internal/core/contract_check.go` + `cmd/analyze_contract_check.go` con las convenciones `analyze` (stdin JSON, función pura, cero mutación).
- [ ] Tests por tabla: dentro de touch / fuera de touch / glob en touch / archivo en forbid / límites excedidos exactos / contrato sin limits (reglas 3-4 se omiten sin error).
- [ ] Salida con violaciones accionables (qué archivo, qué regla, qué esperaba el contrato).
- [ ] Documentado en CLAUDE.md (Analyze CLI surface).
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- Semántica exacta de `touch` con archivos nuevos creados por el delegado (¿un archivo nuevo necesita estar listado en touch? — verificar la convención vigente del schema de contrato y de `q-implement`).
- ¿El checker recibe `changed_files` calculados por el caller (propuesta: sí, función pura) o corre git él mismo (acopla a un worktree)?
