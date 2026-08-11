# Lista todas las recetas disponibles con su descripción
default:
    @just --list

# --- Build & test (núcleo, ADR 0008: Go puro, sin CGO) ---

# Compila el binario `quorum` local (CGO_ENABLED=0, igual que en CI)
build:
    CGO_ENABLED=0 go build -o quorum .

# Instala `quorum` globalmente en $GOPATH/bin (~/go/bin)
install:
    go install .

# Ejecuta el CLI sin compilar binario: just run task list
run *ARGS:
    go run . {{ARGS}}

# Corre la suite completa de tests del módulo raíz
test:
    go test ./...

# Corre un test puntual: just test-one ./internal/core TestPartitionFeedbackFindings
test-one PKG TEST:
    go test {{PKG}} -run {{TEST}} -v

# Test ácido de ADR 0008: el core compila y pasa tests SIN CGO, sin compilador C y sin depender de semantic/
acid:
    CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go test ./...

# Análisis estático estándar de Go (errores comunes no detectados por el compilador)
vet:
    go vet ./...

# --- Operación diaria ---

# Diagnóstico read-only del proyecto: 6 chequeos, exit 0 limpio / 1 con hallazgos
doctor:
    quorum doctor

# Levanta el visor read-only (proyectos, reports, memorias, estado de tareas) en segundo plano
serve:
    quorum serve start

# Detiene el visor en segundo plano
serve-stop:
    quorum serve stop

# Lista las tareas SDC y su estado (inbox/active/done/failed)
tasks:
    quorum task list

# Telemetría agregada de dispatches externos terminados, por celda/nivel/banda
fleet-stats:
    quorum fleet stats

# --- Puente al módulo semantic (HSME se construye SOLO desde semantic/, ADR 0008) ---

# Reenvía cualquier receta al justfile de semantic/: just sem test, just sem install
sem *ARGS:
    cd semantic && just {{ARGS}}

# --- Limpieza ---

# Elimina el binario local `quorum` (usa el global de ~/go/bin; el local envejece y rutea mal)
clean:
    rm -f quorum
