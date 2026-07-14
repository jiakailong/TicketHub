package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tickethub/app/program-service/internal/domain/program"
)

type ProgramIndexer struct {
	addresses []string
	index     string
	alias     string
	client    *http.Client
}

func NewVersionedProgramIndexer(addresses []string, alias string, version string) ProgramIndexer {
	alias = strings.TrimSpace(alias)
	version = strings.Trim(strings.TrimSpace(version), "_")
	if version == "" {
		version = "v2"
	}
	indexer := NewProgramIndexer(addresses, alias+"_"+version)
	indexer.alias = alias
	return indexer
}

func NewProgramIndexer(addresses []string, index string) ProgramIndexer {
	cleaned := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if value := strings.TrimRight(strings.TrimSpace(address), "/"); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if strings.TrimSpace(index) == "" {
		index = defaultProgramIndex
	}
	return ProgramIndexer{
		addresses: cleaned,
		index:     strings.TrimSpace(index),
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (i ProgramIndexer) EnsureIndex(ctx context.Context) error {
	if len(i.addresses) == 0 {
		return fmt.Errorf("elasticsearch address is required")
	}
	var lastErr error
	for _, address := range i.addresses {
		if err := i.ensureIndexAt(ctx, address); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (i ProgramIndexer) ActivateIndex(ctx context.Context) error {
	if i.alias == "" || i.alias == i.index {
		return nil
	}
	var lastErr error
	for _, address := range i.addresses {
		if err := i.activateAt(ctx, address); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (i ProgramIndexer) activateAt(ctx context.Context, address string) error {
	actions := make([]any, 0, 2)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/_alias/%s", address, i.alias), nil)
	if err != nil {
		return err
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		var aliases map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&aliases); err != nil {
			resp.Body.Close()
			return err
		}
		for index := range aliases {
			if index != i.index {
				actions = append(actions, map[string]any{"remove": map[string]any{"index": index, "alias": i.alias}})
			}
		}
	} else if resp.StatusCode != http.StatusNotFound {
		resp.Body.Close()
		return fmt.Errorf("elasticsearch alias lookup returned status %d", resp.StatusCode)
	}
	resp.Body.Close()
	actions = append(actions, map[string]any{"add": map[string]any{"index": i.index, "alias": i.alias, "is_write_index": true}})
	body, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		return err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, address+"/_aliases", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elasticsearch alias activation returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

func (i ProgramIndexer) ensureIndexAt(ctx context.Context, address string) error {
	indexURL := fmt.Sprintf("%s/%s", address, i.index)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, indexURL, nil)
	if err != nil {
		return err
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("elasticsearch index check returned status %d", resp.StatusCode)
	}

	body, err := json.Marshal(programIndexDefinition())
	if err != nil {
		return err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, indexURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("elasticsearch index creation returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
}

func (i ProgramIndexer) UpsertPrograms(ctx context.Context, programs []program.Program) error {
	if len(programs) == 0 {
		return nil
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	indexedAt := time.Now().UTC()
	for _, item := range programs {
		if err := encoder.Encode(map[string]any{
			"index": map[string]any{"_index": i.index, "_id": strconv.FormatInt(item.ID, 10)},
		}); err != nil {
			return err
		}
		if err := encoder.Encode(map[string]any{
			"id":            item.ID,
			"title":         item.Title,
			"city":          item.City,
			"place":         item.Place,
			"show_time":     item.ShowTime.UTC().Format(time.RFC3339Nano),
			"status":        item.Status,
			"title_suggest": map[string]any{"input": []string{item.Title, item.City + " " + item.Title}},
			"updated_at":    indexedAt.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
	}

	var lastErr error
	for _, address := range i.addresses {
		if err := i.bulkAt(ctx, address, body.Bytes()); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("elasticsearch address is required")
	}
	return lastErr
}

func (i ProgramIndexer) DeletePrograms(ctx context.Context, programIDs []int64) error {
	if len(programIDs) == 0 {
		return nil
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, programID := range programIDs {
		if err := encoder.Encode(map[string]any{
			"delete": map[string]any{"_index": i.index, "_id": strconv.FormatInt(programID, 10)},
		}); err != nil {
			return err
		}
	}
	var lastErr error
	for _, address := range i.addresses {
		if err := i.bulkAt(ctx, address, body.Bytes()); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (i ProgramIndexer) bulkAt(ctx context.Context, address string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address+"/_bulk", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elasticsearch bulk index returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	var payload struct {
		Errors bool `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if payload.Errors {
		return fmt.Errorf("elasticsearch bulk index reported item errors")
	}
	return nil
}

func programIndexDefinition() map[string]any {
	return map[string]any{
		"settings": map[string]any{
			"number_of_shards":     1,
			"number_of_replicas":   0,
			"index.max_ngram_diff": 19,
			"analysis": map[string]any{
				"filter": map[string]any{
					"ticket_edge": map[string]any{"type": "edge_ngram", "min_gram": 1, "max_gram": 20},
				},
				"analyzer": map[string]any{
					"ticket_text":   map[string]any{"type": "standard"},
					"ticket_prefix": map[string]any{"type": "custom", "tokenizer": "standard", "filter": []string{"lowercase", "ticket_edge"}},
				},
			},
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":            map[string]any{"type": "long"},
				"title":         map[string]any{"type": "text", "analyzer": "ticket_text", "fields": map[string]any{"keyword": map[string]any{"type": "keyword"}, "prefix": map[string]any{"type": "text", "analyzer": "ticket_prefix", "search_analyzer": "ticket_text"}}},
				"title_suggest": map[string]any{"type": "completion"},
				"city":          map[string]any{"type": "text", "fields": map[string]any{"keyword": map[string]any{"type": "keyword"}}},
				"place":         map[string]any{"type": "text", "analyzer": "ticket_text"},
				"show_time":     map[string]any{"type": "date"},
				"status":        map[string]any{"type": "keyword"},
				"updated_at":    map[string]any{"type": "date"},
			},
		},
	}
}
