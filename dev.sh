#!/usr/bin/env bash

# ==============================================================================
# Pharus - Script de Desarrollo con Hot Reload
# ==============================================================================

set -e

# Cargar variables de entorno desde .env si existe
if [ -f .env ]; then
    echo "🔑 Cargando variables de entorno desde .env..."
    set -a
    source .env
    set +a
fi

# Configurar PATH para incluir GOPATH/bin
GOPATH=$(go env GOPATH 2>/dev/null || echo "$HOME/go")
export PATH="$GOPATH/bin:$PATH"

echo "🚀 Configurando entorno de desarrollo para Pharus..."

# Crear directorios necesarios
mkdir -p tmp bin

# Verificar si 'air' está instalado
if ! command -v air &> /dev/null; then
    echo "⚠️ 'air' no fue encontrado en el PATH. Instalando github.com/air-verse/air..."
    if go install github.com/air-verse/air@latest; then
        echo "✅ Air instalado exitosamente en $GOPATH/bin/air"
    else
        echo "⚠️ Falló la instalación automática de Air. Iniciando en modo fallback (sin hot reload)..."
        echo "👉 Ejecutando: go run ./cmd/pharus daemon run"
        exec go run ./cmd/pharus daemon run "$@"
    fi
fi

echo "🔥 Iniciando Servidor Demonio Pharus en modo Hot Reload (Air)..."
echo "📡 Socket UDS: ~/.pharus/pharus.sock"
echo "🌐 Servidor HTTP & MCP: http://127.0.0.1:8765"
echo "------------------------------------------------------------------------------"

exec air "$@"
