package metrics

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

type histogram struct {
	buckets []float64
	counts  []uint64
	count   uint64
	sum     float64
}

func newHistogram(buckets []float64) histogram {
	sorted := append([]float64(nil), buckets...)
	sort.Float64s(sorted)
	return histogram{buckets: sorted, counts: make([]uint64, len(sorted))}
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for index, upper := range h.buckets {
		if value <= upper {
			h.counts[index]++
		}
	}
}

type Metrics struct {
	mu       sync.Mutex
	counters map[string]uint64
	ttft     histogram
	duration histogram
	queue    histogram
}

func New() *Metrics {
	return &Metrics{
		counters: make(map[string]uint64),
		ttft:     newHistogram([]float64{0.05, 0.1, 0.2, 0.3, 0.5, 1, 2, 5}),
		duration: newHistogram([]float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}),
		queue:    newHistogram([]float64{0.001, 0.01, 0.05, 0.1, 0.25, 0.5, 1}),
	}
}

func (m *Metrics) Inc(name string) {
	m.mu.Lock()
	m.counters[name]++
	m.mu.Unlock()
}

func (m *Metrics) Add(name string, value uint64) {
	m.mu.Lock()
	m.counters[name] += value
	m.mu.Unlock()
}

func (m *Metrics) ObserveTTFT(seconds float64) {
	m.mu.Lock()
	m.ttft.observe(seconds)
	m.mu.Unlock()
}

func (m *Metrics) ObserveDuration(seconds float64) {
	m.mu.Lock()
	m.duration.observe(seconds)
	m.mu.Unlock()
}

func (m *Metrics) ObserveQueue(seconds float64) {
	m.mu.Lock()
	m.queue.observe(seconds)
	m.mu.Unlock()
}

func (m *Metrics) Counter(name string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[name]
}

func (m *Metrics) Render(w io.Writer, running, queuedRequests, queuedTokens int) {
	m.mu.Lock()
	counters := make(map[string]uint64, len(m.counters))
	for name, value := range m.counters {
		counters[name] = value
	}
	ttft := cloneHistogram(m.ttft)
	duration := cloneHistogram(m.duration)
	queue := cloneHistogram(m.queue)
	m.mu.Unlock()

	fmt.Fprintln(w, "# HELP sloserve_requests_total Requests handled by outcome.")
	fmt.Fprintln(w, "# TYPE sloserve_requests_total counter")
	for _, outcome := range []string{"completed", "shed", "canceled", "upstream_error"} {
		fmt.Fprintf(w, "sloserve_requests_total{outcome=%q} %d\n", outcome, counters["request_"+outcome])
	}
	fmt.Fprintln(w, "# HELP sloserve_retries_total Retries performed before the first token.")
	fmt.Fprintln(w, "# TYPE sloserve_retries_total counter")
	fmt.Fprintf(w, "sloserve_retries_total %d\n", counters["retry"])
	fmt.Fprintln(w, "# HELP sloserve_output_tokens_total Output tokens streamed to clients.")
	fmt.Fprintln(w, "# TYPE sloserve_output_tokens_total counter")
	fmt.Fprintf(w, "sloserve_output_tokens_total %d\n", counters["output_token"])
	fmt.Fprintln(w, "# HELP sloserve_goodput_requests_total Completed requests that met the configured TTFT target.")
	fmt.Fprintln(w, "# TYPE sloserve_goodput_requests_total counter")
	fmt.Fprintf(w, "sloserve_goodput_requests_total %d\n", counters["goodput"])

	fmt.Fprintln(w, "# HELP sloserve_running_requests Requests currently holding an inference slot.")
	fmt.Fprintln(w, "# TYPE sloserve_running_requests gauge")
	fmt.Fprintf(w, "sloserve_running_requests %d\n", running)
	fmt.Fprintln(w, "# HELP sloserve_queued_requests Requests waiting for an inference slot.")
	fmt.Fprintln(w, "# TYPE sloserve_queued_requests gauge")
	fmt.Fprintf(w, "sloserve_queued_requests %d\n", queuedRequests)
	fmt.Fprintln(w, "# HELP sloserve_queued_tokens Estimated input and output tokens waiting for capacity.")
	fmt.Fprintln(w, "# TYPE sloserve_queued_tokens gauge")
	fmt.Fprintf(w, "sloserve_queued_tokens %d\n", queuedTokens)

	renderHistogram(w, "sloserve_ttft_seconds", "Time from admission to the first output token.", ttft)
	renderHistogram(w, "sloserve_request_duration_seconds", "End-to-end request duration after admission.", duration)
	renderHistogram(w, "sloserve_queue_wait_seconds", "Time spent waiting for an inference slot.", queue)
}

func cloneHistogram(source histogram) histogram {
	return histogram{
		buckets: append([]float64(nil), source.buckets...),
		counts:  append([]uint64(nil), source.counts...),
		count:   source.count,
		sum:     source.sum,
	}
}

func renderHistogram(w io.Writer, name, help string, h histogram) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)
	for index, upper := range h.buckets {
		fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, fmt.Sprintf("%g", upper), h.counts[index])
	}
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", name, h.count)
	fmt.Fprintf(w, "%s_sum %g\n", name, h.sum)
	fmt.Fprintf(w, "%s_count %d\n", name, h.count)
}
