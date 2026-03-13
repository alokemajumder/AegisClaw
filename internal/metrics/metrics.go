package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "aegisclaw",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "aegisclaw",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "aegisclaw",
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being processed",
		},
	)

	dbPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "aegisclaw",
			Name:      "db_pool_connections",
			Help:      "Database connection pool stats",
		},
		[]string{"state"},
	)

	natsPublishedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "aegisclaw",
			Name:      "nats_messages_published_total",
			Help:      "Total NATS messages published",
		},
		[]string{"subject"},
	)

	natsConsumedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "aegisclaw",
			Name:      "nats_messages_consumed_total",
			Help:      "Total NATS messages consumed",
		},
		[]string{"consumer", "status"},
	)
)

// Middleware returns an HTTP middleware that records request metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		httpRequestsInFlight.Inc()

		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)

		httpRequestsInFlight.Dec()
		duration := time.Since(start).Seconds()

		// Normalize path to avoid high cardinality (collapse UUIDs)
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(ww.statusCode)

		httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordDBPoolStats records database connection pool statistics.
func RecordDBPoolStats(total, idle, inUse int) {
	dbPoolSize.WithLabelValues("total").Set(float64(total))
	dbPoolSize.WithLabelValues("idle").Set(float64(idle))
	dbPoolSize.WithLabelValues("in_use").Set(float64(inUse))
}

// RecordNATSPublish records a NATS publish event.
func RecordNATSPublish(subject string) {
	natsPublishedTotal.WithLabelValues(subject).Inc()
}

// RecordNATSConsume records a NATS consume event.
func RecordNATSConsume(consumer, status string) {
	natsConsumedTotal.WithLabelValues(consumer, status).Inc()
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// normalizePath collapses UUID segments to {id} to prevent high-cardinality labels.
func normalizePath(path string) string {
	// Simple approach: replace segments that look like UUIDs
	parts := splitPath(path)
	for i, p := range parts {
		if looksLikeUUID(p) {
			parts[i] = "{id}"
		}
	}
	result := "/" + joinPath(parts)
	return result
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func joinPath(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "/"
		}
		result += p
	}
	return result
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
