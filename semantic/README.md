# HSME (Hybrid Semantic Memory Engine) v1.0.1+

HSME es un motor de memoria semántica híbrida de alto rendimiento diseñado para proporcionar una base de conocimiento persistente, con trazabilidad y observabilidad profunda para agentes de IA. Combina la velocidad de la búsqueda léxica, la profundidad de la búsqueda semántica y la estructura de un Grafo de Conocimiento técnico.

## 🏗️ Arquitectura de Tres Procesos

HSME separa runtime, procesamiento semántico y mantenimiento operativo para mantener baja latencia y permitir observabilidad escalable:

1.  **MCP Server (`hsme`)**: Servidor ligero que maneja las peticiones del agente vía `stdio`. Realiza búsquedas híbridas instantáneas y encola tareas de enriquecimiento.
2.  **Async Worker (`hsme-worker`)**: Proceso en segundo plano que consume la cola de tareas para:
    *   Generar embeddings de vectores (`nomic-embed-text`) con aceleración GPU (CUDA).
    *   Extraer entidades y relaciones técnicas (`phi3.5`).
    *   Construir el Grafo de Conocimiento dinámicamente con parsing tolerante.
3.  **Ops Runner (`hsme-ops`)**: Runner dedicado para mantenimiento operativo. Ejecuta rollups de métricas, políticas de retención y limpieza de base de datos sin afectar el rendimiento de las consultas.

## 🛡️ Mejoras de "Hardening" (v1.0.1)

*   **Integridad Autoritativa**: Implementación de **triggers de SQLite** para la sincronización automática de FTS5. Se eliminó la sincronización manual en favor de una consistencia garantizada a nivel de base de datos.
*   **Concurrencia Optimizada**: Uso de `_txlock=immediate` y limitación estratégica del pool de conexiones (MaxOpenConns=4) para maximizar el rendimiento en modo WAL, eliminando colisiones de escritura en ingestas masivas.
*   **Aceleración GPU (CUDA)**: Soporte completo para `ollama-cuda`, logrando una aceleración de **10x** en tareas de extracción de grafos y generación de embeddings.

## 📊 Observabilidad Industrial

HSME v1.0.1 integra un sistema de telemetría interno completo persistido en SQLite:

*   **Distributed Tracing**: Registro de trazas y spans para cada request MCP y tarea del worker.
*   **Metric Rollups**: Agregación automática de métricas (p50, p95, throughput) por minuto, hora y día.
*   **Retention Policies**: Limpieza automática de datos de telemetría según antigüedad y criticidad.
*   **Operator Views**: Vistas SQL predefinidas para identificar operaciones lentas y errores recurrentes (`obs_recent_slow_operations`, `obs_error_events`).
*   **Catch-up en restart**: Si `hsme-ops` no corre por un período, los buckets perdidos se procesan al reiniciar usando checkpoint persistente (`last_completed_bucket_start_utc`).

### Variables de Entorno de Observabilidad

| Variable | Default | Descripción |
|----------|---------|-------------|
| `HSME_OBS_LEVEL` | `off` | Nivel: `off`, `basic`, `debug`, `trace` |
| `HSME_OBS_SAMPLE_RATE` | `0.10` | Tasa de sampleo para modo basic |
| `HSME_OBS_SLOW_THRESHOLDS` | (ver abajo) | Umbrales de operaciones lentas |
| `HSME_OBS_RAW_RETENTION_DAYS` | `7` | Retención para trazas/spans/events crudos |
| `HSME_OBS_MINUTE_RETENTION_DAYS` | `7` | Retención para rollups por minuto |
| `HSME_OBS_HOUR_RETENTION_DAYS` | `30` | Retención para rollups por hora |
| `HSME_OBS_DAY_RETENTION_DAYS` | `365` | Retención para rollups por día |

Umbrales por defecto para operaciones lentas:
- `mcp.request`: 100ms
- `mcp.tools/call`: 100ms
- `worker.lease`: 200ms
- `worker.execute`: 2s
- `ops.raw_to_minute`: 2s
- `ops.retention`: 2s

## 🚀 Instalación y Setup

