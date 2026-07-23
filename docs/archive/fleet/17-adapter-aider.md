# 17 — Adapter aider: tercer transporte (editor-CLI, `quota_class: api`)

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** internal/core/fleet_adapter_aider.go + contract test aider.yaml.

**Tipo:** tercer adapter sobre el motor de la tarea 06, extensión del harness de contract tests de la tarea 07.
**Depende de:** 06 (motor de dispatch) y 07 (harness de contract tests, que este doc EXTIENDE, no reescribe). Precondición propia: un inventario manual estilo Fase 0a del comportamiento headless de aider.
**Riesgo sugerido:** medium (no es código nuevo de efectos —el wrapper de 06 ya los tiene—, pero es el PRIMER transporte `quota_class: api` y por tanto el primer consumidor real de `max_cost_per_call_usd` y el primero que maneja API keys de proveedor).

## Contexto

codex (suscripción) y agy (suscripción Antigravity) cubren la flota v1, pero ambos son agentes autónomos: exploran, razonan, deciden. aider es de otra especie — un **editor-CLI**: recibe archivos acotados, un mensaje y un backend, y aplica una edición localizada con mapa del repo en vez de exploración amplia (`ideas/aider.md` §2). Ese perfil calza exactamente con la celda barata de `q-implement` sobre tareas mecánicas ya diseñadas por un agente fuerte (`ideas/aider.md` §3.10): el modelo fuerte razona, aider barato ejecuta, tests y diff validan.

Dos diferencias estructurales frente a codex/agy obligan a tratarlo aparte, no como "un modelo más":

1. **El backend es configurable (litellm).** aider no ES un modelo; es una herramienta que habla con el modelo que se le configure. Por eso la metadata de familia/proveedor NO puede colgar del transporte `aider` — cuelga del **modelo** que aider corre en cada dispatch. Un `aider` con Flash barato y un `aider` con DeepSeek son transportes distintos a efectos de familia y `provider_trust`.
2. **aider auto-commitea por defecto.** Escribe commits en la rama al terminar cada edición. Eso choca de frente con el invariante forense del wrapper (tarea 06 §4): la rama de tarea NUNCA debe quedar apuntando al intento; el forense vive en `refs/fleet/<task>/attempt-N-<model>` y el worktree se resetea a baseline. Un auto-commit sin neutralizar rompe "commit forense ≠ baseline limpio" y contamina la rama.

El alcance es **`q-implement` y solo `q-implement`**. La revisión final queda explícitamente fuera: aider es una herramienta de edición, y `ideas/aider.md` §4.2 reserva review/diagnóstico complejo/revisión final para el modelo fuerte. Meter aider en review sería confundir al ejecutor con el juez.

## Objetivo

1. Adapter aider integrado al motor de la tarea 06 como **tercer transporte**, declarado en `agents.yaml` con `quota_class: api`.
2. Extensión del harness de contract tests de la tarea 07 para cubrir aider en sus dos niveles (estructural y smoke), sin reescribir el harness.
3. Primer camino real de `max_cost_per_call_usd` (tarea 02: "hoy: ninguno") y de manejo de API keys de proveedor, sin resucitar presupuestos por ventana.

## Diseño propuesto

**Inventario manual (precondición, estilo Fase 0a) — a resolver ANTES del blueprint:**

- Flags headless que evitan cualquier prompt interactivo: `--yes` / `--yes-always` (confirmar cuál aplica a qué y si basta para un run 100% no interactivo).
- Códigos de salida: qué exit code produce éxito, fallo de edición, error de backend/API (para mapear a la taxonomía de la tarea 03).
- Fijación de cwd/worktree: verificar que aider opera sobre el cwd del proceso y no sobre un workspace global; el wrapper lanza con `cwd=worktree` (nunca la raíz).
- Comportamiento de auto-commit: confirmar el flag exacto para desactivarlo (`--no-auto-commits`) y/o el patrón de squash/reset alternativo.

**Adapter aider (ajustar con el inventario):**

- **Entrada:** `input: prompt_file` en `agents.yaml`. El wrapper pasa el bundle (tarea 05) vía `--message-file {prompt_file}` y los archivos permitidos como **argumentos posicionales** derivados de `02-contract.yaml.touch`. Nada de interpolación shell del payload: argv directo por exec, sin shell (misma disciplina que agy en la tarea 07).
- **Plantilla desde el contrato (mapeo 1:1, `ideas/aider.md` §6):** el wrapper renderiza el mensaje de aider desde el contrato — `Allowed files` = `02-contract.touch`; `Do not edit` = `02-contract.forbid`; `Constraints`/`Acceptance`/`Validation` del bloque de protocolo del bundle. El bundler (tarea 05) ya produce este contenido; el adapter solo lo materializa en el formato que aider espera.
- **Neutralización de auto-commits (invariante duro):** el wrapper corre aider con `--no-auto-commits` (o, si el inventario lo exige, squash/reset del commit que aider deje). El resto del invariante forense es idéntico a codex/agy: si hubo diff → snapshot como `refs/fleet/<task>/attempt-N-<model>` y `reset --hard` a baseline según la regla de la tarea 06; si no hubo diff → reset directo. aider NO administra la rama; el wrapper sí.
- **cwd:** proceso lanzado con `cwd=worktree`; verificar contra el inventario que aider no reabre un repo distinto.
- **Timeout:** el timeout del wrapper manda (kill del process group completo, tarea 06 §1). Coordinar con cualquier timeout propio de aider si existe — el del wrapper debe ser mayor para que el fallo limpio llegue antes que el kill.
- **Salida y usage:** aider emite texto/diff, no JSONL estructurado de eventos → usage `none` o `estimated`, **jamás inventado** (mismo precedente que el adapter agy, tarea 07). Las notas/señal BLOCKED se extraen del texto con los delimitadores que el protocolo del bundle define (tarea 05).
- **Metadata de familia POR MODELO:** en `agents.yaml`, la familia/proveedor se declara en cada entrada de modelo que aider expone (nombre canónico → string litellm), no en el transporte. El join contra `config.yaml.levels` (tarea 02) resuelve por modelo, como el resto.
- **Firmas de fallo:** firmas de quota/auth del proveedor detectadas en salida → outcome de reroute + evento `quota_red` hacia el kill-switch (tarea 11), exactamente como codex/agy.

