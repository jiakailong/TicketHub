package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

func TestProgramRepositorySearchesElasticsearchAndHydratesFromFallback(t *testing.T) {
	fallback := &fakeProgramRepository{
		programs: map[int64]program.Program{
			10001: {ID: 10001, Title: "TicketHub Live", City: "Shanghai", Place: "Arena", ShowTime: time.Unix(1, 0), Status: "ON_SALE"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tickethub_programs/_search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"id":10001}}]}}`))
	}))
	defer server.Close()

	repo := NewProgramRepository(fallback, []string{server.URL})
	items, err := repo.SearchPrograms(context.Background(), "live", "Shanghai", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 10001 {
		t.Fatalf("items = %+v", items)
	}
	if fallback.searchCalls != 0 {
		t.Fatalf("expected ES path to avoid fallback search, got %d calls", fallback.searchCalls)
	}
}

func TestProgramRepositoryFallsBackToMySQLSearchWhenElasticsearchFails(t *testing.T) {
	fallback := &fakeProgramRepository{
		searchResult: []program.Program{{ID: 20001, Title: "Fallback Show", City: "Beijing"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	repo := NewProgramRepository(fallback, []string{server.URL})
	items, err := repo.SearchPrograms(context.Background(), "show", "Beijing", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 20001 {
		t.Fatalf("items = %+v", items)
	}
	if fallback.searchCalls != 1 {
		t.Fatalf("fallback search calls = %d", fallback.searchCalls)
	}
}

func TestSearchBodyBuildsExpectedFilters(t *testing.T) {
	body := searchBody("live", "Shanghai", 2, 10)
	if body["from"] != 10 {
		t.Fatalf("from = %v", body["from"])
	}
	raw := stringify(body)
	if !strings.Contains(raw, "multi_match") || !strings.Contains(raw, "city.keyword") {
		t.Fatalf("unexpected body: %s", raw)
	}
}

func TestProgramRepositoryUsesSearchAfterCursor(t *testing.T) {
	fallback := &fakeProgramRepository{programs: map[int64]program.Program{
		10001: {ID: 10001, Title: "A"},
		10002: {ID: 10002, Title: "B"},
	}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if requests == 2 {
			if values, ok := body["search_after"].([]any); !ok || len(values) != 2 {
				t.Fatalf("search_after = %v", body["search_after"])
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"id":10001},"sort":["2027-01-01T00:00:00Z",10001]}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"id":10002},"sort":["2027-01-02T00:00:00Z",10002]}]}}`))
		}
	}))
	defer server.Close()
	repo := NewProgramRepository(fallback, []string{server.URL})
	_, cursor, err := repo.SearchProgramsAfter(context.Background(), "", "", "", 1)
	if err != nil || cursor == "" {
		t.Fatalf("cursor=%q error=%v", cursor, err)
	}
	items, _, err := repo.SearchProgramsAfter(context.Background(), "", "", cursor, 1)
	if err != nil || len(items) != 1 || items[0].ID != 10002 {
		t.Fatalf("items=%+v error=%v", items, err)
	}
}

type fakeProgramRepository struct {
	programs     map[int64]program.Program
	searchResult []program.Program
	searchCalls  int
}

func (r *fakeProgramRepository) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	r.searchCalls++
	return append([]program.Program(nil), r.searchResult...), nil
}

func (r *fakeProgramRepository) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	item, ok := r.programs[programID]
	if !ok {
		return program.Program{}, therrors.New(therrors.CodeNotFound, "program not found")
	}
	return item, nil
}

func (r *fakeProgramRepository) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	return nil, nil
}

func (r *fakeProgramRepository) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	return nil, nil
}

func stringify(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
