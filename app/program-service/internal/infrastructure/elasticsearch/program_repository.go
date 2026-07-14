package elasticsearch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

func (r ProgramRepository) SearchProgramsAfter(ctx context.Context, keyword string, city string, cursor string, pageSize int) ([]program.Program, string, error) {
	if len(r.addresses) == 0 {
		items, err := r.fallback.SearchPrograms(ctx, keyword, city, 1, pageSize)
		return items, "", err
	}
	body := searchBody(keyword, city, 1, pageSize)
	delete(body, "from")
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", therrors.New(therrors.CodeInvalidArgument, "invalid program page cursor")
		}
		var values []any
		if err := json.Unmarshal(decoded, &values); err != nil || len(values) != 2 {
			return nil, "", therrors.New(therrors.CodeInvalidArgument, "invalid program page cursor")
		}
		body["search_after"] = values
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	var lastErr error
	for _, address := range r.addresses {
		ids, next, queryErr := r.queryAddressPage(ctx, address, encoded)
		if queryErr == nil {
			items, hydrateErr := r.hydrate(ctx, ids)
			return items, next, hydrateErr
		}
		lastErr = queryErr
	}
	items, fallbackErr := r.fallback.SearchPrograms(ctx, keyword, city, 1, pageSize)
	if fallbackErr != nil {
		return nil, "", lastErr
	}
	return items, "", nil
}

