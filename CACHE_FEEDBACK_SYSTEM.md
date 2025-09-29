# Sistema de Feedback Automático de Cache

## 🎯 Descripción

El Sistema de Feedback Automático de Cache es una funcionalidad avanzada que detecta automáticamente cuando las respuestas cacheadas son problemáticas y las invalida para mejorar la calidad del servicio de forma colaborativa.

## 🔍 Problema que Resuelve

### Problema Original:

- **Cache Global**: Las respuestas se comparten entre todos los usuarios
- **Código Malo se Propaga**: Si un usuario recibe código con errores, todos los usuarios futuros reciben el mismo código malo
- **Sin Mecanismo de Corrección**: No había forma de corregir automáticamente respuestas problemáticas

### Ejemplo del Problema:

1. Usuario A pregunta: "Escribe una función para ordenar un array en JavaScript"
2. La AI genera código con bugs
3. El código malo se guarda en cache
4. Usuarios B, C, D... reciben el mismo código malo del cache

## 💡 Solución Implementada

### Detección Automática Inteligente:

El sistema analiza automáticamente cada prompt del usuario para detectar si indica problemas con la respuesta anterior.

### Flujo de Auto-Corrección:

```
Usuario recibe código malo → Usuario dice "este código no funciona" →
Sistema detecta problema automáticamente → Cache invalidado →
Próxima pregunta genera respuesta nueva → Código corregido se cachea →
Usuarios futuros reciben código bueno
```

## 🛠️ Componentes Técnicos

### 1. Base de Datos (PostgreSQL)

#### Tabla `cache_feedback`:

