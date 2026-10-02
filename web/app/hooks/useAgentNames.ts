"use client";

import { useEffect, useState } from "react";
import { agentApi } from "@/lib/api";

export interface AgentName {
  name: string;
}

let cachedNames: Map<number, AgentName> | null = null;
let inflight: Promise<Map<number, AgentName>> | null = null;

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
    for (const agent of reply.agents) {
      map.set(Number(agent.botUserId), { name: agent.name });
    }
    cachedNames = map;
    return map;
  })().finally(() => {
    inflight = null;
  });
  return inflight;
}

// invalidateAgentNames drops the cached directory; called on logout so the
// next user to log in on this tab fetches their own agents (logout navigates
// client-side, which keeps the module cache alive across users).
export function invalidateAgentNames(): void {
  cachedNames = null;
}

export function useAgentNames(): Map<number, AgentName> {
  const [names, setNames] = useState<Map<number, AgentName>>(
    cachedNames ?? new Map()
  );

  useEffect(() => {
    let active = true;
    loadAgentNames()
      .then((map) => {
        if (active) setNames(map);
      })
      .catch(() => {
        // Degrade to the plain username rendering on API failure.
      });
    return () => {
      active = false;
    };
  }, []);

  return names;
}