func (r ProgramRepository) SuggestPrograms(ctx context.Context, prefix string, limit int) ([]string, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	body, err := json.Marshal(map[string]any{
		"_source": false,
		"suggest": map[string]any{
			"program-title": map[string]any{
				"prefix":     prefix,
				"completion": map[string]any{"field": "title_suggest", "size": limit, "skip_duplicates": true, "fuzzy": map[string]any{"fuzziness": "AUTO"}},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, address := range r.addresses {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/_search", address, r.index), bytes.NewReader(body))
		if requestErr != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, requestErr := r.client.Do(req)
		if requestErr != nil {
			continue
		}
		var payload struct {
			Suggest map[string][]struct {
				Options []struct {
					Text string `json:"text"`
				} `json:"options"`
			} `json:"suggest"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if decodeErr == nil && resp.StatusCode < http.StatusMultipleChoices {
			entries := payload.Suggest["program-title"]
			if len(entries) == 0 {
				return []string{}, nil
			}
			result := make([]string, 0, len(entries[0].Options))
			for _, option := range entries[0].Options {
				result = append(result, option.Text)
			}
			return result, nil
		}
	}
	items, err := r.fallback.SearchPrograms(ctx, prefix, "", 1, limit)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Title)
	}
	return result, nil
}

const defaultProgramIndex = "tickethub_programs"

type fallbackRepository interface {
	SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error)
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error)
}

type ProgramRepository struct {
	fallback  fallbackRepository
	addresses []string
	index     string
	client    *http.Client
}

func NewProgramRepository(fallback fallbackRepository, addresses []string) ProgramRepository {
	return NewProgramRepositoryWithIndex(fallback, addresses, defaultProgramIndex)
}

func NewProgramRepositoryWithIndex(fallback fallbackRepository, addresses []string, index string) ProgramRepository {
	cleaned := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if value := strings.TrimRight(strings.TrimSpace(address), "/"); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if strings.TrimSpace(index) == "" {
		index = defaultProgramIndex
	}
	return ProgramRepository{
		fallback:  fallback,
		addresses: cleaned,
		index:     strings.TrimSpace(index),
		client:    &http.Client{Timeout: 2 * time.Second},
	}
}

func (r ProgramRepository) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	if len(r.addresses) == 0 {
		return r.fallback.SearchPrograms(ctx, keyword, city, page, pageSize)
	}
	ids, err := r.searchProgramIDs(ctx, keyword, city, page, pageSize)
	if err != nil {
		return r.fallback.SearchPrograms(ctx, keyword, city, page, pageSize)
	}
	if len(ids) == 0 {
		return []program.Program{}, nil
	}
	return r.hydrate(ctx, ids)
}

func (r ProgramRepository) hydrate(ctx context.Context, ids []int64) ([]program.Program, error) {
	if batch, ok := r.fallback.(interface {
		FindProgramsByIDs(context.Context, []int64) ([]program.Program, error)
	}); ok {
		return batch.FindProgramsByIDs(ctx, ids)
	}
	items := make([]program.Program, 0, len(ids))
	for _, id := range ids {
		item, findErr := r.fallback.FindProgram(ctx, id)
		if therrors.IsCode(findErr, therrors.CodeNotFound) {
			continue
		}
		if findErr != nil {
			return nil, findErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (r ProgramRepository) MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error) {
	if batch, ok := r.fallback.(interface {
		MinPricesByProgramIDs(context.Context, []int64) (map[int64]int64, error)
	}); ok {
		return batch.MinPricesByProgramIDs(ctx, programIDs)
	}
	result := make(map[int64]int64, len(programIDs))
	for _, programID := range programIDs {
		categories, err := r.fallback.ListTicketCategories(ctx, programID)
		if err != nil {
			return nil, err
		}
		for _, category := range categories {
			if result[programID] == 0 || category.PriceCent < result[programID] {
				result[programID] = category.PriceCent
			}
		}
	}
	return result, nil
}

func (r ProgramRepository) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	return r.fallback.FindProgram(ctx, programID)
}

func (r ProgramRepository) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	return r.fallback.ListTicketCategories(ctx, programID)
}

func (r ProgramRepository) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	return r.fallback.ListSeats(ctx, programID, ticketCategoryID)
}

func (r ProgramRepository) searchProgramIDs(ctx context.Context, keyword string, city string, page int, pageSize int) ([]int64, error) {
	body, err := json.Marshal(searchBody(keyword, city, page, pageSize))
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, address := range r.addresses {
		ids, err := r.queryAddress(ctx, address, body)
		if err == nil {
			return ids, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (r ProgramRepository) queryAddress(ctx context.Context, address string, body []byte) ([]int64, error) {
	ids, _, err := r.queryAddressPage(ctx, address, body)
	return ids, err
}

func (r ProgramRepository) queryAddressPage(ctx context.Context, address string, body []byte) ([]int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/_search", address, r.index), bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("elasticsearch returned status %d", resp.StatusCode)
	}
	var payload searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", err
	}
	ids := make([]int64, 0, len(payload.Hits.Hits))
	for _, hit := range payload.Hits.Hits {
		id := hit.Source.ID
		if id == 0 && hit.ID != "" {
			parsed, err := strconv.ParseInt(hit.ID, 10, 64)
			if err == nil {
				id = parsed
			}
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	var next string
	if len(payload.Hits.Hits) > 0 {
		sortValues := payload.Hits.Hits[len(payload.Hits.Hits)-1].Sort
		if len(sortValues) == 2 {
			encoded, _ := json.Marshal(sortValues)
			next = base64.RawURLEncoding.EncodeToString(encoded)
		}
	}
	return ids, next, nil
}

type searchResponse struct {
	Hits struct {
		Hits []struct {
			ID     string `json:"_id"`
			Source struct {
				ID int64 `json:"id"`
			} `json:"_source"`
			Sort []any `json:"sort"`
		} `json:"hits"`
	} `json:"hits"`
}

func searchBody(keyword string, city string, page int, pageSize int) map[string]any {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	keyword = strings.TrimSpace(keyword)
	city = strings.TrimSpace(city)

	query := map[string]any{"match_all": map[string]any{}}
	if keyword != "" || city != "" {
		boolQuery := map[string]any{}
		if keyword != "" {
			boolQuery["must"] = []any{
				map[string]any{
					"multi_match": map[string]any{
						"query":         keyword,
						"fields":        []string{"title^5", "title.prefix^2", "city^2", "place"},
						"fuzziness":     "AUTO",
						"prefix_length": 1,
					},
				},
			}
		}
		if city != "" {
			boolQuery["filter"] = []any{
				map[string]any{
					"term": map[string]any{"city.keyword": city},
				},
			}
		}
		query = map[string]any{"bool": boolQuery}
	}

	return map[string]any{
		"from":             (page - 1) * pageSize,
		"size":             pageSize,
		"track_total_hits": false,
		"_source":          []string{"id"},
		"query":            query,
		"sort": []any{
			map[string]any{"show_time": map[string]any{"order": "asc"}},
			map[string]any{"id": map[string]any{"order": "asc"}},
		},
	}
}
