package observability

import (
	"context"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	operationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "a2a888",
		Subsystem: "operation",
		Name:      "requests_total",
		Help:      "Total number of tenant-scoped 888a2a operations.",
	}, []string{"organization_id", "surface", "operation", "status"})
	operationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "a2a888",
		Subsystem: "operation",
		Name:      "duration_seconds",
		Help:      "Duration of tenant-scoped 888a2a operations in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"organization_id", "surface", "operation"})
)

// RecordOperation records a bounded, tenant-scoped operation metric. Callers
// must pass a stable surface and operation name; arbitrary payloads and
// credentials must never become metric labels.
func RecordOperation(ctx context.Context, surface, operation, status string, duration time.Duration) {
	organizationID := boundedLabel(Tenant(ctx), "default")
	operationTotal.WithLabelValues(organizationID, boundedLabel(surface, "unknown"), boundedLabel(operation, "unknown"), boundedLabel(status, "unknown")).Inc()
	operationDuration.WithLabelValues(organizationID, boundedLabel(surface, "unknown"), boundedLabel(operation, "unknown")).Observe(duration.Seconds())
}

func boundedLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 128 || strings.ContainsAny(value, "\r\n") {
		return fallback
	}
	return value
}
