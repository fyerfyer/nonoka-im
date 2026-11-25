package search

import (
	"fmt"
	"os"
	"strings"

	"nonoka-im/internal/conf"
	imapp "nonoka-im/internal/im/app"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	BackendMongo       = "mongo"
	BackendMeilisearch = "meilisearch"
)

// Backend resolves the configured adapter. Empty backend infers Meilisearch
// when a URL is set and Mongo otherwise, so existing deployments keep working
// while a fresh install needs no search service.
func Backend(c *conf.Search) string {
	enabled := c != nil && c.GetEnabled()
	if raw := strings.TrimSpace(os.Getenv("SEARCH_ENABLED")); raw != "" {
		enabled = raw == "1" || strings.EqualFold(raw, "true")
	}
	if !enabled {
		return ""
	}
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("SEARCH_BACKEND")))
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(c.GetBackend()))
	}
	if backend != "" {
		return backend
	}
	if strings.TrimSpace(c.GetUrl()) != "" {
		return BackendMeilisearch
	}
	return BackendMongo
}

// NewIndex builds the configured SearchIndex. The returned value is nil when
// search is disabled.
func NewIndex(c *conf.Search, db *mongo.Database) (imapp.SearchIndex, error) {
	switch Backend(c) {
	case "":
		return nil, nil
	case BackendMongo:
		if db == nil {
			return nil, fmt.Errorf("mongo search requires a MongoDB database")
		}
		return NewMongoIndex(db), nil
	case BackendMeilisearch:
		client := FromConfig(c)
		if !client.Enabled() {
			return nil, fmt.Errorf("meilisearch backend requires a URL")
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unknown search backend %q", c.GetBackend())
	}
}

// IsMeilisearch reports whether the durable index outbox/relay should run.
func IsMeilisearch(c *conf.Search) bool { return Backend(c) == BackendMeilisearch }
