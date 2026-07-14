package observability

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricRegistryWritesPrometheusCountersAndHistograms(t *testing.T) {
	registry := newMetricRegistry()
	registry.service = "program-service"
	registry.addCounter("ticket_hub_test_total", map[string]string{"result": "ok"}, 2)
	registry.observeHistogram("ticket_hub_test_seconds", nil, 0.02)

	var output bytes.Buffer
	if err := registry.write(&output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `ticket_hub_test_total{result="ok",service="program-service"} 2`) {
		t.Fatalf("counter output missing: %s", text)
	}
	if !strings.Contains(text, `ticket_hub_test_seconds_count{service="program-service"} 1`) {
		t.Fatalf("histogram output missing: %s", text)
	}
}
