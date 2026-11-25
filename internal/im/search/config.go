package search

import (
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/conf"
)

// FromConfig builds the optional search client. Environment variables override
// the YAML block so deployments can inject the URL and API key without editing
// files. An empty URL disables search.
func FromConfig(c *conf.Search) *Client {
	url := ""
	index := ""
	timeout := 5 * time.Second
	if c != nil {
		url = c.GetUrl()
		index = c.GetIndex()
		if c.GetTimeout() != nil && c.GetTimeout().AsDuration() > 0 {
			timeout = c.GetTimeout().AsDuration()
		}
	}
	if v := strings.TrimSpace(os.Getenv("MEILISEARCH_URL")); v != "" {
		url = v
	}
	if v := strings.TrimSpace(os.Getenv("MEILISEARCH_INDEX")); v != "" {
		index = v
	}
	if v := strings.TrimSpace(os.Getenv("MEILISEARCH_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	if strings.TrimSpace(url) == "" {
		return &Client{}
	}
	return NewClient(url, strings.TrimSpace(os.Getenv("MEILISEARCH_API_KEY")), index, timeout)
}

// RelayInterval reads the optional relay cadence.
func RelayInterval(c *conf.Search) time.Duration {
	if c != nil && c.GetRelayInterval() != nil && c.GetRelayInterval().AsDuration() > 0 {
		return c.GetRelayInterval().AsDuration()
	}
	if v := strings.TrimSpace(os.Getenv("MEILISEARCH_RELAY_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return time.Second
}

// RelayBatchSize reads the optional relay batch size.
func RelayBatchSize(c *conf.Search) int {
	if c != nil && c.GetRelayBatchSize() > 0 {
		return int(c.GetRelayBatchSize())
	}
	if v := strings.TrimSpace(os.Getenv("MEILISEARCH_RELAY_BATCH")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 32
}
