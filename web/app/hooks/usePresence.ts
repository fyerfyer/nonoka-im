"use client";

import { useEffect, useState } from "react";
import { userApi } from "@/lib/api";

const PRESENCE_POLL_MS = 10_000;

export function usePresence(userId: number | undefined, enabled: boolean) {
  const [presence, setPresence] = useState<{
    userId: number;
    online: boolean;
  }>();

  useEffect(() => {
    if (!enabled || !userId) {
      return;
    }

    let active = true;
    const refresh = async () => {
      try {
        const reply = await userApi.getPresence(userId);
        if (active) setPresence({ userId, online: reply.online });
      } catch {
        // Preserve the last known state during a transient API failure.
      }
    };

    void refresh();
    const timer = window.setInterval(refresh, PRESENCE_POLL_MS);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [enabled, userId]);

  return enabled && userId && presence?.userId === userId
    ? presence.online
    : undefined;
}