**Outcomes — SIN clases nuevas.** Todo mapea a la taxonomía existente de la tarea 03: diff no vacío → `attempt` (consume `max_attempts` del contrato); firma de quota/timeout/crash con diff vacío → `reroute` (consume `reroute_budget` del dispatch); salida imparseable por cambio de flags → `reroute` + alerta `wrapper_broken`; señal BLOCKED → `blocked` (no castiga a nadie). aider NO introduce una categoría "editor" ni un outcome propio.

**Contract tests (extensión de la tarea 07, mismos dos niveles):**

1. **Estructural (gratis, preflight/CI):** binario existe, `--help` parsea, los flags que el adapter usa (`--message-file`, `--no-auto-commits`, `--model`, `--yes*`) siguen existiendo. No invoca modelo ni consume cuota.
2. **Smoke (consume cuota API, manual):** prompt trivial por la vía completa del adapter contra el backend configurado; valida el mapeo contrato→plantilla, la neutralización de auto-commits y el parseo de salida de punta a punta. Documentado como `quorum fleet smoke aider` (mismo verbo propuesto en la tarea 07).

**Control de costo y kill-switch.** Sin presupuestos por ventana ni contadores de requests (prohibido por el ítem 10 del horizonte, tarea 16, salvo evidencia de dolor real): el único control por llamada es `max_cost_per_call_usd` sobre la `quota_class: api`, más el kill-switch manual + rojo reactivo (429) de la tarea 11. Este adapter es el primer lugar donde `max_cost_per_call_usd` deja de ser letra muerta.

## Criterios de aceptación

- [ ] `agents.yaml` declara `aider` con `input: prompt_file`, `quota_class: api`, plantilla argv con archivos posicionales, y familia/proveedor declarados POR MODELO (no en el transporte); validado por el schema de la tarea 02.
- [ ] Una hija real implementada vía aider de punta a punta (bundle → dispatch → 04 → verify) con el backend barato que el inventario haya validado, restringida a un caso mecánico del catálogo de `ideas/aider.md` §3.
- [ ] El wrapper neutraliza el auto-commit: tras un dispatch con diff, la rama de tarea sigue en baseline y el forense vive en `refs/fleet/<task>/attempt-N-<model>` (test que falla si aider deja un commit en la rama).
- [ ] Tests del adapter con fake binario (éxito/timeout/texto basura/sin diff/exit de error de API) — sin cuota real en CI.
- [ ] Contract test nivel 1 de aider corriendo en CI; nivel 2 documentado como `quorum fleet smoke aider` manual.
- [ ] Romper artificialmente un flag (fake binario sin `--message-file`) produce `wrapper_broken`, no `reroute_quota`.
- [ ] Los outcomes de aider caen en las clases existentes de la tarea 03; cero clases nuevas en la taxonomía.
- [ ] `max_cost_per_call_usd` se lee y aplica para la `quota_class: api` (test del corte por costo por llamada); ningún presupuesto por ventana reintroducido.
- [ ] Ninguna API key aparece en `agents.yaml` ni en artefactos versionados (test/lint que lo verifique).
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- **Backend por defecto de aider y su `provider_trust`.** ¿Qué modelo litellm corre aider como celda barata (Flash / DeepSeek / Llama / local)? Si el elegido es `external_low`, se ACTIVA la condición del ítem 3 del horizonte (tarea 16: `data_classification` + `provider_trust` + data-gate en el bundler) y este adapter deja de ser puramente aditivo — hay que decidir si se difiere hasta que ese gate exista o si el backend v1 se restringe a `external_standard`.
- **¿aider entra en la muestra de medición del gate G1 (Fase 0, tarea 10) o después?** Sumarlo como tercera celda enriquece el dato de "barato + reintentos vs mid directo", pero agrega una variable (editor-CLI vs agente autónomo) que puede confundir la lectura; alternativa: aider entra tras G1, ya con las bandas calibradas.
- **`--auto-lint` / `--test-cmd` DENTRO de aider: ¿pre-check advisory o se omiten?** Correrlos dentro de aider (`ideas/aider.md` §7) puede subir el pass@1 del intento, pero introduce una validación paralela a `q-verify`. La Regla #4 dice que la validación es finalidad y `q-verify` es la única fuente de verdad. Propuesta a ratificar: omitirlos (una sola fuente de verdad) o admitirlos solo como pre-check advisory cuyo resultado NUNCA sustituye a `q-verify`.
- **Valor concreto de `max_cost_per_call_usd`** para la `quota_class: api` (primer valor real del campo; hoy "ninguno" en la tarea 02).
- **Manejo de API keys:** por variables de entorno del proceso, NUNCA en `agents.yaml` (riesgo de secreto versionado). Definir el nombre de las env vars por proveedor y el comportamiento del preflight si faltan (¿transporte inválido ruidoso, como el join de la tarea 02?).
- **Granularidad del kill-switch: `aider` vs `aider/<model>`.** Como el backend es configurable, apagar "aider" apaga todos sus modelos; apagar `aider/<model>` permite bajar solo el backend que dio 429. ¿El kill-switch (tarea 11) opera a nivel transporte o transporte×modelo para este adapter?
