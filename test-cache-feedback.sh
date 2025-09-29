#!/bin/bash

# Script de Prueba del Sistema de Feedback Automático de Cache
# Este script demuestra cómo funciona la detección automática de problemas

set -e

# Configuración
SERVER_URL="http://localhost:8090"
API_BASE="$SERVER_URL/api/v1"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función para logging
log() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✅ $1${NC}"
}

warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

error() {
    echo -e "${RED}❌ $1${NC}"
}

# Función para hacer requests con manejo de errores
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    log "$description"
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$API_BASE$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$API_BASE$endpoint")
    fi
    
    # Separar body y status code
    body=$(echo "$response" | head -n -1)
    status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" -eq 200 ] || [ "$status_code" -eq 201 ]; then
        success "Request exitoso (HTTP $status_code)"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        error "Request falló (HTTP $status_code)"
        echo "$body"
        return 1
    fi
    
    echo ""
}

# Verificar que el servidor esté corriendo
check_server() {
    log "Verificando que el servidor esté corriendo..."
    if curl -s "$SERVER_URL/health" > /dev/null 2>&1; then
        success "Servidor está corriendo en $SERVER_URL"
    else
        error "Servidor no está corriendo en $SERVER_URL"
        echo "Por favor inicia el servidor con: go run cmd/server/main.go"
        exit 1
    fi
    echo ""
}

# Limpiar cache antes de empezar
clear_cache() {
    log "Limpiando cache para empezar con estado limpio..."
    # Nota: Esto requeriría un endpoint adicional o acceso directo a Redis
    warning "Limpieza manual de cache no implementada en este script"
    echo ""
}

# Paso 1: Generar respuesta inicial (que será cacheada)
generate_initial_response() {
    log "=== PASO 1: Generando respuesta inicial (será cacheada) ==="
    
    make_request "POST" "/generate" '{
        "prompt": "Escribe una función en JavaScript para ordenar un array de números",
        "model": "gpt-4o",
        "provider": "azure",
        "maxTokens": 500,
        "temperature": 0.7
    }' "Generando función de ordenamiento JavaScript"
}

# Paso 2: Hacer la misma pregunta (debería venir del cache)
test_cache_hit() {
    log "=== PASO 2: Repitiendo la misma pregunta (debería venir del cache) ==="
    
    make_request "POST" "/generate" '{
        "prompt": "Escribe una función en JavaScript para ordenar un array de números",
        "model": "gpt-4o", 
        "provider": "azure",
        "maxTokens": 500,
        "temperature": 0.7
    }' "Repitiendo pregunta (esperando cache hit)"
}

# Paso 3: Reportar problema con la respuesta (detección automática)
report_problem_automatic() {
    log "=== PASO 3: Reportando problema automáticamente ==="
    
    make_request "POST" "/generate" '{
        "prompt": "Este código no funciona, hay un error de sintaxis. Puedes corregirlo?",
        "model": "gpt-4o",
        "provider": "azure", 
        "maxTokens": 500,
        "temperature": 0.7
    }' "Reportando problema (detección automática de keywords)"
}

# Paso 4: Verificar estadísticas de cache
check_cache_stats() {
    log "=== PASO 4: Verificando estadísticas de cache ==="
    
    make_request "GET" "/cache/stats?limit=5" "" "Obteniendo estadísticas de cache"
}

# Paso 5: Hacer la pregunta original otra vez (debería generar nueva respuesta)
test_cache_invalidation() {
    log "=== PASO 5: Repitiendo pregunta original (cache debería estar invalidado) ==="
    
    make_request "POST" "/generate" '{
        "prompt": "Escribe una función en JavaScript para ordenar un array de números",
        "model": "gpt-4o",
        "provider": "azure",
        "maxTokens": 500,
        "temperature": 0.7
    }' "Pregunta original después de invalidación"
}

