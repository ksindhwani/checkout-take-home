package platform

import (
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	paymentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payments_total",
		Help: "Total number of payments processed, by terminal status.",
	}, []string{"status"})

	bankCallDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "bank_call_duration_seconds",
		Help:    "Latency of calls to the acquiring bank.",
		Buckets: prometheus.DefBuckets,
	})

	bankCallErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bank_call_errors_total",
		Help: "Total number of failed (unreachable/timed out/non-200) calls to the acquiring bank.",
	})
)

// RecordPaymentOutcome increments the payment counter for a terminal status,
// typed so a label typo can't create a bogus metric series.
func RecordPaymentOutcome(status models.PaymentStatus) {
	paymentsTotal.With(prometheus.Labels{"status": string(status)}).Inc()
}

// ObserveBankCallDuration records how long a call to the bank took.
func ObserveBankCallDuration(d time.Duration) {
	bankCallDuration.Observe(d.Seconds())
}

// IncBankCallError increments the bank call error counter.
func IncBankCallError() {
	bankCallErrorsTotal.Inc()
}