### Requisitos
- Go 1.26+ con CGO habilitado.
- [Just](https://github.com/casey/just) para la gestión de tareas.
- Ollama (preferiblemente `ollama-cuda`) con modelos: `nomic-embed-text` y `phi3.5`.

### Instalación Rápida
```bash
# Compila e instala binarios en ~/go/bin
just install
```

## 🔄 Migración de Legado (Engram)

HSME incluye herramientas para la transición completa desde el sistema Engram original, restaurando metadatos históricos y asegurando la integridad del corpus:

### Flujo de Cutover
1. **Full Run**: `just migrate full` — Sincroniza metadatos, limpia basura e ingiere huérfanos.
2. **Cutover**: Desactivar el servidor MCP de Engram en el cliente (Claude Code).
3. **Delta Replay**: `just migrate delta` — Ingiere cualquier escritura realizada durante la ventana de transición.
4. **Verificación**: `just verify-cutover` — Compara conteos entre legado y HSME.

Consulta la [Guía de Cutover](docs/legacy-cutover-checklist.md) para el checklist operativo paso a paso.

## 📁 Filtrado por Proyecto

Las herramientas de búsqueda soportan un parámetro opcional `project` para restringir los resultados a un contexto específico:

*   **`search_fuzzy`**: Acepta `project` (string) para filtrar candidatos léxicos y semánticos.
*   **`search_exact`**: Acepta `project` (string) para búsquedas de subcadenas exactas.
*   **`store_context`**: Permite asignar un `project` al guardar nuevas memorias.
*   **`recall_recent_session`**: Acepta `project` (string) para filtrar memorias recentes por proyecto.

Si se omite el proyecto, la búsqueda se realiza sobre todo el corpus (comportamiento por defecto).


## ⏳ Ranking por Recencia (Time Decay)

HSME soporta ranking por recencia para priorizar información fresca sin perder relevancia semántica. Con `RRF_TIME_DECAY=on`, las consultas con intención explícita de recencia (`latest`, `recent`, `last`, etc.) reciben candidatos recientes adicionales inferidos por tipo (`session_summary`, `bugfix`, `decision`, `architecture`) y términos del tema. Las consultas sin intención de recencia conservan el ranking de relevancia base para no enterrar memorias antiguas pero críticas.

### Configuración
- `RRF_TIME_DECAY`: Establecer en `on` para activar el decaimiento o `off` para desactivarlo (por defecto `off`). Otros valores se rechazan al iniciar.
- `RRF_HALF_LIFE_DAYS`: Vida media en días (por defecto `14.0`). Un documento con esta antigüedad verá su score de relevancia reducido a la mitad.

### Benchmarking
Puedes evaluar el impacto del decaimiento en tu corpus actual usando la herramienta de benchmark:
```bash
# Compilar y ejecutar contra el corpus actual
go build -tags "sqlite_fts5 sqlite_vec" -o bench-decay ./cmd/bench-decay
./bench-decay \
  -db data/engram.db \
  -eval docs/future-missions/mission-3-eval-set.yaml \
  -baseline docs/future-missions/mission-3-baseline.json \
  -half-life 14.0
```
Los reportes se generan en `data/benchmarks/<run_id>/` e incluyen comparativas OFF vs ON, deltas de ranking, métricas por categoría y muestras de búsqueda exacta. Por defecto el benchmark usa el mismo embedder de `search_fuzzy`; `-no-vector` queda reservado para pruebas offline y no debe usarse para aceptación de misión.

### Seguridad y Rollback
El decaimiento está desactivado por defecto. Para revertir cualquier cambio en el ranking, simplemente elimina la variable de entorno `RRF_TIME_DECAY` o establécela en `off`.

## 📂 Operación y Mantenimiento

### Gestión de Procesos en Segundo Plano
HSME permite ejecutar sus componentes de enriquecimiento y mantenimiento en segundo plano para no bloquear la terminal:

*   **`just start-all`**: Lanza el worker y el runner de ops en background.
*   **`just stop-all`**: Detiene todos los procesos de HSME en segundo plano de forma segura.
*   **`just work-bg` / `just stop-work`**: Control individual del worker de grafos/embeddings.
*   **`just ops-bg` / `just stop-ops`**: Control individual del runner de observabilidad.

Los logs de estos procesos se almacenan en el directorio `logs/` (`logs/worker.log` y `logs/ops.log`).

### Monitoreo y Diagnóstico
Para supervisar el sistema, se utiliza directamente el **CLI Unificado** (`hsme-cli`), que proporciona las herramientas principales de diagnóstico:

*   **`hsme-cli status`**: Muestra una instantánea de la salud del sistema, incluyendo el estado de la cola de tareas, la cobertura de vectores y el progreso del grafo de conocimiento.
*   **`watch -n 2 -c "hsme-cli status"`**: Permite un monitoreo en tiempo real del estado del sistema.
*   **`hsme-cli admin retry-failed`**: Reencola automáticamente tareas que fallaron o agotaron su tiempo de ejecución.

### Comandos de Utilidad
- `just serve`: Inicia el servidor MCP (comunicación stdio).
- `just install`: Compila e instala todos los binarios (`hsme`, `hsme-worker`, `hsme-ops`, `hsme-cli`) en `~/go/bin`.
- `just cli-install`: Instala únicamente el CLI.
- `just migrate [full|delta]`: Ejecuta la migración desde Engram legado (mode: `full`, `delta`, o `dry-run`).
- `just backup/restore`: Gestión de snapshots atómicos compatibles con WAL.
- `just clean`: Elimina binarios locales y limpia los archivos de log.

## 🔌 Configuración del Cliente MCP

```json
{
  "mcpServers": {
    "hsme": {
      "command": "/absolute/path/to/hsme",
      "env": {
        "SQLITE_DB_PATH": "/absolute/path/to/data/engram.db",
        "OLLAMA_HOST": "http://localhost:11434",
        "HSME_OBS_LEVEL": "basic" 
      }
    }
  }
}
```

---
**Desarrollo**: Este proyecto sigue los principios de **Spec-Driven Development (SDD)** con **Strict TDD Mode**. Consulta el `Technical_Specification.md` para detalles internos del esquema y el flujo de ingesta.
