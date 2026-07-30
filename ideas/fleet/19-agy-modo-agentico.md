# 19 — Desbloquear el modo agéntico de `agy` como segundo transporte editor

**Fecha:** 2026-07-30
**Tipo:** insumo de tarea Quorum (pasar por `/q-brief` → `/q-blueprint`). Gated: leer "Decisiones abiertas" antes de implementar.
**Origen:** hallazgo operando `fleet-delegate` (skill de Claude Code). Un usuario cuestionó por qué el skill trata a `agy` como incapaz de editar archivos. La verificación contra la config real confirmó que es una limitación de **configuración del fleet**, no del binario.

## Hallazgo (verificado 2026-07-30)

Hoy el fleet tiene **un solo transporte agéntico editor**: `opencode` (y `aider`, `quota_class: api`). `agy` está cableado como **one-shot de texto** (`--print`), no como editor. Consecuencia práctica: cualquier tarea que edite archivos SOLO puede ir a opencode/aider; si esos se saturan (429) o fallan, no hay fallback agéntico sobre la suscripción Gemini — agy solo sirve para análisis/review one-shot (nivel 1 de verificación).

Pero el binario `agy` SÍ soporta un loop agéntico con herramientas de escritura. El fleet, deliberadamente, no lo activa.

## Evidencia

Fuente: `/home/gary/dev/quorum/.agents/fleet/agents.yaml` (bloque `agy`) + `agy --help`.

- **`agents.yaml:136-142`** — `argv_template` de agy fija: `--model {model_arg} --print-timeout {print_timeout} --print {prompt}`. Usa **`--print`** = modo "run a single prompt non-interactively and print the response". One-shot, no interactivo, sin edición.
- **`agents.yaml:148`** — `output_format: text`. La salida es texto plano al wrapper, no un diff de archivos.
- **`agy --help`** expone flags que el fleet NO usa: `--mode accept-edits`, `--dangerously-skip-permissions`, `--sandbox`, `--add-dir`. Esos habilitan el loop agéntico con escritura.
- Contraste en `docs/fleet-run-for-agents.md`: describe a `opencode`/`aider` explícitamente como **"agentic / implement-only edits"**; a `agy` nunca — solo "broader use, higher-effort models". El diseño ya distingue las dos clases.

**Conclusión:** "agy no edita" es cierto para el fleet tal como está, y falso para el binario. La brecha es de config.

## La decisión a resolver

¿Se agrega una **variante agéntica de agy** al fleet (transporte o modo de transporte que use `--mode accept-edits` + scope de directorio), manteniendo la variante `--print` actual para análisis/review?

No es reemplazar: el modo `--print` sigue siendo el correcto para el nivel-1 de verificación (revisar diff) y para generación one-shot. Sería **añadir** una capacidad de edición, no migrar.

## Tradeoffs

**A favor:**
- **Redundancia agéntica.** Hoy si opencode se satura, no hay fallback editor sobre suscripción (agy es solo texto). agy agéntico daría un segundo editor de familia distinta (Gemini/Claude vía Antigravity) — diversidad de familia que el fleet ya valora (decisión #4 del índice).
- **Aprovecha modelos de mayor capacidad** que agy ya expone (Gemini 3.1 Pro, Claude Sonnet/Opus 4.6) para ediciones MEDIUM que hoy caen directo a interno.

**En contra:**
- **`--dangerously-skip-permissions` es exactamente el riesgo que el diseño one-shot evita.** Un agente externo editando el repo con permisos saltados deja como única red el hard-gate de `git status --porcelain` limpio + revert. Hay que evaluar si `--sandbox` de agy es suficiente contención sin saltar permisos.
- **Soberanía de datos (Regla #10, no se pisa).** Un editor agéntico manda más contexto del repo al proveedor que un `--print` acotado. Encaja con el diferido de `provider_trust` + data-gate (ver `16-horizonte-gated.md`, ítems 3 y 12): si agy-agéntico se clasifica `external_standard` como hoy, no discrimina nada; si en el futuro entra un backend `external_low`, se activa la condición del data-gate.
- **Costo de suscripción vs USD 0.** opencode (OpenRouter free) sigue siendo peldaño 1. agy-agéntico sería peldaño ~3-4: solo cuando opencode falla/satura o la tarea pide un modelo que opencode no tiene. NO debe volverse default y pisar el "cheapest capable wins".

## Encaje con el diseño existente

- El slot de transporte ya existe (`agents.yaml` tiene `quota_class`, `argv_template`, `output_format` por agente). Añadir una entrada `agy-edit` (o un flag de modo) es aditivo, no un rediseño.
- Contract tests: la tarea 07 (`adapter agy + contract tests`, archivada) ya dejó el harness de contrato para agy `--print`. Una variante agéntica necesita su propio contract test (el `output_format` pasa de `text` a diff — se compara contra el harness de opencode/aider, no el de agy-texto).
- Invariante a respetar: nada de runtime auto-encadenador (`task_manager_test.go:735`). Esto es un transporte más, invocado por fase; no toca esa prohibición.
- Relación con `16-horizonte-gated.md` ítem 12 (transporte opencode/API): es distinto. Ese ítem es transporte API directo a un modelo nombrado. Éste es desbloquear un modo YA presente en un transporte YA integrado. No requiere API keys nuevas.

## Decisiones abiertas (las cierra el `/q-brief`)

1. **¿Modo o transporte?** ¿Se agrega un agente `agy-edit` separado en `agents.yaml`, o un parámetro de modo sobre el `agy` existente que intercambie `--print` ↔ `--mode accept-edits` según la fase? (Impacta el router y `fleet route`.)
2. **Contención:** ¿`--sandbox` sin `--dangerously-skip-permissions`, o el modo agéntico de agy exige saltar permisos para editar? Verificar contra `agy --help` en máquina antes de decidir. Si exige saltar permisos, ¿el hard-gate git + revert es red suficiente, o se rechaza?
3. **Scope de directorio:** el argv de agy hoy NO pasa `-C/--cd` (el wrapper lanza con `cwd=worktree`). El modo edición ¿necesita `--add-dir` explícito para acotar el blast radius? ¿A qué se le da acceso?
4. **Posición en la escalera:** ¿peldaño 3-4 (solo cuando opencode satura o falta el modelo), o se le da un rol propio en `fleet route`? No debe pisar a opencode como peldaño 1.
5. **Clasificación de confianza:** ¿la variante editora sigue `external_standard`, o fuerza reconsiderar el data-gate diferido (ítem 3 del horizonte)? Editar manda más contexto que `--print`.
6. **Modelo por defecto de la celda editora:** ¿Gemini Flash (barato, calidad de tabs dudosa como se observó en opencode free), o directo Pro/Sonnet para ediciones que valen la suscripción?

## Nota de actualización aguas abajo (fuera de quorum)

Independiente de esta tarea: el `SKILL.md` de `fleet-delegate` (repo `claude-skills`) dice *"agy is `--print` one-shot text (NO agentic file editing)"*. Esa frase es correcta HOY. Si esta tarea se implementa y agy gana modo editor, el wording del skill debe corregirse para reflejar que agy pasa a ser transporte editor. No tocar el skill antes de que esta tarea aterrice.
