# Problema: Memorias sin `project` no aparecen en búsquedas filtradas

## Contexto

La base de datos de producción (`data/engram.db`) acumula memorias que fueron ingresadas sin el campo `project` asignado — ya sea porque el cliente (Claude Code + MCP) no lo enviaba en ese momento, o porque se ingresaron antes de que el filtro por proyecto existiera.

Esto genera un falso negativo crítico: al llamar `search_fuzzy` con `--project <nombre>` (o el parámetro MCP equivalente), el resultado es 0 registros aunque la base de datos contenga contexto altamente relevante para ese proyecto.

## Reproducción

```bash
# Retorna 0 resultados pese a que el corpus tiene data del proyecto
hsme-cli search-fuzzy "arquitectura diseño SIAT integración facturación" --project "integration-bo-new"

# Retorna resultados — confirma que la data existe pero sin project asignado
hsme-cli search-fuzzy "arquitectura diseño SIAT integración facturación"
```

## Causa raíz

El filtro por `project` en `FuzzySearch` es una cláusula `WHERE project = ?` estricta. Las memorias con `project = ''` o `project IS NULL` quedan fuera del conjunto de candidatos léxicos y semánticos, sin fallback ni expansión.

## Impacto

- Búsquedas con `--project` son poco confiables mientras existan memorias huérfanas.
- El agente no puede acceder al contexto histórico de un proyecto si ese contexto fue guardado sin tag.
- A medida que el corpus crece, la proporción de memorias huérfanas se diluye, pero las más antiguas (y frecuentemente las más valiosas arquitectónicamente) permanecen invisibles.

## Soluciones posibles

### Opción A — Backfill por heurística de contenido (recomendada)

Crear un comando CLI o script que:
1. Lea todas las memorias con `project` vacío o nulo.
2. Aplique una heurística de clasificación (keywords del contenido, entidades del grafo de conocimiento, o embedding similarity contra un centroide por proyecto conocido).
3. Asigne el `project` más probable y marque el registro como `backfilled`.

Ventaja: no requiere input manual, escala.  
Riesgo: clasificación errónea si el contenido es ambiguo o cross-proyecto.

### Opción B — Fallback en búsqueda: si `project` filtra a 0, reintentar sin filtro

Modificar `FuzzySearch` para que, cuando el resultado filtrado es vacío, ejecute una segunda búsqueda sin filtro y marque los resultados como `untagged`.

Ventaja: inmediato, sin migración de datos.  
Desventaja: mezcla contexto de proyectos distintos; la señal de `project` pierde semántica.

### Opción C — Comando `admin tag-orphans` interactivo

CLI que presenta memorias huérfanas en lotes y permite al operador asignar el project manualmente o por regex sobre el contenido.

Ventaja: máxima precisión.  
Desventaja: no escala si el volumen es alto.

### Opción D — Índice invertido proyecto↔memoria por grafo de conocimiento

Aprovechar las entidades ya extraídas en el grafo para inferir el proyecto: si una memoria tiene entidades que el grafo asocia a un proyecto conocido, heredar ese tag.

Ventaja: usa información ya computada, alta coherencia arquitectónica.  
Desventaja: requiere que el grafo esté poblado y las entidades estén bien vinculadas.

## Decisión pendiente

Elegir entre A y D (o combinarlas) según la calidad del grafo actual y el volumen de memorias huérfanas. Medir primero:

```sql
SELECT COUNT(*) FROM memories WHERE project = '' OR project IS NULL;
SELECT COUNT(*) FROM memories;
```
