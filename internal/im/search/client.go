package search

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

	"nonoka-im/internal/im/domain"
)

const defaultIndex = "messages"

// Client is a small Meilisearch HTTP adapter. Keeping the wire calls explicit
// avoids a generated SDK dependency and makes the exact filter syntax auditable.
type Client struct {
	BaseURL string
	APIKey  string
	Index   string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey, index string, timeout time.Duration) *Client {
	if strings.TrimSpace(index) == "" {
		index = defaultIndex
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Index:   index,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool { return c != nil && c.BaseURL != "" }

// EnsureIndex creates the index (idempotently) and applies the settings that
// the search query depends on. Meilisearch applies settings asynchronously;
// callers do not need to wait for the task to finish.
func (c *Client) EnsureIndex(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	body := map[string]any{"uid": c.Index, "primaryKey": "id"}
	err := c.do(ctx, http.MethodPost, "/indexes", body, nil)
	if err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("create search index: %w", err)
	}
	settings := map[string]any{
		"searchableAttributes": []string{"content"},
		"filterableAttributes": []string{"topic", "sender_id", "timestamp", "msg_type", "has_file", "recalled"},
		"sortableAttributes":   []string{"timestamp"},
		"typoTolerance":        map[string]any{"enabled": false},
	}
	if err := c.do(ctx, http.MethodPatch, "/indexes/"+c.Index+"/settings", settings, nil); err != nil {
		return fmt.Errorf("configure search index: %w", err)
	}
	return nil
}

// IndexDocuments upserts one or more documents. The primary key is the IM
// msg_id, so replaying a task is idempotent.
func (c *Client) IndexDocuments(ctx context.Context, docs []Document) error {
	if !c.Enabled() || len(docs) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/indexes/"+c.Index+"/documents?primaryKey=id", docs, nil)
}

// DeleteDocument removes one message from the index.
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	if !c.Enabled() {
		return nil
	}
	return c.do(ctx, http.MethodDelete, "/indexes/"+c.Index+"/documents/"+id, nil, nil)
}

// ClearIndex removes every document from the index. It is intended for tests
// and the explicit rebuild path, never for normal operation.
func (c *Client) ClearIndex(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.do(ctx, http.MethodDelete, "/indexes/"+c.Index+"/documents", nil, nil)
}

func (c *Client) Healthy(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.do(ctx, http.MethodGet, "/health", nil, nil)
}

// Search implements app.SearchIndex. Topic and all filters are sent to
// Meilisearch so filtering happens before the result limit is applied.
func (c *Client) Search(ctx context.Context, topic, query string, filter domain.SearchFilter, limit, offset int) ([]domain.SearchHit, bool, error) {
	if !c.Enabled() {
		return nil, false, fmt.Errorf("search is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	body := map[string]any{
		"q":                     query,
		"filter":                buildFilter(topic, filter),
		"limit":                 limit + 1,
		"offset":                offset,
		"attributesToRetrieve":  []string{"msg_id", "topic", "topic_seq", "sender_id", "timestamp", "msg_type", "has_file"},
		"attributesToCrop":      []string{"content"},
		"cropLength":            200,
		"cropMarker":            "…",
		"attributesToHighlight": []string{"content"},
		"highlightPreTag":       "<em>",
		"highlightPostTag":      "</em>",
	}
	var reply struct {
		Hits []struct {
			MsgID     int64  `json:"msg_id"`
			Topic     string `json:"topic"`
			TopicSeq  uint64 `json:"topic_seq"`
			SenderID  int64  `json:"sender_id"`
			Timestamp int64  `json:"timestamp"`
			MsgType   int32  `json:"msg_type"`
			HasFile   bool   `json:"has_file"`
			Formatted struct {
				Content string `json:"content"`
			} `json:"_formatted"`
		} `json:"hits"`
	}
	if err := c.do(ctx, http.MethodPost, "/indexes/"+c.Index+"/search", body, &reply); err != nil {
		return nil, false, err
	}
	hasMore := len(reply.Hits) > limit
	if hasMore {
		reply.Hits = reply.Hits[:limit]
	}
	hits := make([]domain.SearchHit, 0, len(reply.Hits))
	for _, hit := range reply.Hits {
		hits = append(hits, domain.SearchHit{
			MsgID: hit.MsgID, Topic: hit.Topic, TopicSeq: hit.TopicSeq,
			SenderID: hit.SenderID, Timestamp: hit.Timestamp, MsgType: hit.MsgType,
			HasFile: hit.HasFile, Snippet: stripHighlight(hit.Formatted.Content),
		})
	}
	return hits, hasMore, nil
}

func buildFilter(topic string, filter domain.SearchFilter) string {
	parts := []string{
		`topic = "` + escapeFilter(topic) + `"`,
		"recalled = false",
	}
	if filter.SenderID > 0 {
		parts = append(parts, "sender_id = "+strconv.FormatInt(filter.SenderID, 10))
	}
	if filter.Since > 0 {
		parts = append(parts, "timestamp >= "+strconv.FormatInt(filter.Since, 10))
	}
	if filter.Until > 0 {
		parts = append(parts, "timestamp <= "+strconv.FormatInt(filter.Until, 10))
	}
	if filter.HasFile {
		parts = append(parts, "has_file = true")
	}
	return strings.Join(parts, " AND ")
}

func escapeFilter(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}

func stripHighlight(s string) string {
	s = strings.ReplaceAll(s, "<em>", "")
	return strings.ReplaceAll(s, "</em>", "")
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode search response: %w", err)
		}
	}
	return nil
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("search backend returned %d", e.Status)
	}
	return fmt.Sprintf("search backend returned %d: %s", e.Status, e.Body)
}

func isAlreadyExists(err error) bool {
	if e, ok := err.(*httpError); ok {
		return e.Status == http.StatusConflict || strings.Contains(strings.ToLower(e.Body), "already exists")
	}
	return false
}
