package observability

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var defaultRegistry = newMetricRegistry()

var histogramBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type metricRegistry struct {
	mu         sync.RWMutex
	service    string
	counters   map[string]*counterSeries
	gauges     map[string]*gaugeSeries
	histograms map[string]*histogramSeries
}

type counterSeries struct {
	name   string
	labels map[string]string
	value  float64
}

type gaugeSeries struct {
	name   string
	labels map[string]string
	value  float64
}

type histogramSeries struct {
	name    string
	labels  map[string]string
	buckets []uint64
	count   uint64
	sum     float64
}

func newMetricRegistry() *metricRegistry {
	return &metricRegistry{
		counters:   make(map[string]*counterSeries),
		gauges:     make(map[string]*gaugeSeries),
		histograms: make(map[string]*histogramSeries),
	}
}

func ConfigureMetrics(service string) {
	defaultRegistry.mu.Lock()
	defaultRegistry.service = strings.TrimSpace(service)
	defaultRegistry.mu.Unlock()
	SetGauge("tickethub_service_up", nil, 1)
}

func IncCounter(name string, labels map[string]string) {
	AddCounter(name, labels, 1)
}

func AddCounter(name string, labels map[string]string, delta float64) {
	defaultRegistry.addCounter(name, labels, delta)
}

func SetGauge(name string, labels map[string]string, value float64) {
	defaultRegistry.setGauge(name, labels, value)
}

func ObserveHistogram(name string, labels map[string]string, value float64) {
	defaultRegistry.observeHistogram(name, labels, value)
}

func WritePrometheus(w io.Writer) error {
	return defaultRegistry.write(w)
}

func (r *metricRegistry) addCounter(name string, labels map[string]string, delta float64) {
	labels = r.withService(labels)
	key := metricKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	series, ok := r.counters[key]
	if !ok {
		series = &counterSeries{name: name, labels: labels}
		r.counters[key] = series
	}
	series.value += delta
}

func (r *metricRegistry) setGauge(name string, labels map[string]string, value float64) {
	labels = r.withService(labels)
	key := metricKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	series, ok := r.gauges[key]
	if !ok {
		series = &gaugeSeries{name: name, labels: labels}
		r.gauges[key] = series
	}
	series.value = value
}

func (r *metricRegistry) observeHistogram(name string, labels map[string]string, value float64) {
	labels = r.withService(labels)
	key := metricKey(name, labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	series, ok := r.histograms[key]
	if !ok {
		series = &histogramSeries{name: name, labels: labels, buckets: make([]uint64, len(histogramBuckets))}
		r.histograms[key] = series
	}
	series.count++
	series.sum += value
	for index, upperBound := range histogramBuckets {
		if value <= upperBound {
			series.buckets[index]++
		}
	}
}

func (r *metricRegistry) withService(labels map[string]string) map[string]string {
	r.mu.RLock()
	service := r.service
	r.mu.RUnlock()
	result := cloneLabels(labels)
	if service != "" {
		result["service"] = service
	}
	return result
}

func (r *metricRegistry) write(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0, len(r.counters)+len(r.gauges)+len(r.histograms))
	for key := range r.counters {
		keys = append(keys, "counter\x00"+key)
	}
	for key := range r.gauges {
		keys = append(keys, "gauge\x00"+key)
	}
	for key := range r.histograms {
		keys = append(keys, "histogram\x00"+key)
	}
	sort.Strings(keys)
	declared := make(map[string]bool)
	for _, typedKey := range keys {
		parts := strings.SplitN(typedKey, "\x00", 2)
		kind, key := parts[0], parts[1]
		switch kind {
		case "counter":
			series := r.counters[key]
			if err := writeMetricDeclaration(w, declared, series.name, "counter"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s%s %s\n", series.name, formatLabels(series.labels), formatFloat(series.value)); err != nil {
				return err
			}
		case "gauge":
			series := r.gauges[key]
			if err := writeMetricDeclaration(w, declared, series.name, "gauge"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s%s %s\n", series.name, formatLabels(series.labels), formatFloat(series.value)); err != nil {
				return err
			}
		case "histogram":
			series := r.histograms[key]
			if err := writeMetricDeclaration(w, declared, series.name, "histogram"); err != nil {
				return err
			}
			for index, upperBound := range histogramBuckets {
				labels := cloneLabels(series.labels)
				labels["le"] = formatFloat(upperBound)
				if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", series.name, formatLabels(labels), series.buckets[index]); err != nil {
					return err
				}
			}
			labels := cloneLabels(series.labels)
			labels["le"] = "+Inf"
			if _, err := fmt.Fprintf(w, "%s_bucket%s %d\n", series.name, formatLabels(labels), series.count); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s_sum%s %s\n", series.name, formatLabels(series.labels), formatFloat(series.sum)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s_count%s %d\n", series.name, formatLabels(series.labels), series.count); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeMetricDeclaration(w io.Writer, declared map[string]bool, name string, kind string) error {
	if declared[name] {
		return nil
	}
	declared[name] = true
	_, err := fmt.Fprintf(w, "# HELP %s TicketHub runtime metric.\n# TYPE %s %s\n", name, name, kind)
	return err
}

func metricKey(name string, labels map[string]string) string {
	return name + formatLabels(labels)
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"=\""+escapeLabelValue(labels[key])+"\"")
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func cloneLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		result[key] = value
	}
	return result
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
