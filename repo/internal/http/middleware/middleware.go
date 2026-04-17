// Package middleware holds shared Gin middleware used by router.go.
package middleware

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/auth"
)

// RequestLogger is a minimal request/response logger for observability.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s %d %s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}

// metrics tracks a handful of counters surfaced on /api/metrics.
type metrics struct {
	requests atomic.Int64
	errors   atomic.Int64
}

var globalMetrics = &metrics{}

// Metrics returns the global request metrics counter.
func Metrics() (requests, errors int64) {
	return globalMetrics.requests.Load(), globalMetrics.errors.Load()
}

// Observability increments counters for every response.
func Observability() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		globalMetrics.requests.Add(1)
		if c.Writer.Status() >= 500 {
			globalMetrics.errors.Add(1)
		}
	}
}

// ContextKey types for auth.
type ContextKey string

const (
	UserKey ContextKey = "user"
)

// RequireAuth attaches the authenticated user to the Gin context and
// returns 401 otherwise.
func RequireAuth(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := auth.ExtractBearerToken(c.GetHeader("Authorization"))
		if tok == "" {
			tok = c.GetHeader("X-Api-Token")
		}
		if tok == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing token"})
			return
		}
		u, err := svc.Resolve(c.Request.Context(), tok)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid session"})
			return
		}
		c.Set(string(UserKey), u)
		c.Next()
	}
}
