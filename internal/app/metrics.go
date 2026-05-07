package app

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type metrics struct {
	requestsTotal   atomic.Int64
	requestsFailed  atomic.Int64
	requestDuration atomic.Int64
	dbPoolActive    atomic.Int64
	dbPoolIdle      atomic.Int64
	dbPoolMax       atomic.Int64
}

func (m *metrics) recordRequest(duration time.Duration, failed bool) {
	m.requestsTotal.Add(1)
	m.requestDuration.Add(duration.Milliseconds())
	if failed {
		m.requestsFailed.Add(1)
	}
}

func (m *metrics) updatePoolStats(active, idle, max int) {
	m.dbPoolActive.Store(int64(active))
	m.dbPoolIdle.Store(int64(idle))
	m.dbPoolMax.Store(int64(max))
}

func (a *App) metricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		total := a.metrics.requestsTotal.Load()
		failed := a.metrics.requestsFailed.Load()
		duration := a.metrics.requestDuration.Load()
		active := a.metrics.dbPoolActive.Load()
		idle := a.metrics.dbPoolIdle.Load()
		max := a.metrics.dbPoolMax.Load()

		avgDuration := int64(0)
		if total > 0 {
			avgDuration = duration / total
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		lines := []string{
			"# HELP rentpulse_requests_total Total HTTP requests",
			"# TYPE rentpulse_requests_total counter",
			fmt.Sprintf("rentpulse_requests_total %d", total),
			"",
			"# HELP rentpulse_requests_failed Total failed HTTP requests",
			"# TYPE rentpulse_requests_failed counter",
			fmt.Sprintf("rentpulse_requests_failed %d", failed),
			"",
			"# HELP rentpulse_request_duration_ms_total Total request duration in milliseconds",
			"# TYPE rentpulse_request_duration_ms_total counter",
			fmt.Sprintf("rentpulse_request_duration_ms_total %d", duration),
			"",
			"# HELP rentpulse_request_duration_ms_avg Average request duration in milliseconds",
			"# TYPE rentpulse_request_duration_ms_avg gauge",
			fmt.Sprintf("rentpulse_request_duration_ms_avg %d", avgDuration),
			"",
			"# HELP rentpulse_db_pool_active Active database connections",
			"# TYPE rentpulse_db_pool_active gauge",
			fmt.Sprintf("rentpulse_db_pool_active %d", active),
			"",
			"# HELP rentpulse_db_pool_idle Idle database connections",
			"# TYPE rentpulse_db_pool_idle gauge",
			fmt.Sprintf("rentpulse_db_pool_idle %d", idle),
			"",
			"# HELP rentpulse_db_pool_max Maximum database connections",
			"# TYPE rentpulse_db_pool_max gauge",
			fmt.Sprintf("rentpulse_db_pool_max %d", max),
			"",
		}

		for _, line := range lines {
			if _, err := w.Write([]byte(line + "\n")); err != nil {
				a.logger.Error("failed to write metrics", "error", err)
				return
			}
		}
	})
}
