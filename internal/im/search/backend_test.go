package search

import (
	"testing"

	"nonoka-im/internal/conf"
)

func TestBackendInference(t *testing.T) {
	cases := []struct {
		name string
		cfg  *conf.Search
		want string
	}{
		{name: "disabled", cfg: &conf.Search{Enabled: false, Url: "http://meili"}, want: ""},
		{name: "explicit mongo", cfg: &conf.Search{Enabled: true, Backend: "MONGO", Url: "http://meili"}, want: BackendMongo},
		{name: "explicit meili", cfg: &conf.Search{Enabled: true, Backend: "meilisearch"}, want: BackendMeilisearch},
		{name: "url infers meili", cfg: &conf.Search{Enabled: true, Url: "http://meili"}, want: BackendMeilisearch},
		{name: "empty infers mongo", cfg: &conf.Search{Enabled: true}, want: BackendMongo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Backend(tc.cfg); got != tc.want {
				t.Fatalf("Backend() = %q, want %q", got, tc.want)
			}
		})
	}
}
