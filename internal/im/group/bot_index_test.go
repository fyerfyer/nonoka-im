package group

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

func TestBotIndexFilterHumansBatchesMissesAndKeepsNegativeCache(t *testing.T) {
	var loads atomic.Int32
	var queries atomic.Int32
	index := NewBotIndexWithQuery(
		func(context.Context) ([]int64, error) { return []int64{1}, nil },
		nil,
		log.DefaultLogger,
	)
	index.SetQueryMany(func(_ context.Context, ids []int64) ([]int64, error) {
		queries.Add(1)
		bots := make([]int64, 0, 1)
		for _, id := range ids {
			if id == 2 {
				bots = append(bots, id)
			}
		}
		return bots, nil
	})
	if err := index.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	loads.Add(1)

	humans, err := index.FilterHumans(context.Background(), []int64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(humans) != 1 || humans[0] != 3 {
		t.Fatalf("humans = %v, want [3]", humans)
	}
	if queries.Load() != 1 {
		t.Fatalf("batch queries = %d, want 1", queries.Load())
	}

	// A second call must use the bot set and negative cache, not query again.
	humans, err = index.FilterHumans(context.Background(), []int64{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(humans) != 1 || humans[0] != 3 {
		t.Fatalf("second humans = %v, want [3]", humans)
	}
	if queries.Load() != 1 {
		t.Fatalf("negative cache missed: batch queries = %d, want 1", queries.Load())
	}
	if loads.Load() != 1 {
		t.Fatalf("unexpected extra load: %d", loads.Load())
	}
}
