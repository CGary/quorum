# Serie FLEET — Dispatcher multi-LLM de Quorum (índice)

**Fecha:** 2026-06-12
**Origen:** `ideas/0-sistema-inteligente-enrutamiento-tareas-entre-múltiples-llms.md` (v2) + análisis (`*.analysis*.md`) + decisiones del humano.
**Uso:** cada archivo `NN-*.md` de esta carpeta es el insumo de UNA tarea Quorum (`/q-brief` la entrevista, `/q-blueprint` la estrategia). Las "Decisiones abiertas" de cada doc son las preguntas que el brief debe cerrar.

## Decisiones ya tomadas (no re-litigar en los briefs)

1. **El código entra directo al core** (`internal/core` + `cmd/`), con los ADRs como primeras tareas. Sin repo aparte, sin módulo intermedio. El skill es interfaz fina; la lógica determinista es código Go.
2. **Semáforo manual + rojo reactivo.** No se calcula cuota de proveedores (denominador desconocido). El humano monitorea usage externamente y apaga modelos vía kill-switch; un 429 apaga automático. Telemetría laxa: se registra lo que el CLI reporte, sin gobernador de presupuesto.
3. **Human-in-the-loop en ambigüedad.** Nada de loops ni heurísticas que quemen tokens: señal BLOCKED con pregunta de contexto rico (intentos, evidencia, opciones con consecuencias, recomendación y SIEMPRE opción abierta). Canal v1: CLI. La línea de cuándo preguntar es el contrato: ambigüedad dentro del contrato y reversible → decidir y registrar en trace; sobre el contrato o irreversible → preguntar.
4. **Flota real hoy (verificada en máquina 2026-06-12):** `codex` (suscripción ChatGPT; `codex exec` headless con `--json`, `-o`, `--output-schema`, `-C`, `--sandbox`, stdin nativo) y `agy` (Antigravity CLI de Google; `--print`, `--model`, `--sandbox`, `--print-timeout`; modelos: Gemini 3.5 Flash M/H/L, Gemini 3.1 Pro L/H, Claude Sonnet 4.6 T, Claude Opus 4.6 T, GPT-OSS 120B M). La celda "barata" y la diversidad de familia existen vía agy. `claude -p` queda como transporte opcional. opencode/minimax/deepseek/qwen: sin transporte hoy.
5. **Dashboard final mínimo:** `quorum serve start` con status de flota + kill-switch. Sin responder BLOCKED desde web ni visor q-report (futuro, ver 16).
6. **Todos los docs se redactan ahora**; las tareas posteriores a la medición llevan gate explícito de ejecución.

## Lemas (heredados de la v2, vigentes)

Claude propone, la política dispone · Transporte ≠ política · Trace es la verdad, el ledger es índice · Compliance by construction · Telemetría antes que automatización · El humano lidera.

## DAG de tareas y gates

```
01 Fase 0a (manual, sin código) ──► GATE G0: ¿delegar funciona? si NO → se archiva la serie
   │
02 ADR-A transporte/política ── 03 ADR-B attempt/reroute/blocked + trace
   │
04 complexity-score   05 bundler   09 contract-checker     (independientes entre sí)
   │                      │             │
06 dispatch engine + adapter codex ─────┘
07 adapter agy + contract tests
   │
17 adapter aider (mismo harness que 07, quota_class: api) — Depende de: 06, 07
08 fabricador del 04
   │
10 Fase 0 medición instrumentada ──► GATE G1: pass@1 de celdas barato/mid → rol de L0 y bandas
   │
11 kill-switch + semáforo reactivo      12 protocolo BLOCKED pregunta rica
   │                                        │
13 router `fleet route` (gate G1) ──────────┘
   │
14 skill /q-dispatch
   │
15 quorum serve: dashboard status + kill-switch
   │
16 horizonte (gated, NO implementar sin sus condiciones)
```

Orden recomendado de ejecución: 01 → (02,03) → (04,05,09) → 06 → (07,08) → 17 (opcional antes de 10 si aider entra en la muestra de medición — decisión del brief de 17) → 10 → (11,12) → 13 → 14 → 15.

## Convenciones para todos los blueprints

- Funciones puras de análisis siguen el patrón existente: `internal/core/*.go` + shim `cmd/analyze_*.go`, request JSON por stdin (ver `internal/core/risk.go` como molde).
- Comandos nuevos de flota bajo `quorum fleet <verbo>`; nunca un runtime auto-encadenador (existe test que lo prohíbe: `internal/core/task_manager_test.go:735`).
- Artefactos persistidos en inglés conciso; salida de skills en español; `go test ./...` verde con `CGO_ENABLED=0` siempre (nada de dependencias pesadas, tampoco en el server HTTP).
- Telemetría a `07-trace.json` desde el primer dispatch (el schema ya soporta `model`, `tokens_in/out`, `cost_usd`, `events[]`).
