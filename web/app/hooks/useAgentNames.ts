"use client";

import { useEffect, useState } from "react";
import { agentApi } from "@/lib/api/agents";

export interface AgentName {
  name: string;
}

let cachedNames: Map<number, AgentName> | null = null;
let inflight: Promise<Map<number, AgentName>> | null = null;
const listeners = new Set<() => void>();

// loadAgentNames fetches the agent directory once per session and caches it
// module-side so concurrently mounted components share a single request. On
// failure the cache stays empty and the in-flight promise is cleared, so the
// next mount retries instead of serving stale emptiness.
function loadAgentNames(): Promise<Map<number, AgentName>> {
  if (cachedNames) return Promise.resolve(cachedNames);
  if (inflight) return inflight;
  inflight = (async () => {
    const reply = await agentApi.list();
    const map = new Map<number, AgentName>();
    for (const agent of reply.items) {
      map.set(Number(agent.botUserId), { name: agent.name });
    }
    cachedNames = map;
    return map;
  })().finally(() => {
    inflight = null;
  });
  return inflight;
}

// invalidateAgentNames drops the cached directory and notifies live
// subscribers so they re-render with fresh data; called on logout (the next
// user on this tab must not see the previous user's agents) and after agent
// create/delete on the Agents page.
export function invalidateAgentNames(): void {
  cachedNames = null;
  for (const listener of listeners) listener();
}

export function useAgentNames(): Map<number, AgentName> {
  const [names, setNames] = useState<Map<number, AgentName>>(
    cachedNames ?? new Map()
  );

  useEffect(() => {
    let active = true;
    const reload = () => {
      loadAgentNames()
        .then((map) => {
          if (active) setNames(map);
        })
        .catch(() => {
          // Degrade to the plain username rendering on API failure.
        });
    };
    reload();
    listeners.add(reload);
    return () => {
      active = false;
      listeners.delete(reload);
    };
  }, []);

  return names;
}
