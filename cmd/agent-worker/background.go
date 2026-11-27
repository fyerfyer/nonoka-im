package main

import (
	"context"
	"fmt"
	"time"

	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
	"nonoka-im/internal/llm"

	"github.com/go-kratos/kratos/v2/log"
)

func ensureAgentGatewayKeys(ctx context.Context, store *agentdata.Store, models *llm.Service, logger log.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	run := func() {
		agents, err := store.ListPublishedAgents(ctx)
		if err != nil {
			logger.Log(log.LevelWarn, "msg", fmt.Sprintf("list published agents for keys: %v", err))
			return
		}
		for i := range agents {
			versions, err := store.ListVersions(ctx, agents[i].ID)
			if err != nil {
				continue
			}
			seen := map[int64]bool{}
			modelsForAgent := make([]domain.Model, 0, len(versions))
			for _, version := range versions {
				if version.ModelID <= 0 || seen[version.ModelID] {
					continue
				}
				seen[version.ModelID] = true
				model, err := models.Lookup(ctx, version.ModelID)
				if err != nil {
					continue
				}
				modelsForAgent = append(modelsForAgent, *model)
			}
			if len(modelsForAgent) == 0 {
				continue
			}
			if err := models.EnsureAgentKey(ctx, agents[i].ID, modelsForAgent); err != nil {
				logger.Log(log.LevelWarn, "msg", fmt.Sprintf("ensure agent key agent=%d: %v", agents[i].ID, err))
			}
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
