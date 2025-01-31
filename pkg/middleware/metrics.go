package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

type gaugeOptions struct {
	RequestsInFlight prometheus.GaugeOpts
	RequestsTotal    prometheus.CounterOpts
	RequestLatency   prometheus.HistogramOpts
	RequestSize      prometheus.HistogramOpts
	ResponseSize     prometheus.HistogramOpts
}

func newOptions(subsystem string) gaugeOptions {
	sizeBuckets := prometheus.ExponentialBuckets(256, 4, 8)
	return gaugeOptions{
		RequestsInFlight: prometheus.GaugeOpts{
			Namespace: "piko",
			Subsystem: subsystem,
			Name:      "requests_in_flight",
			Help:      "Number of requests currently handled by this server.",
		},
		RequestsTotal: prometheus.CounterOpts{
			Namespace: "piko",
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total requests.",
		},
		RequestLatency: prometheus.HistogramOpts{
			Namespace: "piko",
			Subsystem: subsystem,
			Name:      "request_latency_seconds",
			Help:      "Request latency.",
			Buckets:   prometheus.DefBuckets,
		},
		RequestSize: prometheus.HistogramOpts{
			Namespace: "piko",
			Subsystem: subsystem,
			Name:      "request_size_bytes",
			Help:      "Request size",
			Buckets:   sizeBuckets,
		},
		ResponseSize: prometheus.HistogramOpts{
			Namespace: "piko",
			Subsystem: subsystem,
			Name:      "response_size_bytes",
			Help:      "Response size",
			Buckets:   sizeBuckets,
		},
	}
}

type LabeledMetrics struct {
	RequestsInFlight *prometheus.GaugeVec
	RequestsTotal    *prometheus.CounterVec
	RequestLatency   *prometheus.HistogramVec
	RequestSize      *prometheus.HistogramVec
	ResponseSize     *prometheus.HistogramVec
}

type Metrics struct {
	RequestsInFlight prometheus.Gauge
	RequestsTotal    *prometheus.CounterVec
	RequestLatency   prometheus.ObserverVec
	RequestSize      prometheus.Observer
	ResponseSize     prometheus.Observer
}

func NewLabeledMetrics(registry *prometheus.Registry, subsystem string) *LabeledMetrics {
	if registry == nil {
		return nil
	}
	opts := newOptions(subsystem)
	requestsInFlight := prometheus.NewGaugeVec(
		opts.RequestsInFlight,
		[]string{"endpoint"},
	)
	requestsTotal := prometheus.NewCounterVec(
		opts.RequestsTotal,
		[]string{"endpoint", "status", "method"},
	)
	requestLatency := prometheus.NewHistogramVec(
		opts.RequestLatency,
		[]string{"endpoint", "status", "method"},
	)
	requestSize := prometheus.NewHistogramVec(opts.RequestSize, []string{"endpoint"})
	responseSize := prometheus.NewHistogramVec(opts.ResponseSize, []string{"endpoint"})
	registry.MustRegister(
		requestsInFlight,
		requestsTotal,
		requestLatency,
		requestSize,
		responseSize,
	)
	return &LabeledMetrics{
		RequestsInFlight: requestsInFlight,
		RequestsTotal:    requestsTotal,
		RequestLatency:   requestLatency,
		RequestSize:      requestSize,
		ResponseSize:     responseSize,
	}
}

func (lm *LabeledMetrics) Handler(endpointID string) gin.HandlerFunc {
	metrics := &Metrics{
		RequestsInFlight: lm.RequestsInFlight.WithLabelValues(endpointID),
		RequestsTotal:    lm.RequestsTotal.MustCurryWith(prometheus.Labels{"endpoint": endpointID}),
		RequestLatency:   lm.RequestLatency.MustCurryWith(prometheus.Labels{"endpoint": endpointID}),
		RequestSize:      lm.RequestSize.WithLabelValues(endpointID),
		ResponseSize:     lm.ResponseSize.WithLabelValues(endpointID),
	}
	return metrics.Handler()
}

func NewMetrics(registry *prometheus.Registry, subsystem string) *Metrics {
	if registry == nil {
		return nil
	}
	opts := newOptions(subsystem)
	requestsInFlight := prometheus.NewGauge(opts.RequestsInFlight)
	requestsTotal := prometheus.NewCounterVec(opts.RequestsTotal,
		[]string{"status", "method"},
	)
	requestLatency := prometheus.NewHistogramVec(
		opts.RequestLatency,
		[]string{"status", "method"},
	)
	requestSize := prometheus.NewHistogram(opts.RequestSize)
	responseSize := prometheus.NewHistogram(opts.ResponseSize)
	registry.MustRegister(
		requestsInFlight,
		requestsTotal,
		requestLatency,
		requestSize,
		responseSize,
	)
	return &Metrics{
		RequestsInFlight: requestsInFlight,
		RequestsTotal:    requestsTotal,
		RequestLatency:   requestLatency,
		RequestSize:      requestSize,
		ResponseSize:     responseSize,
	}
}

func (m *Metrics) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		m.RequestsInFlight.Inc()
		defer m.RequestsInFlight.Dec()

		start := time.Now()

		// Process request.
		c.Next()

		m.RequestsTotal.With(prometheus.Labels{
			"status": strconv.Itoa(c.Writer.Status()),
			"method": c.Request.Method,
		}).Inc()
		m.RequestLatency.With(prometheus.Labels{
			"status": strconv.Itoa(c.Writer.Status()),
			"method": c.Request.Method,
		}).Observe(float64(time.Since(start).Milliseconds()) / 1000)

		m.RequestSize.Observe(float64(computeApproximateRequestSize(c.Request)))
		m.ResponseSize.Observe(float64(c.Writer.Size()))
	}
}

func computeApproximateRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s += len(r.URL.String())
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}
