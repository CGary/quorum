# Patrón propuesto: `ReportService`

**Estado:** Idea de refactorización.
**Prioridad:** Media.
**Tipo:** Service layer para separar CLI de lógica de negocio.
**Alcance inicial:** `cmd/report.go` y funciones relacionadas en `internal/core/report.go`.

---

## 1. Problema actual

`cmd/report.go` contiene bastante lógica de negocio dentro de closures de Cobra:

- validación del ID del reporte;
- carga de template desde disco o bundle embebido;
- scaffold con fecha e ID;
- autofill de metadata;
- validación contra `report.schema.json`;
- dry-run;
- persistencia en `.ai/reports/`;
- registro del proyecto en memoria para `report new`.

El comando funciona, pero mezcla parsing CLI, mensajes de error, reglas de reporte y escritura de artefactos.

---

## 2. Patrón sugerido

Mover la lógica a un servicio de core:

```go
type ReportService struct {
    ProjectRoot string
    Store       ArtifactStore
}

type ReportNewOptions struct {
    OutputPath string
}

type ReportSaveOptions struct {
    DryRun bool
}

func (s ReportService) NewReport(id string, opts ReportNewOptions) (ReportResult, error)
func (s ReportService) SaveReport(id string, raw []byte, opts ReportSaveOptions) (ReportResult, error)
```

El CLI quedaría reducido a:

1. leer flags y stdin;
2. llamar al servicio;
3. imprimir resultado;
4. salir con código correcto.

---

## 3. Beneficios post-refactorización

### 3.1 CLI más delgada y consistente

`cmd/report.go` quedaría alineado con el resto de comandos: shim de Cobra hacia `internal/core`.

### 3.2 Tests de negocio sin Cobra

Se podrían probar casos como:

- `--dry-run` no escribe;
- `meta.id` debe coincidir con filename;
- template inválido falla antes de escribir;
- `--output` scaffolda fuera de `.ai/reports/`;
- metadata ausente se autocompleta.

sin ejecutar comandos CLI completos.

### 3.3 Menos duplicación de convenciones

El servicio puede reutilizar `ArtifactStore` y conservar la regla “validate-before-write” igual que los artefactos lifecycle.

### 3.4 Mejor evolución de ADR 0004

El visor de reportes es una excepción acotada en `quorum.md`. Un servicio dedicado permite mantener ese alcance claro y evitar que reportes contaminen el lifecycle `00`→`07`.

### 3.5 Facilidad para futuros comandos read-only

Si se agregan comandos seguros como `report list` o `report inspect`, el servicio puede exponer operaciones read-only sin duplicar path logic.

---

## 4. Riesgos y límites

- No convertir reportes en parte del lifecycle numerado.
- No agregar edición interactiva, temas, PDF ni builder visual sin ADR nuevo.
- No hacer que `ReportService` escriba memoria salvo el registro de proyecto ya justificado por el flujo actual.

---

## 5. Criterio de éxito

La refactorización es útil si `cmd/report.go` queda mayormente como capa de entrada/salida, mientras las reglas testeables viven en `internal/core`, manteniendo intacta la excepción estrecha definida por ADR 0004.