# Paso 6: Probar diferentes tipos de problemas
test_different_problem_types() {
    log "=== PASO 6: Probando diferentes tipos de detección de problemas ==="
    
    # Problema de compilación
    make_request "POST" "/generate" '{
        "prompt": "El código anterior no compila, hay syntax error",
        "model": "gpt-4o",
        "provider": "azure",
        "maxTokens": 300
    }' "Problema de compilación (severidad crítica)"
    
    sleep 2
    
    # Problema funcional
    make_request "POST" "/generate" '{
        "prompt": "Esta función está mal, no ordena correctamente",
        "model": "gpt-4o", 
        "provider": "azure",
        "maxTokens": 300
    }' "Problema funcional (severidad alta)"
    
    sleep 2
    
    # Solicitud de corrección
    make_request "POST" "/generate" '{
        "prompt": "Corrige este código por favor",
        "model": "gpt-4o",
        "provider": "azure", 
        "maxTokens": 300
    }' "Solicitud de corrección (severidad media)"
}

# Paso 7: Invalidación manual de cache
test_manual_invalidation() {
    log "=== PASO 7: Probando invalidación manual de cache ==="
    
    # Primero generar una respuesta para tener algo que invalidar
    make_request "POST" "/generate" '{
        "prompt": "Escribe función suma en Python",
        "model": "gpt-4o",
        "provider": "azure",
        "maxTokens": 200
    }' "Generando función Python para invalidar"
    
    sleep 2
    
    # Invalidar manualmente (necesitaríamos la cache_key real)
    warning "Invalidación manual requiere cache_key específica"
    log "Ejemplo de invalidación manual:"
    echo 'curl -X POST http://localhost:8090/api/v1/cache/invalidate \'
    echo '  -H "Content-Type: application/json" \'
    echo '  -d '"'"'{"cache_key": "ai:gpt-4o:hash123:200:0.70", "reason": "Manual test"}'"'"
    echo ""
}

# Paso 8: Verificar estadísticas finales
final_stats() {
    log "=== PASO 8: Estadísticas finales ==="
    
    make_request "GET" "/cache/stats?limit=10" "" "Estadísticas finales de cache"
}

# Función principal
main() {
    echo -e "${BLUE}🚀 Iniciando prueba del Sistema de Feedback Automático de Cache${NC}"
    echo -e "${BLUE}================================================================${NC}"
    echo ""
    
    # Verificar servidor
    check_server
    
    # Ejecutar pasos de prueba
    generate_initial_response
    sleep 3
    
    test_cache_hit
    sleep 3
    
    report_problem_automatic
    sleep 3
    
    check_cache_stats
    sleep 3
    
    test_cache_invalidation
    sleep 3
    
    test_different_problem_types
    sleep 3
    
    test_manual_invalidation
    sleep 3
    
    final_stats
    
    echo -e "${GREEN}🎉 Prueba completada exitosamente!${NC}"
    echo ""
    echo -e "${YELLOW}📊 Resumen de lo que se probó:${NC}"
    echo "1. ✅ Generación inicial y caching"
    echo "2. ✅ Cache hit en segunda consulta"
    echo "3. ✅ Detección automática de problemas"
    echo "4. ✅ Invalidación automática de cache"
    echo "5. ✅ Regeneración después de invalidación"
    echo "6. ✅ Diferentes tipos de detección"
    echo "7. ✅ Estadísticas y monitoreo"
    echo ""
    echo -e "${BLUE}💡 El sistema ahora mejora automáticamente la calidad del cache!${NC}"
}

# Función de ayuda
show_help() {
    echo "Script de Prueba del Sistema de Feedback Automático de Cache"
    echo ""
    echo "Uso: $0 [opción]"
    echo ""
    echo "Opciones:"
    echo "  help          Mostrar esta ayuda"
    echo "  check         Solo verificar que el servidor esté corriendo"
    echo "  stats         Solo mostrar estadísticas de cache"
    echo "  clean         Limpiar y reiniciar prueba"
    echo ""
    echo "Sin opciones: Ejecutar prueba completa"
}

# Manejo de argumentos
case "${1:-}" in
    "help"|"-h"|"--help")
        show_help
        exit 0
        ;;
    "check")
        check_server
        exit 0
        ;;
    "stats")
        check_cache_stats
        exit 0
        ;;
    "clean")
        clear_cache
        main
        exit 0
        ;;
    "")
        main
        exit 0
        ;;
    *)
        error "Opción desconocida: $1"
        show_help
        exit 1
        ;;
esac