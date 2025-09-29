package cache

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// FeedbackManager maneja la detección automática de problemas en cache
type FeedbackManager struct {
	db     *sql.DB
	redis  *RedisClient
	logger *zap.Logger
}

// ProblemDetection resultado de análisis de problemas
type ProblemDetection struct {
	HasProblem      bool
	DetectionMethod string
	Keywords        []string
	SeverityScore   int
	Confidence      float64
	SuggestedAction string
}

// CacheStats estadísticas de una clave de cache
type CacheStats struct {
	CacheKey       string
	TotalUses      int
	ProblemReports int
	QualityScore   float64
	IsBlacklisted  bool
	LastProblemAt  *time.Time
	ProblemRate    float64
	QualityRating  string
}

// NewFeedbackManager crea un nuevo manager de feedback
func NewFeedbackManager(db *sql.DB, redis *RedisClient, logger *zap.Logger) *FeedbackManager {
	return &FeedbackManager{
		db:     db,
		redis:  redis,
		logger: logger,
	}
}

// AnalyzeForProblems analiza si un prompt indica problemas con la respuesta anterior
func (fm *FeedbackManager) AnalyzeForProblems(ctx context.Context, currentPrompt, previousResponse string) *ProblemDetection {
	detection := &ProblemDetection{
		HasProblem:      false,
		DetectionMethod: "",
		Keywords:        []string{},
		SeverityScore:   0,
		Confidence:      0.0,
		SuggestedAction: "none",
	}

	// Normalizar prompt para análisis
	promptLower := strings.ToLower(strings.TrimSpace(currentPrompt))

	// 1. Detección por palabras clave críticas
	criticalKeywords := []string{
		// Errores generales
		"error", "bug", "falla", "problema", "issue", "fail", "failed", "failure",
		"exception", "crash", "crashed", "hang", "hung", "freeze", "frozen",

		// Problemas de funcionamiento
		"no funciona", "doesn't work", "not working", "broken", "roto", "dañado",
		"no responde", "not responding", "unresponsive", "stuck", "atascado",
		"lento", "slow", "timeout", "time out", "expired", "expirado",

		// Problemas de código
		"no compila", "syntax error", "runtime error", "compilation error",
		"parse error", "undefined", "null pointer", "segmentation fault",
		"stack overflow", "memory leak", "out of memory", "access denied",
		"permission denied", "file not found", "connection refused",

		// Calidad/Corrección
		"incorrecto", "wrong", "está mal", "malo", "bad", "terrible",
		"horrible", "awful", "useless", "inútil", "garbage", "basura",
		"nonsense", "sin sentido", "absurd", "absurdo", "ridiculous",

		// Solicitudes de corrección
		"corrige", "arregla", "fix", "repair", "solve", "resuelve",
		"help", "ayuda", "please fix", "por favor arregla", "necesito ayuda",
		"can you fix", "puedes arreglar", "how to fix", "cómo arreglar",

		// Problemas específicos
		"missing", "falta", "incomplete", "incompleto", "truncated", "cortado",
		"invalid", "inválido", "corrupt", "corrupto", "malformed", "malformado",
		"outdated", "desactualizado", "deprecated", "obsoleto",

		// Expresiones de frustración
		"wtf", "what the hell", "qué diablos", "no entiendo", "confusing",
		"confuso", "makes no sense", "no tiene sentido", "weird", "extraño",
		"strange", "raro", "unexpected", "inesperado",

		// Problemas de rendimiento
		"too slow", "muy lento", "takes forever", "tarda mucho",
		"performance", "rendimiento", "optimization", "optimización",

		// Problemas de seguridad
		"security", "seguridad", "vulnerability", "vulnerabilidad",
		"exploit", "hack", "breach", "violación", "leak", "filtración",
	}

	foundKeywords := []string{}
	severityScore := 0
	// Buscar palabras clave en el prompt actual y previo si aplica (para contexto).
	for _, keyword := range criticalKeywords {
		if strings.Contains(promptLower, keyword) {
			foundKeywords = append(foundKeywords, keyword)
			// Asignar severidad basada en la palabra clave
			keywordSeverity := getSeverityForKeyword(keyword)
			if keywordSeverity > severityScore {
				severityScore = keywordSeverity
			}
		}
	}

	// 2. Detección por patrones de corrección
	correctionPatterns := []string{
		// Patrones de referencia a código anterior
		`(?i)(este|el|this|that)\s+(código|code|script|programa|program)\s+(no|está|tiene|doesn't|isn't|has|contains)`,
		`(?i)(el|la|the)\s+(respuesta|response|solución|solution|código|code)\s+(anterior|previa|de\s+arriba|above|before|previous)\s+(no|está|tiene|doesn't|isn't|has)`,
		`(?i)(tu|your)\s+(respuesta|response|código|code|solución|solution)\s+(anterior|previa|pasada|last|previous)\s+(no|está|tiene|doesn't|isn't|has)`,

		// Patrones de solicitud de corrección
		`(?i)(puedes?|podrías?|can\s+you|could\s+you|please)\s+(corregir|arreglar|fix|repair|correct|solve|resolver)`,
		`(?i)(necesito|need|requiero|require)\s+(que\s+)?(corrijas|arregles|fix|repair|correct)`,
		`(?i)(ayúdame|help\s+me|asísteme|assist\s+me)\s+(a\s+)?(corregir|arreglar|fix|repair)`,

		// Patrones de identificación de errores
		`(?i)(hay|there\s+is|existe|exists?)\s+(un\s+|an?\s+)?(error|problema|issue|bug|falla)`,
		`(?i)(encontré|found|detecté|detected)\s+(un\s+|an?\s+)?(error|problema|issue|bug)`,
		`(?i)(tengo|have|me\s+sale|getting)\s+(un\s+|an?\s+)?(error|problema|issue)`,

		// Patrones de funcionamiento incorrecto
		`(?i)(esto|this|eso|that)\s+(no\s+funciona|doesn't\s+work|no\s+anda|isn't\s+working)`,
		`(?i)(no\s+me\s+funciona|it\s+doesn't\s+work|not\s+working)\s+(el|la|the)?\s*(código|code|script)?`,
		`(?i)(el\s+resultado|the\s+result|la\s+salida|the\s+output)\s+(está\s+mal|is\s+wrong|es\s+incorrecto|is\s+incorrect)`,

		// Patrones de problemas específicos
		`(?i)(no\s+compila|doesn't\s+compile|compilation\s+error|error\s+de\s+compilación)`,
		`(?i)(syntax\s+error|error\s+de\s+sintaxis|parse\s+error|error\s+de\s+parsing)`,
		`(?i)(runtime\s+error|error\s+en\s+tiempo\s+de\s+ejecución|execution\s+error)`,
		`(?i)(undefined|no\s+definido|not\s+defined|variable\s+no\s+declarada)`,

		// Patrones de comparación negativa
		`(?i)(esto\s+no\s+es|this\s+is\s+not|eso\s+no\s+es|that\s+is\s+not)\s+(lo\s+que|what)\s+(necesito|need|quiero|want)`,
		`(?i)(esperaba|expected|quería|wanted)\s+(algo\s+)?(diferente|different|distinto|otro|another)`,
		`(?i)(no\s+es\s+lo\s+que|not\s+what)\s+(pedí|asked\s+for|solicité|requested)`,

		// Patrones de frustración técnica
		`(?i)(por\s+qué|why)\s+(no\s+funciona|doesn't\s+work|falla|fails?)`,
		`(?i)(qué\s+está\s+mal|what's\s+wrong|qué\s+pasa|what's\s+happening)\s+(con|with)`,
		`(?i)(no\s+entiendo|don't\s+understand)\s+(por\s+qué|why)\s+(no|doesn't?)`,

		// Patrones de solicitud de revisión
		`(?i)(revisa|review|verifica|verify|checa|check)\s+(el|la|the)?\s*(código|code|script|solución|solution)`,
		`(?i)(mira|look\s+at|ve|see)\s+(si|if|whether)\s+(hay|there\s+is)\s+(algún|any|un|an?)\s+(error|problema|issue)`,

		// Patrones de problemas de rendimiento
		`(?i)(muy\s+lento|too\s+slow|demasiado\s+lento|extremely\s+slow)`,
		`(?i)(tarda\s+mucho|takes\s+too\s+long|demora\s+mucho|very\s+slow)`,
		`(?i)(no\s+responde|not\s+responding|se\s+cuelga|hangs?|freezes?)`,

		// Patrones de problemas de lógica
		`(?i)(la\s+lógica|the\s+logic)\s+(está\s+mal|is\s+wrong|es\s+incorrecta|is\s+incorrect)`,
		`(?i)(el\s+algoritmo|the\s+algorithm)\s+(no\s+funciona|doesn't\s+work|falla|fails?)`,
		`(?i)(el\s+flujo|the\s+flow)\s+(está\s+mal|is\s+wrong|es\s+incorrecto|is\s+incorrect)`,
	}

	for _, pattern := range correctionPatterns {
		if matched, _ := regexp.MatchString(pattern, currentPrompt); matched {
			foundKeywords = append(foundKeywords, "pattern_match")
			if severityScore < 3 {
				severityScore = 3
			}
			break
		}
	}

	// 3. Detección por contexto de código
	if previousResponse != "" && strings.Contains(previousResponse, "```") {
		// La respuesta anterior contenía código
		codeRelatedIssues := []string{
			// Errores de compilación
			"no compila", "doesn't compile", "compilation error", "compile error",
			"build error", "error de compilación", "failed to compile", "compilation failed",
			"build failed", "make error", "cmake error",

			// Errores de sintaxis
			"syntax error", "error de sintaxis", "parse error", "parsing error",
			"invalid syntax", "sintaxis inválida", "malformed syntax", "syntax issue",
			"unexpected token", "token inesperado", "missing bracket", "falta paréntesis",
			"missing brace", "falta llave", "missing parenthesis", "falta corchete",

			// Errores de indentación y formato
			"indentation", "indentación", "indent error", "error de indentación",
			"whitespace error", "spacing error", "tab error", "space error",
			"formatting error", "error de formato", "code formatting",

			// Errores de puntuación
			"missing semicolon", "falta punto y coma", "semicolon error",
			"missing comma", "falta coma", "comma error", "punctuation error",
			"missing colon", "falta dos puntos", "colon error",

			// Errores de variables y referencias
			"undefined variable", "variable no definida", "variable not defined",
			"undeclared variable", "variable no declarada", "unknown variable",
			"variable desconocida", "unresolved reference", "referencia no resuelta",
			"name error", "error de nombre", "identifier error",

			// Errores de tipos
			"type error", "error de tipo", "type mismatch", "tipos no coinciden",
			"wrong type", "tipo incorrecto", "invalid type", "tipo inválido",
			"type conversion", "conversión de tipo", "casting error",
			"incompatible types", "tipos incompatibles",

			// Errores de memoria y punteros
			"null pointer", "puntero nulo", "nullptr", "null reference",
			"referencia nula", "memory error", "error de memoria",
			"segmentation fault", "segfault", "access violation",
			"violación de acceso", "buffer overflow", "desbordamiento de buffer",
			"memory leak", "fuga de memoria", "dangling pointer", "puntero colgante",

			// Errores de ejecución
			"runtime error", "error en tiempo de ejecución", "execution error",
			"error de ejecución", "stack overflow", "desbordamiento de pila",
			"infinite loop", "bucle infinito", "recursion error", "error de recursión",
			"timeout error", "error de tiempo", "deadlock", "bloqueo mutuo",

			// Errores de importación y módulos
			"import error", "error de importación", "module not found",
			"módulo no encontrado", "package not found", "paquete no encontrado",
			"dependency error", "error de dependencia", "library error",
			"error de librería", "missing import", "falta importación",

			// Errores de funciones y métodos
			"function not defined", "función no definida", "method not found",
			"método no encontrado", "missing function", "falta función",
			"wrong parameters", "parámetros incorrectos", "parameter error",
			"error de parámetros", "argument error", "error de argumentos",
			"return error", "error de retorno", "missing return", "falta return",

			// Errores de lógica
			"logic error", "error de lógica", "algorithm error", "error de algoritmo",
			"wrong logic", "lógica incorrecta", "flow error", "error de flujo",
			"condition error", "error de condición", "loop error", "error de bucle",

			// Errores de base de datos
			"database error", "error de base de datos", "sql error", "query error",
			"error de consulta", "connection error", "error de conexión",
			"table not found", "tabla no encontrada", "column error",

			// Errores de red y API
			"network error", "error de red", "connection refused", "conexión rechazada",
			"timeout", "tiempo agotado", "api error", "endpoint error",
			"http error", "status code", "código de estado", "request failed",

			// Errores de archivos y sistema
			"file not found", "archivo no encontrado", "permission denied",
			"permiso denegado", "access denied", "acceso denegado",
			"path error", "error de ruta", "directory error", "error de directorio",
			"disk error", "error de disco", "io error", "error de entrada/salida",

			// Errores de configuración
			"config error", "error de configuración", "environment error",
			"error de entorno", "variable not set", "variable no establecida",
			"missing config", "falta configuración", "setup error", "error de configuración",

			// Errores de rendimiento
			"performance issue", "problema de rendimiento", "slow execution",
			"ejecución lenta", "memory usage", "uso de memoria", "cpu usage",
			"optimization needed", "necesita optimización",
		}

		// Buscar problemas relacionados con código
		maxSeverity := 0
		for _, issue := range codeRelatedIssues {
			if strings.Contains(promptLower, issue) {
				foundKeywords = append(foundKeywords, issue)
				// Asignar severidad específica para problemas de código
				issueSeverity := getCodeIssueSeverity(issue)
				if issueSeverity > maxSeverity {
					maxSeverity = issueSeverity
				}
			}
		}

		// Actualizar severidad si encontramos problemas de código
		if maxSeverity > severityScore {
			severityScore = maxSeverity
		}
	}

	// 4. Calcular confianza y determinar si hay problema
	if len(foundKeywords) > 0 {
		detection.HasProblem = true
		detection.Keywords = foundKeywords
		detection.SeverityScore = severityScore
		detection.DetectionMethod = "keywords"

		// Calcular confianza basada en número de keywords y severidad
		baseConfidence := float64(len(foundKeywords)) * 0.3
		severityBonus := float64(severityScore) * 0.1
		detection.Confidence = baseConfidence + severityBonus

		if detection.Confidence > 1.0 {
			detection.Confidence = 1.0
		}

		// Sugerir acción basada en severidad
		switch {
		case severityScore >= 4:
			detection.SuggestedAction = "invalidate_immediately"
		case severityScore >= 2:
			detection.SuggestedAction = "mark_for_review"
		default:
			detection.SuggestedAction = "log_feedback"
		}

		fm.logger.Debug("Problema detectado en cache",
			zap.Strings("keywords", foundKeywords),
			zap.Int("severity", severityScore),
			zap.Float64("confidence", detection.Confidence),
			zap.String("action", detection.SuggestedAction))
	}

	return detection
}

