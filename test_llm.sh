#!/usr/bin/env bash

# ==============================================================================
# Pharus - Runner de Pruebas en Vivo para LLM (MiniMax, OpenAI, Anthropic, Ollama)
# ==============================================================================

set -e

# Cargar variables desde .env si existe
if [ -f .env ]; then
    echo "🔑 Cargando variables de entorno desde .env..."
    set -a
    source .env
    set +a
fi

echo "🧪 Ejecutando suite de pruebas de integración LLM en vivo..."
echo "------------------------------------------------------------------------------"

go test -v -run TestLive ./internal/llm/... ./internal/research/...

echo "------------------------------------------------------------------------------"
echo "✅ ¡Pruebas de integración de LLM finalizadas con éxito!"
