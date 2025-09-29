-- Migración para sistema de feedback automático de cache
-- Permite detectar y rastrear problemas en respuestas cacheadas

CREATE TABLE cache_feedback (
    id SERIAL PRIMARY KEY,
    cache_key VARCHAR(255) NOT NULL,
    user_id VARCHAR(100),
    session_id VARCHAR(100),
    problem_detected BOOLEAN DEFAULT true,
    detection_method VARCHAR(50) NOT NULL, -- 'keywords', 'pattern', 'sequence', 'multiple_reports'
    problem_keywords TEXT, -- Palabras clave que activaron la detección
    original_prompt TEXT, -- Prompt que generó la respuesta problemática
    problem_prompt TEXT, -- Prompt que indicó el problema
    severity_score INTEGER DEFAULT 1, -- 1=bajo, 5=crítico
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Índices para optimizar consultas
CREATE INDEX idx_cache_feedback_cache_key ON cache_feedback(cache_key);
CREATE INDEX idx_cache_feedback_user_session ON cache_feedback(user_id, session_id);
CREATE INDEX idx_cache_feedback_created_at ON cache_feedback(created_at);
CREATE INDEX idx_cache_feedback_problem_detected ON cache_feedback(problem_detected);

-- Tabla para estadísticas de cache por clave
CREATE TABLE cache_stats (
    cache_key VARCHAR(255) PRIMARY KEY,
    total_uses INTEGER DEFAULT 0,
    problem_reports INTEGER DEFAULT 0,
    last_problem_at TIMESTAMP,
    quality_score DECIMAL(3,2) DEFAULT 1.0, -- 0.0 = muy malo, 1.0 = perfecto
    is_blacklisted BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Índices para cache_stats
CREATE INDEX idx_cache_stats_quality_score ON cache_stats(quality_score);
CREATE INDEX idx_cache_stats_blacklisted ON cache_stats(is_blacklisted);
CREATE INDEX idx_cache_stats_problem_reports ON cache_stats(problem_reports);

-- Función para actualizar estadísticas automáticamente
CREATE OR REPLACE FUNCTION update_cache_stats()
RETURNS TRIGGER AS $$
BEGIN
    -- Insertar o actualizar estadísticas de cache
    INSERT INTO cache_stats (cache_key, problem_reports, last_problem_at, updated_at)
    VALUES (NEW.cache_key, 1, NEW.created_at, NOW())
    ON CONFLICT (cache_key) 
    DO UPDATE SET 
        problem_reports = cache_stats.problem_reports + 1,
        last_problem_at = NEW.created_at,
        quality_score = GREATEST(0.0, cache_stats.quality_score - 0.1), -- Reducir calidad
        is_blacklisted = CASE 
            WHEN cache_stats.problem_reports + 1 >= 3 THEN true 
            ELSE cache_stats.is_blacklisted 
        END,
        updated_at = NOW();
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger para actualizar estadísticas automáticamente
CREATE TRIGGER trigger_update_cache_stats
    AFTER INSERT ON cache_feedback
    FOR EACH ROW
    EXECUTE FUNCTION update_cache_stats();

-- Función para incrementar uso de cache (se llamará desde Go)
CREATE OR REPLACE FUNCTION increment_cache_usage(p_cache_key VARCHAR(255))
RETURNS VOID AS $$
BEGIN
    INSERT INTO cache_stats (cache_key, total_uses, updated_at)
    VALUES (p_cache_key, 1, NOW())
    ON CONFLICT (cache_key)
    DO UPDATE SET 
        total_uses = cache_stats.total_uses + 1,
        updated_at = NOW();
END;
$$ LANGUAGE plpgsql;

-- Vista para obtener estadísticas de calidad de cache
CREATE VIEW cache_quality_report AS
SELECT 
    cs.cache_key,
    cs.total_uses,
    cs.problem_reports,
    cs.quality_score,
    cs.is_blacklisted,
    cs.last_problem_at,
    CASE 
        WHEN cs.total_uses = 0 THEN 0
        ELSE ROUND((cs.problem_reports::DECIMAL / cs.total_uses::DECIMAL) * 100, 2)
    END as problem_rate_percent,
    CASE
        WHEN cs.quality_score >= 0.8 THEN 'EXCELLENT'
        WHEN cs.quality_score >= 0.6 THEN 'GOOD'
        WHEN cs.quality_score >= 0.4 THEN 'FAIR'
        WHEN cs.quality_score >= 0.2 THEN 'POOR'
        ELSE 'CRITICAL'
    END as quality_rating
FROM cache_stats cs
ORDER BY cs.problem_reports DESC, cs.total_uses DESC;

-- Comentarios para documentación
COMMENT ON TABLE cache_feedback IS 'Rastrea problemas detectados automáticamente en respuestas cacheadas';
COMMENT ON TABLE cache_stats IS 'Estadísticas agregadas de calidad y uso de cache por clave';
COMMENT ON VIEW cache_quality_report IS 'Reporte de calidad de cache con métricas calculadas';
COMMENT ON COLUMN cache_feedback.detection_method IS 'Método usado para detectar el problema: keywords, pattern, sequence, multiple_reports';
COMMENT ON COLUMN cache_feedback.severity_score IS 'Puntuación de severidad del problema: 1=bajo, 5=crítico';
COMMENT ON COLUMN cache_stats.quality_score IS 'Puntuación de calidad: 0.0=muy malo, 1.0=perfecto';
COMMENT ON COLUMN cache_stats.is_blacklisted IS 'Si true, esta clave de cache será invalidada automáticamente';
