"use client";

import { useRealtime } from "@/hooks/useRealtime";

// Mounts the single WebSocket connection for the whole app so chat, the agent
// playground and the runs pages all receive run events without reconnecting.
export function RealtimeProvider() {
  useRealtime();
  return null;
}
