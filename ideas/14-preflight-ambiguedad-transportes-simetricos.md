# 🐞 Defecto: `quorum analyze fleet-preflight` marca como error toda celda declarada en dos transportes

**Estado:** Detectado el 2026-09-03 durante el verify de FLEET-035; sin implementar.
**Contexto:** Quorum, `internal/core/fleet_preflight.go` (check 2: "cada modelo de `config.yaml.levels` debe casar con exactamente un transporte activo"), `.agents/fleet/agents.yaml`, `internal/core/fleet_route.go`.

## 1. Lo que pasó

El check 2 del preflight devuelve `ambiguous=matched by transports [agy agy_edit]` para sonnet-4-6, opus-4-6, 3.6-flash-high y 3.7-flash-medium, y `[aider opencode]` para nemotron-ultra, north-mini-code y nemotron-nano-omni. En `main` (antes de FLEET-035) ya había 11 errores de este tipo; después, 10. Ninguno es un defecto real: el catálogo es **simétrico por diseño** desde el 2026-07-31 (los mismos modelos en `agy` one-shot y `agy_edit` agéntico) y los modelos gratuitos de OpenRouter viven a la vez en `opencode` y `aider`.

## 2. Por qué es un defecto del preflight y no del catálogo

`core.Route` ya desambigua: excluye los transportes `mode: oneshot` del implement y ordena el resto con `config.yaml.policies.fleet_transport_order`. El preflight no aplica ninguna de las dos reglas, así que reporta como error la situación normal del sistema. Consecuencia práctica: un `assert not errors` sobre el preflight es insatisfacible, y un modelo realmente muerto (0 transportes) queda enterrado entre falsos positivos.

## 3. Corrección propuesta

- Que el check 2 resuelva la ambigüedad igual que el router: descartar transportes `oneshot` para la fase implement y, si sigue habiendo más de uno, elegir por `fleet_transport_order`; solo 0 coincidencias es error. Mantener la pureza (los datos ya vienen en el request; `config.yaml.policies.fleet_transport_order` habría que añadirlo al JSON de entrada).
- Degradar la ambigüedad residual a `warnings`, nunca a `errors`.
- Test con fixture simétrico (`agy`/`agy_edit`) que hoy fallaría.

## 4. Relación

Idea 13 / FLEET-035: el comando 4 de `verify.commands` se acotó a "ningún modelo muerto ni sin transporte" ignorando los errores `ambiguous=` precisamente por este defecto. FLEET-037 (`quorum fleet catalog`) cubre el lado vivo del catálogo; esta idea cubre el lado declarado.
