"use client";

import { useChatStore } from "@/stores/chatStore";
import { cn } from "@/lib/utils";
import { Loader2, Wifi, WifiOff, AlertCircle } from "lucide-react";

export function ConnectionBar() {
  const state = useChatStore((s) => s.connectionState);

  if (state === "authed" || state === "connected") return null;

  const config = {
    connecting: {
      icon: Loader2,
      text: "Connecting...",
      className: "bg-amber-50 text-amber-700 border-amber-200",
    },
    reconnecting: {
      icon: Loader2,
      text: "Reconnecting...",
      className: "bg-amber-50 text-amber-700 border-amber-200",
    },
    disconnected: {
      icon: WifiOff,
      text: "Disconnected",
      className: "bg-red-50 text-red-700 border-red-200",
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
