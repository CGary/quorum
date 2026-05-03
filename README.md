# Quorum ⚖️

**Constraints in. Verified diffs out. Costs bounded. Humans only where humans matter.**

Quorum es un framework de orquestación AI-first para el desarrollo de funcionalidades complejas. Trata a los agentes de IA como unidades de ejecución quirúrgica que operan a partir de artefactos de planificación en YAML (legibles por humanos y máquinas) y artefactos de captura del sistema en JSON.

## 🧠 Filosofía

- **Machine-First, Audience-Aware**: YAML para planificación (`spec`, `blueprint`, `contract`), JSON para captura de salida del sistema (`validation`, `review`, `trace`).
- **Spec-Driven Contracts (SDC)**: El flujo lógico va de Spec → Blueprint → Contract → Verified Diff.
- **Ejecución Aislada**: Cada tarea se ejecuta en un Git worktree dedicado.
- **Autoridad Humana de Merge**: El sistema puede realizar commits, pero el merge a `main` es una acción exclusivamente humana.

### 🚫 Lo que Quorum NO es

- **Una herramienta de triaje para cambios triviales**: Quorum está diseñado para funcionalidades complejas donde la estructura aporta valor. Bugfixes, typos y ediciones de 5 líneas están fuera del alcance. Usa herramientas de CLI directas para esos casos.

## 📜 Constitución (Reglas Inmutables)

1. **Git es la Verdad del Código**: La memoria semántica es para patrones; Git es para el código.
2. **Contexto Determinista**: Los agentes nunca reciben "todo el proyecto". Reciben el `context_bundle` derivado del Blueprint.
3. **Sin Parches fuera del Contrato**: Si un agente toca un archivo no incluido en la lista `touch`, el cambio es rechazado.
4. **La Validación es la Finalidad**: Una tarea NO está terminada hasta que `verify.commands` devuelva 0.
5. **Artefactos Machine-First, Formato según Audiencia**: Todos los archivos de planificación son YAML o JSON. Markdown solo se permite en `/docs/adr/` y documentación externa.
6. **El Sistema Commitea, Nunca Mergea**: Los agentes hacen commit en ramas de funcionalidad en worktrees aislados. El merge es humano.
7. **El Costo está Limitado por Política, no por Confianza**: El dispatcher decide el enrutamiento y escalado, nunca el agente.
8. **Los Tests son la única Prueba de Trabajo**: Solo los `verify.commands` prueban la funcionalidad.

## 🚀 Flujo Operativo

1. **Specify**: `agents task specify <ID>` crea `.ai/tasks/inbox/<ID>/00-spec.yaml`.
2. **Blueprint**: El **Surgical Cartographer** genera el plan de ataque en `01-blueprint.yaml` y el contrato en `02-contract.yaml`.
3. **Execute**: `agents task start <ID>` crea el worktree aislado.
4. **Run / Verify**: El agente ejecuta los cambios siguiendo el contrato y valida con `verify.commands`.
5. **Review**: Los resultados de la revisión se capturan en `06-review.json`.
6. **Merge Gate**: El humano ejecuta la suite de aceptación BDD antes de aprobar el merge.

## 🧪 Política de Testing

`verify.commands` (Bucle rápido del agente): Unit tests y lint, objetivo <60s.
`acceptance.bdd_suite` (Puerta de merge humana): Suite de aceptación lenta, objetivo <10min.

Los agentes nunca esperan al BDD; los humanos nunca aprueban sin el BDD.

## 📂 Estructura del Sistema

```bash
project/
├── .agents/
│   ├── schemas/         # JSON Schemas para validar artefactos YAML y JSON
│   ├── prompts/         # Instrucciones de sistema por rol
│   ├── policies/        # Lógica de riesgo y enrutamiento
│   └── config.yaml      # Asignación de modelos y límites de costo
├── .ai/tasks/
│   ├── inbox/           # Specs y blueprints en borrador
│   ├── active/          # Artefactos de tareas en ejecución (00-07)
│   ├── done/            # Tareas completadas y archivadas
│   └── failed/          # Tareas que fallaron el contrato
├── docs/adr/            # Decisiones arquitectónicas (Markdown permitido)
├── memory/              # Aprendizaje semántico selectivo
└── worktrees/           # Sandboxes aislados para agentes (gitignored)
```

## 🛠 Instalación

```bash
git clone https://github.com/your-username/quorum.git
cd quorum
uv sync
chmod +x agents
```

## ⚖️ Licencia

MIT