// ReportProblem registra un problema detectado en la base de datos
func (fm *FeedbackManager) ReportProblem(ctx context.Context, cacheKey, userID, sessionID string, detection *ProblemDetection, originalPrompt, problemPrompt string) error {
	if !detection.HasProblem {
		return nil
	}

	// Insertar feedback en la base de datos
	query := `
		INSERT INTO cache_feedback (
			cache_key, user_id, session_id, problem_detected, 
			detection_method, problem_keywords, original_prompt, 
			problem_prompt, severity_score
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	keywordsStr := strings.Join(detection.Keywords, ",")

	_, err := fm.db.ExecContext(ctx, query,
		cacheKey, userID, sessionID, detection.HasProblem,
		detection.DetectionMethod, keywordsStr, originalPrompt,
		problemPrompt, detection.SeverityScore)

	if err != nil {
		fm.logger.Error("Error al registrar problema de cache",
			zap.String("cache_key", cacheKey),
			zap.Error(err))
		return fmt.Errorf("failed to report cache problem: %w", err)
	}

	// Si es crítico, invalidar cache inmediatamente
	if detection.SuggestedAction == "invalidate_immediately" {
		if err := fm.InvalidateCache(ctx, cacheKey); err != nil {
			fm.logger.Warn("Error al invalidar cache crítico",
				zap.String("cache_key", cacheKey),
				zap.Error(err))
		}
	}

	fm.logger.Info("Problema de cache reportado",
		zap.String("cache_key", cacheKey),
		zap.String("method", detection.DetectionMethod),
		zap.Int("severity", detection.SeverityScore),
		zap.String("action", detection.SuggestedAction))

	return nil
}

// InvalidateCache invalida una clave de cache específica
func (fm *FeedbackManager) InvalidateCache(ctx context.Context, cacheKey string) error {
	// Eliminar de Redis
	if err := fm.redis.Delete(ctx, cacheKey); err != nil {
		return fmt.Errorf("failed to delete from Redis: %w", err)
	}

	// Marcar como blacklisted en la base de datos
	query := `
		INSERT INTO cache_stats (cache_key, is_blacklisted, updated_at)
		VALUES ($1, true, NOW())
		ON CONFLICT (cache_key)
		DO UPDATE SET is_blacklisted = true, updated_at = NOW()
	`

	_, err := fm.db.ExecContext(ctx, query, cacheKey)
	if err != nil {
		return fmt.Errorf("failed to blacklist cache key: %w", err)
	}

	fm.logger.Info("Cache invalidado",
		zap.String("cache_key", cacheKey))

	return nil
}

// IsBlacklisted verifica si una clave de cache está en la lista negra
func (fm *FeedbackManager) IsBlacklisted(ctx context.Context, cacheKey string) (bool, error) {
	query := `SELECT is_blacklisted FROM cache_stats WHERE cache_key = $1`

	var isBlacklisted bool
	err := fm.db.QueryRowContext(ctx, query, cacheKey).Scan(&isBlacklisted)

	if err == sql.ErrNoRows {
		return false, nil // No existe, no está blacklisted
	}

	if err != nil {
		return false, fmt.Errorf("failed to check blacklist status: %w", err)
	}

	return isBlacklisted, nil
}

// IncrementCacheUsage incrementa el contador de uso de cache
func (fm *FeedbackManager) IncrementCacheUsage(ctx context.Context, cacheKey string) error {
	query := `SELECT increment_cache_usage($1)`
	_, err := fm.db.ExecContext(ctx, query, cacheKey)

	if err != nil {
		fm.logger.Warn("Error al incrementar uso de cache",
			zap.String("cache_key", cacheKey),
			zap.Error(err))
		return err
	}

	return nil
}

// GetCacheStats obtiene estadísticas de una clave de cache
func (fm *FeedbackManager) GetCacheStats(ctx context.Context, cacheKey string) (*CacheStats, error) {
	query := `
		SELECT cache_key, total_uses, problem_reports, quality_score, 
		       is_blacklisted, last_problem_at, problem_rate_percent, quality_rating
		FROM cache_quality_report 
		WHERE cache_key = $1
	`

	stats := &CacheStats{}
	var lastProblemAt sql.NullTime

	err := fm.db.QueryRowContext(ctx, query, cacheKey).Scan(
		&stats.CacheKey, &stats.TotalUses, &stats.ProblemReports,
		&stats.QualityScore, &stats.IsBlacklisted, &lastProblemAt,
		&stats.ProblemRate, &stats.QualityRating)

	if err == sql.ErrNoRows {
		// Cache nuevo, estadísticas por defecto
		return &CacheStats{
			CacheKey:       cacheKey,
			TotalUses:      0,
			ProblemReports: 0,
			QualityScore:   1.0,
			IsBlacklisted:  false,
			ProblemRate:    0.0,
			QualityRating:  "EXCELLENT",
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	if lastProblemAt.Valid {
		stats.LastProblemAt = &lastProblemAt.Time
	}

	return stats, nil
}

// GetTopProblematicCaches obtiene las claves de cache con más problemas
func (fm *FeedbackManager) GetTopProblematicCaches(ctx context.Context, limit int) ([]*CacheStats, error) {
	query := `
		SELECT cache_key, total_uses, problem_reports, quality_score, 
		       is_blacklisted, last_problem_at, problem_rate_percent, quality_rating
		FROM cache_quality_report 
		WHERE problem_reports > 0
		ORDER BY problem_reports DESC, problem_rate_percent DESC
		LIMIT $1
	`

	rows, err := fm.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query problematic caches: %w", err)
	}
	defer rows.Close()

	var results []*CacheStats

	for rows.Next() {
		stats := &CacheStats{}
		var lastProblemAt sql.NullTime

		err := rows.Scan(
			&stats.CacheKey, &stats.TotalUses, &stats.ProblemReports,
			&stats.QualityScore, &stats.IsBlacklisted, &lastProblemAt,
			&stats.ProblemRate, &stats.QualityRating)

		if err != nil {
			return nil, fmt.Errorf("failed to scan cache stats: %w", err)
		}

		if lastProblemAt.Valid {
			stats.LastProblemAt = &lastProblemAt.Time
		}

		results = append(results, stats)
	}

	return results, nil
}

// CleanupOldFeedback limpia feedback antiguo (más de 30 días)
func (fm *FeedbackManager) CleanupOldFeedback(ctx context.Context) error {
	query := `DELETE FROM cache_feedback WHERE created_at < NOW() - INTERVAL '30 days'`

	result, err := fm.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to cleanup old feedback: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fm.logger.Info("Feedback antiguo limpiado",
		zap.Int64("rows_deleted", rowsAffected))

	return nil
}

// getSeverityForKeyword determina la severidad basada en la palabra clave
func getSeverityForKeyword(keyword string) int {
	// Severidad 5 - Crítico: Errores graves del sistema que impiden funcionamiento
	criticalErrors := []string{
		// Errores del sistema y crashes
		"error", "exception", "crash", "crashed", "hang", "hung", "freeze", "frozen",
		"fatal", "fatal error", "critical", "crítico", "emergency", "emergencia",
		"panic", "pánico", "abort", "abortar", "terminate", "terminar", "kill", "matar",
		"system failure", "falla del sistema", "system crash", "crash del sistema",
		"application crash", "crash de aplicación", "blue screen", "pantalla azul",

		// Errores de compilación críticos
		"no compila", "doesn't compile", "compilation error", "error de compilación",
		"compile error", "build error", "error de construcción", "build failed",
		"construcción fallida", "compilation failed", "compilación fallida",
		"fatal compilation error", "error fatal de compilación",

		// Errores de sintaxis graves
		"syntax error", "error de sintaxis", "parse error", "error de análisis",
		"parsing error", "parser error", "lexical error", "error léxico",
		"tokenization error", "error de tokenización", "malformed code", "código malformado",

		// Errores de memoria críticos
		"segmentation fault", "segfault", "access violation", "violación de acceso",
		"stack overflow", "desbordamiento de pila", "memory leak", "fuga de memoria",
		"buffer overflow", "desbordamiento de buffer", "heap overflow", "out of memory",
		"sin memoria", "memory exhausted", "memoria agotada", "null pointer", "puntero nulo",
		"nullptr", "double free", "use after free", "memory corruption", "corrupción de memoria",

		// Errores de ejecución críticos
		"runtime error", "error en tiempo de ejecución", "execution error", "error de ejecución",
		"unhandled exception", "excepción no manejada", "fatal exception", "excepción fatal",
		"core dump", "volcado de núcleo", "stack trace", "traza de pila",

		// Errores de seguridad críticos
		"security", "seguridad", "vulnerability", "vulnerabilidad", "exploit", "explotar",
		"hack", "hackear", "breach", "violación", "leak", "filtración", "data breach",
		"violación de datos", "security hole", "agujero de seguridad", "backdoor", "puerta trasera",
		"injection", "inyección", "xss", "csrf", "sql injection", "inyección sql",

		// Errores de acceso y permisos críticos
		"access denied", "acceso denegado", "permission denied", "permiso denegado",
		"unauthorized", "no autorizado", "forbidden", "prohibido", "authentication failed",
		"autenticación fallida", "authorization failed", "autorización fallida",

		// Errores de red críticos
		"connection refused", "conexión rechazada", "network failure", "falla de red",
		"connection lost", "conexión perdida", "network unreachable", "red inalcanzable",
		"dns failure", "falla de dns", "ssl error", "error ssl", "certificate error",
		"error de certificado", "handshake failed", "handshake fallido",
	}

	// Severidad 4 - Alto: Problemas graves de funcionamiento
	highSeverity := []string{
		// Problemas de funcionamiento general
		"no funciona", "doesn't work", "not working", "broken", "roto", "dañado",
		"no responde", "not responding", "unresponsive", "stuck", "atascado",
		"fail", "failed", "failure", "falla", "fallo", "bug", "error", "problema",
		"issue", "defect", "defecto", "malfunction", "mal funcionamiento",

		// Problemas de tiempo y rendimiento graves
		"timeout", "time out", "tiempo agotado", "expired", "expirado", "slow",
		"lento", "very slow", "muy lento", "extremely slow", "extremadamente lento",
		"takes forever", "tarda una eternidad", "never finishes", "nunca termina",
		"infinite wait", "espera infinita", "blocking", "bloqueando", "deadlock",
		"bloqueo mutuo", "race condition", "condición de carrera",

		// Problemas de datos y corrupción
		"corrupt", "corrupto", "corrupted", "corrompido", "malformed", "malformado",
		"invalid data", "datos inválidos", "data corruption", "corrupción de datos",
		"data loss", "pérdida de datos", "missing data", "datos faltantes",
		"inconsistent data", "datos inconsistentes", "data integrity", "integridad de datos",

		// Expresiones de frustración alta
		"wtf", "what the fuck", "what the hell", "qué diablos", "qué carajo",
		"this is bullshit", "esto es una mierda", "piece of shit", "pedazo de mierda",
		"fucking broken", "jodidamente roto", "completely broken", "completamente roto",
		"total disaster", "desastre total", "nightmare", "pesadilla",

		// Problemas de lógica graves
		"wrong result", "resultado incorrecto", "incorrect output", "salida incorrecta",
		"logic error", "error de lógica", "algorithm error", "error de algoritmo",
		"calculation error", "error de cálculo", "computation error", "error de cómputo",
		"wrong calculation", "cálculo incorrecto", "mathematical error", "error matemático",
	}

	// Severidad 3 - Medio: Problemas de calidad y usabilidad
	mediumSeverity := []string{
		// Problemas de calidad general
		"incorrecto", "wrong", "está mal", "malo", "bad", "terrible", "horrible",
		"awful", "aweful", "useless", "inútil", "garbage", "basura", "trash", "desperdicio",
		"nonsense", "sin sentido", "absurd", "absurdo", "ridiculous", "ridículo",
		"stupid", "estúpido", "dumb", "tonto", "silly", "bobo", "pointless", "sin sentido",

		// Problemas de completitud
		"missing", "falta", "incomplete", "incompleto", "partial", "parcial",
		"truncated", "cortado", "cut off", "cortado", "unfinished", "sin terminar",
		"half done", "a medias", "not complete", "no completo", "lacking", "carente",
		"insufficient", "insuficiente", "inadequate", "inadecuado",

		// Problemas de validez
		"invalid", "inválido", "not valid", "no válido", "illegitimate", "ilegítimo",
		"unauthorized", "no autorizado", "improper", "impropio", "inappropriate", "inapropiado",
		"unsuitable", "inadecuado", "unacceptable", "inaceptable",

		// Problemas de actualización
		"outdated", "desactualizado", "old", "viejo", "deprecated", "obsoleto",
		"legacy", "heredado", "ancient", "antiguo", "obsolete", "obsoleto",
		"out of date", "fuera de fecha", "expired", "expirado", "stale", "obsoleto",

		// Problemas de comprensión
		"confusing", "confuso", "unclear", "poco claro", "ambiguous", "ambiguo",
		"makes no sense", "no tiene sentido", "doesn't make sense", "no tiene sentido",
		"hard to understand", "difícil de entender", "complicated", "complicado",
		"complex", "complejo", "convoluted", "enrevesado",

		// Problemas de expectativa
		"weird", "extraño", "strange", "raro", "odd", "extraño", "unusual", "inusual",
		"unexpected", "inesperado", "surprising", "sorprendente", "bizarre", "bizarro",
		"abnormal", "anormal", "irregular", "irregular", "anomalous", "anómalo",

		// Problemas de diseño
		"poor design", "mal diseño", "bad design", "diseño malo", "ugly", "feo",
		"messy", "desordenado", "cluttered", "abarrotado", "inconsistent", "inconsistente",
		"unprofessional", "poco profesional", "amateurish", "amateur",
	}

	// Severidad 2 - Bajo-Medio: Solicitudes de ayuda y mejora
	lowMediumSeverity := []string{
		// Solicitudes de corrección
		"corrige", "arregla", "fix", "repair", "solve", "resuelve", "correct", "corregir",
		"improve", "mejorar", "enhance", "mejorar", "update", "actualizar", "modify", "modificar",
		"change", "cambiar", "adjust", "ajustar", "tweak", "ajustar", "refine", "refinar",

		// Solicitudes de ayuda
		"help", "ayuda", "assist", "asistir", "support", "soporte", "guide", "guiar",
		"please help", "por favor ayuda", "need help", "necesito ayuda", "can you help",
		"puedes ayudar", "help me", "ayúdame", "assistance needed", "necesito asistencia",
		"please fix", "por favor arregla", "can you fix", "puedes arreglar",
		"how to fix", "cómo arreglar", "how do I fix", "cómo arreglo",

		// Problemas de rendimiento menores
		"slow", "lento", "too slow", "muy lento", "sluggish", "lento", "laggy", "con lag",
		"takes time", "toma tiempo", "takes a while", "toma un rato", "not fast", "no rápido",
		"could be faster", "podría ser más rápido", "performance", "rendimiento",
		"optimization", "optimización", "speed up", "acelerar", "faster", "más rápido",

		// Sugerencias de mejora
		"suggestion", "sugerencia", "recommend", "recomendar", "propose", "proponer",
		"idea", "idea", "improvement", "mejora", "enhancement", "mejora", "feature request",
		"solicitud de característica", "would be nice", "estaría bien", "consider", "considerar",
		"maybe", "tal vez", "perhaps", "quizás", "possibly", "posiblemente",

		// Preguntas y dudas
		"question", "pregunta", "doubt", "duda", "wondering", "preguntándome",
		"curious", "curioso", "how", "cómo", "why", "por qué", "what", "qué",
		"when", "cuándo", "where", "dónde", "which", "cuál", "clarification", "aclaración",
		"explanation", "explicación", "understand", "entender", "confused", "confundido",
	}

	// Verificar en qué categoría está la palabra clave
	for _, critical := range criticalErrors {
		if keyword == critical {
			return 5
		}
	}

	for _, high := range highSeverity {
		if keyword == high {
			return 4
		}
	}

	for _, medium := range mediumSeverity {
		if keyword == medium {
			return 3
		}
	}

	for _, lowMed := range lowMediumSeverity {
		if keyword == lowMed {
			return 2
		}
	}

	// Por defecto, severidad baja
	return 1
}

// getCodeIssueSeverity determina la severidad específica para problemas de código
func getCodeIssueSeverity(issue string) int {
	// Severidad 5 - Crítico: Errores que impiden la ejecución completamente
	criticalCodeIssues := []string{
		// Errores de compilación críticos
		"no compila", "doesn't compile", "compilation error", "compile error",
		"build error", "error de compilación", "failed to compile", "compilation failed",
		"build failed", "make error", "cmake error", "linker error", "error de enlazado",
		"fatal error", "error fatal", "critical error", "error crítico",

		// Errores de sintaxis graves
		"syntax error", "error de sintaxis", "parse error", "parsing error",
		"parser error", "lexical error", "error léxico", "tokenization error",
		"malformed code", "código malformado", "invalid code", "código inválido",

		// Errores de memoria críticos
		"segmentation fault", "segfault", "access violation", "violación de acceso",
		"null pointer", "puntero nulo", "nullptr", "null reference", "referencia nula",
		"stack overflow", "desbordamiento de pila", "memory leak", "fuga de memoria",
		"buffer overflow", "desbordamiento de buffer", "heap overflow", "double free",
		"use after free", "memory corruption", "corrupción de memoria", "out of memory",
		"sin memoria", "memory exhausted", "memoria agotada",

		// Errores de ejecución críticos
		"runtime error", "error en tiempo de ejecución", "execution error", "error de ejecución",
		"fatal exception", "excepción fatal", "unhandled exception", "excepción no manejada",
		"system crash", "crash del sistema", "application crash", "crash de aplicación",
		"core dump", "volcado de núcleo", "abort", "abortar", "terminate", "terminar",

		// Errores de concurrencia críticos
		"deadlock", "bloqueo mutuo", "race condition", "condición de carrera",
		"thread deadlock", "bloqueo de hilo", "mutex deadlock", "semaphore error",
		"synchronization error", "error de sincronización", "concurrent access error",

		// Errores de recursión
		"infinite loop", "bucle infinito", "recursion error", "error de recursión",
		"maximum recursion depth", "profundidad máxima de recursión", "stack exhausted",
		"pila agotada", "call stack overflow", "desbordamiento de pila de llamadas",

		// Errores de sistema críticos
		"system error", "error del sistema", "kernel panic", "pánico del kernel",
		"blue screen", "pantalla azul", "system halt", "parada del sistema",
		"hardware error", "error de hardware", "device error", "error de dispositivo",
	}

	// Severidad 4 - Alto: Errores de lógica y funcionalidad que afectan el comportamiento
	highCodeIssues := []string{
		// Errores de lógica
		"logic error", "error de lógica", "algorithm error", "error de algoritmo",
		"wrong logic", "lógica incorrecta", "flow error", "error de flujo",
		"control flow error", "error de flujo de control", "branch error", "error de rama",
		"condition error", "error de condición", "loop error", "error de bucle",
		"iteration error", "error de iteración", "sequence error", "error de secuencia",

		// Errores de definición y referencias
		"function not defined", "función no definida", "method not found", "método no encontrado",
		"undefined variable", "variable no definida", "variable not defined", "variable no declarada",
		"undeclared variable", "unknown variable", "variable desconocida", "unresolved symbol",
		"símbolo no resuelto", "unresolved reference", "referencia no resuelta", "missing declaration",
		"declaración faltante", "name error", "error de nombre", "identifier error", "error de identificador",
		"scope error", "error de ámbito", "namespace error", "error de espacio de nombres",

		// Errores de tipos
		"type error", "error de tipo", "type mismatch", "tipos no coinciden", "wrong type",
		"tipo incorrecto", "invalid type", "tipo inválido", "type conversion error",
		"error de conversión de tipo", "casting error", "error de casting", "incompatible types",
		"tipos incompatibles", "type checking error", "error de verificación de tipos",
		"generic type error", "error de tipo genérico", "template error", "error de plantilla",

		// Errores de importación y módulos
		"import error", "error de importación", "module not found", "módulo no encontrado",
		"package not found", "paquete no encontrado", "library not found", "librería no encontrada",
		"dependency error", "error de dependencia", "missing dependency", "dependencia faltante",
		"circular dependency", "dependencia circular", "version conflict", "conflicto de versión",
		"library error", "error de librería", "framework error", "error de framework",

		// Errores de base de datos
		"database error", "error de base de datos", "sql error", "error sql", "query error",
		"error de consulta", "connection error", "error de conexión", "transaction error",
		"error de transacción", "constraint violation", "violación de restricción",
		"foreign key error", "error de clave foránea", "primary key error", "error de clave primaria",
		"table not found", "tabla no encontrada", "column error", "error de columna",
		"index error", "error de índice", "schema error", "error de esquema",

		// Errores de red y API
		"network error", "error de red", "connection refused", "conexión rechazada",
		"connection timeout", "tiempo de conexión agotado", "socket error", "error de socket",
		"api error", "error de api", "endpoint error", "error de endpoint", "http error",
		"error http", "rest error", "error rest", "web service error", "error de servicio web",
		"authentication error", "error de autenticación", "authorization error", "error de autorización",
		"ssl error", "error ssl", "certificate error", "error de certificado",

		// Errores de serialización y datos
		"serialization error", "error de serialización", "deserialization error", "error de deserialización",
		"json error", "error json", "xml error", "error xml", "yaml error", "error yaml",
		"data format error", "error de formato de datos", "encoding error", "error de codificación",
		"decoding error", "error de decodificación", "charset error", "error de conjunto de caracteres",
	}

	// Severidad 3 - Medio: Problemas de formato, estructura y estilo
	mediumCodeIssues := []string{
		// Errores de formato e indentación
		"indentation", "indentación", "indent error", "error de indentación", "formatting error",
		"error de formato", "code formatting", "formato de código", "whitespace error",
		"error de espacios en blanco", "spacing error", "error de espaciado", "tab error",
		"error de tabulación", "space error", "error de espacios", "line ending error",
		"error de final de línea", "encoding format error", "error de formato de codificación",

		// Errores de puntuación y sintaxis menor
		"missing semicolon", "falta punto y coma", "semicolon error", "error de punto y coma",
		"missing comma", "falta coma", "comma error", "error de coma", "punctuation error",
		"error de puntuación", "missing colon", "falta dos puntos", "colon error", "error de dos puntos",
		"missing bracket", "falta corchete", "bracket error", "error de corchete",
		"missing brace", "falta llave", "brace error", "error de llave", "missing parenthesis",
		"falta paréntesis", "parenthesis error", "error de paréntesis", "quote error", "error de comillas",
		"missing quote", "falta comilla", "unmatched delimiter", "delimitador no emparejado",

		// Errores de sintaxis menor
		"invalid syntax", "sintaxis inválida", "malformed syntax", "sintaxis malformada",
		"syntax issue", "problema de sintaxis", "unexpected token", "token inesperado",
		"unexpected character", "carácter inesperado", "invalid character", "carácter inválido",
		"reserved word error", "error de palabra reservada", "keyword error", "error de palabra clave",

		// Errores de parámetros y argumentos
		"parameter error", "error de parámetros", "argument error", "error de argumentos",
		"wrong parameters", "parámetros incorrectos", "parameter mismatch", "parámetros no coinciden",
		"missing parameter", "parámetro faltante", "extra parameter", "parámetro extra",
		"parameter type error", "error de tipo de parámetro", "argument count error",
		"error de cantidad de argumentos", "signature mismatch", "firma no coincide",

		// Errores de retorno y funciones
		"return error", "error de retorno", "missing return", "falta return", "return type error",
		"error de tipo de retorno", "void return error", "error de retorno void",
		"function signature error", "error de firma de función", "method signature error",
		"error de firma de método", "overload error", "error de sobrecarga",

		// Errores de archivos y rutas
		"file not found", "archivo no encontrado", "path error", "error de ruta",
		"directory error", "error de directorio", "folder error", "error de carpeta",
		"file path error", "error de ruta de archivo", "relative path error", "error de ruta relativa",
		"absolute path error", "error de ruta absoluta", "file extension error", "error de extensión de archivo",

		// Errores de permisos
		"permission denied", "permiso denegado", "access denied", "acceso denegado",
		"read permission error", "error de permiso de lectura", "write permission error",
		"error de permiso de escritura", "execute permission error", "error de permiso de ejecución",
		"file permission error", "error de permiso de archivo", "directory permission error",
		"error de permiso de directorio",
	}

	// Severidad 2 - Bajo-Medio: Problemas de rendimiento, configuración y advertencias
	lowMediumCodeIssues := []string{
		// Problemas de rendimiento
		"performance issue", "problema de rendimiento", "slow execution", "ejecución lenta",
		"memory usage", "uso de memoria", "cpu usage", "uso de cpu", "high memory usage",
		"alto uso de memoria", "memory consumption", "consumo de memoria", "cpu intensive",
		"intensivo en cpu", "slow query", "consulta lenta", "inefficient algorithm",
		"algoritmo ineficiente", "optimization needed", "necesita optimización",
		"performance bottleneck", "cuello de botella de rendimiento", "scalability issue",
		"problema de escalabilidad", "resource usage", "uso de recursos",

		// Problemas de configuración
		"config error", "error de configuración", "configuration error", "error de configuración",
		"environment error", "error de entorno", "setup error", "error de configuración",
		"initialization error", "error de inicialización", "startup error", "error de inicio",
		"variable not set", "variable no establecida", "missing config", "falta configuración",
		"config file error", "error de archivo de configuración", "settings error", "error de configuración",
		"property error", "error de propiedad", "option error", "error de opción",

		// Problemas de tiempo y timeout
		"timeout", "tiempo agotado", "timeout error", "error de tiempo", "time limit exceeded",
		"límite de tiempo excedido", "execution timeout", "tiempo de ejecución agotado",
		"connection timeout", "tiempo de conexión agotado", "read timeout", "tiempo de lectura agotado",
		"write timeout", "tiempo de escritura agotado", "response timeout", "tiempo de respuesta agotado",

		// Problemas de importación menor
		"missing import", "falta importación", "unused import", "importación no utilizada",
		"import warning", "advertencia de importación", "circular import", "importación circular",
		"import order", "orden de importación", "import style", "estilo de importación",

		// Problemas de funciones y métodos menores
		"missing function", "falta función", "unused function", "función no utilizada",
		"function warning", "advertencia de función", "method warning", "advertencia de método",
		"missing return", "falta return", "unreachable code", "código inalcanzable",
		"dead code", "código muerto", "unused variable", "variable no utilizada",
		"unused parameter", "parámetro no utilizado", "variable shadowing", "sombreado de variable",

		// Advertencias de estilo y convenciones
		"style warning", "advertencia de estilo", "naming convention", "convención de nombres",
		"code style", "estilo de código", "formatting warning", "advertencia de formato",
		"convention violation", "violación de convención", "best practice", "mejor práctica",
		"code smell", "mal olor de código", "refactoring needed", "necesita refactorización",

		// Problemas de documentación
		"missing documentation", "falta documentación", "missing comment", "falta comentario",
		"documentation error", "error de documentación", "comment error", "error de comentario",
		"missing docstring", "falta docstring", "incomplete documentation", "documentación incompleta",

		// Problemas de testing
		"test error", "error de prueba", "test failure", "falla de prueba", "assertion error",
		"error de aserción", "test timeout", "tiempo de prueba agotado", "mock error", "error de mock",
		"test setup error", "error de configuración de prueba", "test data error", "error de datos de prueba",
	}

	// Verificar en qué categoría está el problema de código
	for _, critical := range criticalCodeIssues {
		if issue == critical {
			return 5
		}
	}

	for _, high := range highCodeIssues {
		if issue == high {
			return 4
		}
	}

	for _, medium := range mediumCodeIssues {
		if issue == medium {
			return 3
		}
	}

	for _, lowMed := range lowMediumCodeIssues {
		if issue == lowMed {
			return 2
		}
	}

	// Por defecto, severidad baja para problemas de código
	return 1
}
