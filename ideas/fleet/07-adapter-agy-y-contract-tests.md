# 07 — Adapter agy + contract tests de wrappers

**Tipo:** segundo adapter + harness de smoke tests para todos los transportes.
**Depende de:** 06 (motor de dispatch) y del inventario de 01 (comportamiento real de agy).
**Riesgo sugerido:** medium.

## Contexto

agy (Antigravity CLI) es el segundo transporte y el que aporta la celda barata (Gemini 3.5 Flash Low, GPT-OSS 120B) y la diversidad de familia para review. Pero su superficie headless es menos rica que la de codex: `--print` sin flag JSON visible, sin `-C` para cwd observado, `--print-timeout` propio. Además, los CLIs de vendor cambian flags cada mes: sin contract tests, *wrapper roto* se confunde con *cuota agotada* y el sistema se pudre en silencio.

## Objetivo

1. Adapter agy integrado al motor de la tarea 06.
2. Harness de **contract tests** (smoke) para TODOS los transportes registrados en `agents.yaml`, en dos niveles.

## Diseño propuesto

**Adapter agy** (ajustar con el inventario de Fase 0a):

- Invocación: `agy --print --model {model} --sandbox` con prompt según lo que 0a determine (¿`--print "{texto}"` admite stdin o solo argumento? si solo argumento, usar `prompt_file` y pasar `@file` si lo soporta, o el contenido vía la vía que el inventario valide — NUNCA interpolación de payload multi-KB sin comillas seguras: pasar como argv único vía exec directo, sin shell).
- cwd: proceso lanzado con `cwd=worktree` (no hay `-C`; verificar que agy opera sobre el cwd y no sobre un workspace global; `--add-dir` como refuerzo si hace falta).
- Timeout: coordinar `--print-timeout` (default 5m) con el timeout del wrapper — el del wrapper manda y debe ser mayor, para que el fallo limpio del CLI llegue antes que el kill.
- Salida: texto plano (sin JSON) → usage `none` o `estimated`; las notas/BLOCKED se extraen del texto con los delimitadores que el protocolo del bundle pide (tarea 05).
- Mapeo de modelos: nombres canónicos → strings exactos que `--model` acepta (inventario: `agy models` lista display names; confirmar el identificador real).

**Contract tests** (dos niveles, registrados en `agents.yaml.contract_test`):

1. **Local/estructural (gratis, corre en preflight de sesión y CI):** binario existe, `--help` parsea, los flags que el adapter usa siguen existiendo. No invoca modelo.
2. **Real/smoke (consume cuota, manual o pre-sesión crítica):** prompt trivial ("responde OK") por la vía completa del adapter; valida parseo de salida de punta a punta.

Fallo de contract test ⇒ transporte marcado `wrapper_broken` (NO quota): el router lo excluye y se alerta al humano. Clases de fallo separadas por construcción.

## Criterios de aceptación

- [ ] Una hija real implementada vía agy de punta a punta (bundle → dispatch → 04 → verify), con el modelo que la Fase 0a haya validado.
- [ ] Tests del adapter con fake binario (éxito/timeout/texto basura/sin diff) — sin cuota en CI.
- [ ] Contract tests nivel 1 corriendo en CI para codex y agy; nivel 2 documentado como comando manual (`quorum fleet smoke <agent>` propuesto).
- [ ] Romper artificialmente un flag (fake binario sin `--print`) produce `wrapper_broken`, no `reroute_quota`.
- [ ] `go test ./...` verde.

## Decisiones abiertas para el brief

- Si la Fase 0a concluyó que agy no fija cwd de forma fiable: ¿se difiere este adapter y la serie sigue solo con codex (+claude opcional)? Es la decisión gate de esta tarea.
- Cadencia del smoke real: ¿manual, pre-sesión, o cron? (pregunta abierta #4 de la v2).
- ¿`gpt-oss-120b` o `gemini-3.5-flash-low` como celda barata canónica para la Fase 0 (tarea 10)?
