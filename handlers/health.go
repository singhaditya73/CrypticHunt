package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/labstack/echo/v4"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status            string            `json:"status"`
	Timestamp         string            `json:"timestamp"`
	Database          DatabaseHealth    `json:"database"`
	Server            ServerHealth      `json:"server"`
	SSEConnections    int               `json:"sse_connections"`
}

type DatabaseHealth struct {
	Status         string `json:"status"`
	OpenConnections int   `json:"open_connections"`
	IdleConnections int   `json:"idle_connections"`
	MaxOpenConns    int   `json:"max_open_conns"`
}

type ServerHealth struct {
	Goroutines     int     `json:"goroutines"`
	MemoryAllocMB  float64 `json:"memory_alloc_mb"`
	MemorySysMB    float64 `json:"memory_sys_mb"`
	NumGC          uint32  `json:"num_gc"`
}

// HealthCheckHandler returns server health and metrics
func (ah *AuthHandler) HealthCheckHandler(c echo.Context) error {
	// Check database connection
	dbStatus := "healthy"
	stats := ah.UserServices.GetDBStats()
	
	if err := ah.UserServices.PingDB(); err != nil {
		dbStatus = "unhealthy"
	}

	// Get memory statistics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Database: DatabaseHealth{
			Status:          dbStatus,
			OpenConnections: stats.OpenConnections,
			IdleConnections: stats.Idle,
			MaxOpenConns:    stats.MaxOpenConnections,
		},
		Server: ServerHealth{
			Goroutines:    runtime.NumGoroutine(),
			MemoryAllocMB: float64(m.Alloc) / 1024 / 1024,
			MemorySysMB:   float64(m.Sys) / 1024 / 1024,
			NumGC:         m.NumGC,
		},
		SSEConnections: ah.Broadcaster.GetClientCount(),
	}

	// Set overall status to unhealthy if database is down
	if dbStatus == "unhealthy" {
		response.Status = "degraded"
		return c.JSON(http.StatusServiceUnavailable, response)
	}

	return c.JSON(http.StatusOK, response)
}

// MetricsHandler returns more detailed metrics (optional, can be restricted)
func (ah *AuthHandler) MetricsHandler(c echo.Context) error {
	stats := ah.UserServices.GetDBStats()
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := map[string]interface{}{
		"database": map[string]interface{}{
			"open_connections":        stats.OpenConnections,
			"in_use":                  stats.InUse,
			"idle":                    stats.Idle,
			"wait_count":              stats.WaitCount,
			"wait_duration_ms":        stats.WaitDuration.Milliseconds(),
			"max_idle_closed":         stats.MaxIdleClosed,
			"max_idle_time_closed":    stats.MaxIdleTimeClosed,
			"max_lifetime_closed":     stats.MaxLifetimeClosed,
		},
		"runtime": map[string]interface{}{
			"goroutines":       runtime.NumGoroutine(),
			"memory_alloc_mb":  float64(m.Alloc) / 1024 / 1024,
			"memory_total_mb":  float64(m.TotalAlloc) / 1024 / 1024,
			"memory_sys_mb":    float64(m.Sys) / 1024 / 1024,
			"num_gc":           m.NumGC,
			"gc_pause_ns":      m.PauseNs[(m.NumGC+255)%256],
		},
		"sse": map[string]interface{}{
			"connected_clients": ah.Broadcaster.GetClientCount(),
		},
	}

	return c.JSON(http.StatusOK, metrics)
}
