package elasticsearch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tickethub/app/program-service/internal/domain/program"
)

func TestProgramIndexerCreatesIndexAndBulkUpserts(t *testing.T) {
	created := false
	bulkBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/programs-test":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut && r.URL.Path == "/programs-test":
			created = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/_bulk":
			data, _ := io.ReadAll(r.Body)
			bulkBody = string(data)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errors":false,"items":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	indexer := NewProgramIndexer([]string{server.URL}, "programs-test")
	if err := indexer.EnsureIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := indexer.UpsertPrograms(context.Background(), []program.Program{
		{ID: 1001, Title: "TicketHub Live", City: "Shanghai", ShowTime: time.Unix(1, 0), Status: "ON_SALE"},
	}); err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected index creation")
	}
	if !strings.Contains(bulkBody, `"_id":"1001"`) || !strings.Contains(bulkBody, `"title":"TicketHub Live"`) {
		t.Fatalf("unexpected bulk body: %s", bulkBody)
	}
}
