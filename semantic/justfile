set shell := ["sh", "-c"]

# Configuración de compilación (Sincronizado con Makefile)
export CGO_ENABLED := "1"
GO_TAGS := "sqlite_fts5 sqlite_vec"
INSTALL_PATH := home_dir() + "/go/bin"

# Rutas de datos
PROJECT_ROOT := invocation_directory()
DB_DIR := PROJECT_ROOT + "/data"
export SQLITE_DB_PATH := DB_DIR + "/engram.db"
BACKUP_DIR := "backups"

# Compilar binarios locales
build:
        go build -tags "{{GO_TAGS}}" -o hsme ./cmd/hsme
        go build -tags "{{GO_TAGS}}" -o hsme-worker ./cmd/worker
        go build -tags "{{GO_TAGS}}" -o hsme-ops ./cmd/ops
        go build -tags "{{GO_TAGS}}" -o migrate-legacy ./cmd/migrate-legacy
        @echo "✅ Binarios compilados en la raíz."

# Build hsme-cli binary
cli-build:
        @mkdir -p bin
        go build -tags "{{GO_TAGS}}" -o bin/hsme-cli ./cmd/cli

# Ejecutar la migración de legado
migrate mode="full":
        ./migrate-legacy --mode={{mode}}

# Verificación de cutover
verify-cutover:
        @./scripts/verify_cutover.sh

# Ejecutar tests con soporte para FTS5 y Vectores
test:
        go test -v -tags "{{GO_TAGS}}" ./...

# Compilar e instalar binarios de forma global
install: cli-install
        @mkdir -p {{INSTALL_PATH}}
        go build -tags "{{GO_TAGS}}" -o {{INSTALL_PATH}}/hsme ./cmd/hsme
        go build -tags "{{GO_TAGS}}" -o {{INSTALL_PATH}}/hsme-worker ./cmd/worker
        go build -tags "{{GO_TAGS}}" -o {{INSTALL_PATH}}/hsme-ops ./cmd/ops
        @echo "✅ Binarios instalados en {{INSTALL_PATH}}."

# Install hsme-cli
cli-install: cli-build
        @mkdir -p {{INSTALL_PATH}}
        @if [ -f bin/hsme-cli ]; then cp bin/hsme-cli {{INSTALL_PATH}}/hsme-cli && echo "✅ hsme-cli instalado en {{INSTALL_PATH}}"; else echo "⚠️ hsme-cli no encontrado (cmd/cli puede no existir aún)"; fi

# Ejecutar el servidor MCP
serve:
        ./hsme

# Ejecutar el worker de grafos
work:
        ./hsme-worker

# Lanzar el worker en segundo plano
work-bg:
        @mkdir -p logs
        @nohup ./hsme-worker > logs/worker.log 2>&1 &
        @echo "🚀 Worker lanzado en segundo plano (tail -f logs/worker.log)"

# Detener el worker en segundo plano
stop-work:
        @pkill -f "[h]sme-worker" && echo "🛑 Worker detenido." || echo "⚠️ El worker no estaba corriendo."

# Ejecutar el runner de observabilidad/ops
ops:
        ./hsme-ops once

# Lanzar ops en modo loop
ops-loop:
        ./hsme-ops loop

# Lanzar ops en modo loop en segundo plano
ops-bg:
        @mkdir -p logs
        @nohup ./hsme-ops loop > logs/ops.log 2>&1 &
        @echo "📊 Ops runner lanzado en segundo plano (tail -f logs/ops.log)"

# Detener el ops runner en segundo plano
stop-ops:
        @pkill -f "[h]sme-ops" && echo "🛑 Ops runner detenido." || echo "⚠️ El ops runner no estaba corriendo."

# Detener todos los procesos en segundo plano
stop-all: stop-work stop-ops

# Iniciar todos los procesos en segundo plano
start-all: work-bg ops-bg

# Reencolar tareas fallidas agotadas para que el worker pueda retomarlas
retry-failed:
        @./bin/hsme-cli admin retry-failed

# Realizar un backup ATÓMICO (Compatible con WAL)
backup:
        @./bin/hsme-cli admin backup

# Restaurar base de datos
restore:
        @./bin/hsme-cli admin restore --latest

# Limpiar binarios locales
clean:
        rm -f hsme hsme-worker hsme-ops hsme-cli logs/worker.log logs/ops.log
