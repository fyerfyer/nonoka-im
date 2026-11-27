"use client";

import { useChatStore } from "@/stores/chatStore";
import { cn } from "@/lib/utils";
import { Loader2, WifiOff } from "lucide-react";

export function ConnectionBar() {
  const state = useChatStore((s) => s.connectionState);

  if (state === "authed" || state === "connected") return null;

  const config = {
    connecting: {
      icon: Loader2,
      text: "Connecting...",
      className: "bg-warning/10 text-warning border-warning/30",
    },
    reconnecting: {
      icon: Loader2,
      text: "Reconnecting...",
      className: "bg-warning/10 text-warning border-warning/30",
    },
    disconnected: {
      icon: WifiOff,
      text: "Disconnected",
      className: "bg-destructive/10 text-destructive border-destructive/30",
    },
  }[state];

  const Icon = config.icon;

  return (
    <div
      className={cn(
        "flex items-center justify-center gap-2 border-b px-3 py-1.5 text-xs font-medium",
        config.className
      )}
    >
      <Icon className={cn("h-3.5 w-3.5", Icon === Loader2 && "animate-spin")} />
      {config.text}
    </div>
  );
}