```sql
CREATE TABLE cache_feedback (
    id SERIAL PRIMARY KEY,
    cache_key VARCHAR(255) NOT NULL,
    user_id VARCHAR(100),
    session_id VARCHAR(100),
    problem_detected BOOLEAN DEFAULT true,
    detection_method VARCHAR(50), -- 'keywords', 'pattern', 'sequence'
    problem_keywords TEXT,
    original_prompt TEXT,
    problem_prompt TEXT,
    severity_score INTEGER DEFAULT 1, -- 1=bajo, 5=crítico
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### Tabla `cache_stats`:

```sql
CREATE TABLE cache_stats (
    cache_key VARCHAR(255) PRIMARY KEY,
    total_uses INTEGER DEFAULT 0,
    problem_reports INTEGER DEFAULT 0,
    quality_score DECIMAL(3,2) DEFAULT 1.0, -- 0.0=muy malo, 1.0=perfecto
    is_blacklisted BOOLEAN DEFAULT false,
    last_problem_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### 2. Sistema de Detección (`internal/cache/feedback.go`)

#### Métodos de Detección:

**A. Palabras Clave Críticas:**

- `error`, `no funciona`, `está mal`, `incorrecto`, `bug`
- `no compila`, `syntax error`, `runtime error`
- `corrige`, `arregla`, `fix`, `repair`

**B. Patrones de Corrección:**

- `"este código no funciona"`
- `"el código anterior está mal"`
- `"puedes corregir esto"`
- `"hay un error"`

**C. Contexto de Código:**

- Detecta si la respuesta anterior contenía código (````)
- Busca problemas específicos de programación
- Asigna severidad crítica (5) a problemas de código

#### Puntuación de Severidad:

- **5 (Crítico)**: Errores de compilación, syntax errors
- **4 (Alto)**: "no funciona", "broken"
- **3 (Medio)**: "incorrecto", "wrong"
- **2 (Bajo-Medio)**: "corrige", "arregla"
- **1 (Bajo)**: Otros problemas menores

### 3. Integración con AI Manager

#### Análisis Automático:

```go
// En cada request, analizar si indica problemas con respuesta anterior
if m.feedbackManager != nil && previousResponse != "" {
    detection := m.feedbackManager.AnalyzeForProblems(ctx, originalPrompt, previousResponse)
    if detection.HasProblem {
        // Reportar problema y posiblemente invalidar cache
        m.feedbackManager.ReportProblem(ctx, cacheKey, userID, sessionID, detection, originalPrompt, problemPrompt)
    }
}
```

#### Verificación de Lista Negra:

```go
// Antes de usar cache, verificar si está blacklisted
if blacklisted, err := m.feedbackManager.IsBlacklisted(ctx, cacheKey); err == nil && blacklisted {
    // Saltar cache, generar respuesta nueva
}
```

## 🚀 Funcionalidades

### 1. Detección Automática

- **Sin intervención del usuario**: El sistema detecta problemas automáticamente
- **Análisis contextual**: Considera el historial de conversación
- **Múltiples métodos**: Keywords, patrones regex, análisis de código

### 2. Invalidación Inteligente

- **Severidad crítica**: Invalidación inmediata
- **Múltiples reportes**: Auto-blacklist después de 3+ reportes
- **TTL dinámico**: Respuestas problemáticas expiran más rápido

### 3. Estadísticas y Monitoreo

- **Métricas de calidad**: Score de 0.0 (malo) a 1.0 (perfecto)
- **Reportes de problemas**: Tracking de caches más problemáticos
- **Rate de problemas**: Porcentaje de problemas vs usos totales

### 4. APIs de Administración

#### Obtener Estadísticas:

```bash
curl -X GET http://localhost:8090/api/v1/cache/stats?limit=10
```

#### Invalidar Cache Manualmente:

```bash
curl -X POST http://localhost:8090/api/v1/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{"cache_key": "ai:gpt-4o:abc123:4096:0.70", "reason": "Manual invalidation"}'
```

## 📊 Métricas y Reportes

### Vista `cache_quality_report`:

```sql
SELECT
    cache_key,
    total_uses,
    problem_reports,
    quality_score,
    is_blacklisted,
    problem_rate_percent,
    CASE
        WHEN quality_score >= 0.8 THEN 'EXCELLENT'
        WHEN quality_score >= 0.6 THEN 'GOOD'
        WHEN quality_score >= 0.4 THEN 'FAIR'
        WHEN quality_score >= 0.2 THEN 'POOR'
        ELSE 'CRITICAL'
    END as quality_rating
FROM cache_quality_report
ORDER BY problem_reports DESC;
```

## ⚙️ Configuración

### Variables de Entorno:

```env
# Habilitar sistema de feedback de cache
ENABLE_CACHE_FEEDBACK=true

# Conexión a PostgreSQL (requerida)
POSTGRES_URL=postgresql://user:password@localhost:5432/mcp_ai

# Habilitar gestión de sesiones (recomendado para mejor detección)
ENABLE_SESSION_MANAGEMENT=true
USE_DATABASE_PERSISTENCE=true
```

### Migración de Base de Datos:

```bash
# Ejecutar migración
psql -d mcp_ai -f migrations/0008_cache_feedback.sql
```

## 🔄 Flujo de Funcionamiento

### 1. Usuario Hace Pregunta:

```
POST /api/v1/generate
{
  "prompt": "Escribe función ordenar array",
  "model": "gpt-4o",
  "provider": "azure"
}
```

### 2. Sistema Busca en Cache:

- Genera clave: `ai:gpt-4o:hash123:4096:0.70`
- Verifica si está blacklisted
- Si no está blacklisted y existe, devuelve cache
- Incrementa contador de uso

### 3. Usuario Reporta Problema:

```
POST /api/v1/generate
{
  "prompt": "Este código no funciona, hay un error de sintaxis",
  "model": "gpt-4o",
  "provider": "azure"
}
```

### 4. Sistema Detecta Problema:

- Analiza prompt: detecta keywords "no funciona", "error"
- Identifica severidad: 5 (crítico)
- Reporta problema en base de datos
- Invalida cache inmediatamente (severidad crítica)

### 5. Próximo Usuario:

- Misma pregunta genera respuesta nueva (cache invalidado)
- Código corregido se guarda en cache
- Usuarios futuros reciben código bueno

## 🎯 Beneficios

### ✅ **Auto-Corrección Colaborativa**

- Los usuarios mejoran el sistema para todos automáticamente
- Sin necesidad de intervención manual

### ✅ **Calidad Incremental**

- El cache se vuelve mejor con el tiempo
- Detección de patrones en errores comunes

### ✅ **Costo-Efectivo**

- Solo regenera cuando hay problemas reales
- Mantiene beneficios de cache para respuestas buenas

### ✅ **Experiencia Mejorada**

- Menos código malo circulando
- Respuestas de mejor calidad progresivamente

### ✅ **Transparente**

- Funciona automáticamente en segundo plano
- No requiere acciones adicionales del usuario

## 🔧 Mantenimiento

### Limpieza Automática:

```go
// Limpiar feedback antiguo (>30 días)
feedbackManager.CleanupOldFeedback(ctx)
```

### Monitoreo:

- Revisar `/api/v1/cache/stats` regularmente
- Identificar patrones en caches problemáticos
- Ajustar algoritmos de detección según sea necesario

## 🚀 Futuras Mejoras

### 1. Machine Learning:

- Entrenar modelos para detectar respuestas problemáticas
- Análisis semántico avanzado de prompts

### 2. Feedback Explícito:

- Botones de "👍" / "👎" en interfaces
- Integración con sistemas de rating

### 3. Análisis de Código:

- Validación sintáctica automática
- Ejecución de código en sandbox para detectar errores

### 4. Personalización:

- Cache por usuario para casos específicos
- Configuración de sensibilidad de detección

## 📝 Ejemplo de Uso Completo

```bash
# 1. Usuario A hace pregunta
curl -X POST http://localhost:8090/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Escribe función ordenar array JavaScript",
    "model": "gpt-4o",
    "provider": "azure"
  }'
# Respuesta: código con bug, se guarda en cache

# 2. Usuario A reporta problema (automático)
curl -X POST http://localhost:8090/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Este código no compila, hay error de sintaxis",
    "model": "gpt-4o",
    "provider": "azure"
  }'
# Sistema detecta problema, invalida cache

# 3. Usuario B hace misma pregunta
curl -X POST http://localhost:8090/api/v1/generate \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Escribe función ordenar array JavaScript",
    "model": "gpt-4o",
    "provider": "azure"
  }'
# Cache invalidado, genera respuesta nueva y correcta

# 4. Ver estadísticas
curl -X GET http://localhost:8090/api/v1/cache/stats
# Muestra métricas de calidad y problemas detectados
```

Este sistema convierte el cache global de un problema potencial en una ventaja colaborativa donde la comunidad mejora automáticamente la calidad del sistema para todos los usuarios.
