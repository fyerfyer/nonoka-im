"use client";

import type { ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { RealtimeProvider } from "@/components/RealtimeProvider";
import { WorkspaceRail } from "./WorkspaceRail";
import { MobileTabBar } from "./MobileTabBar";

export function WorkspaceShell({ children }: { children: ReactNode }) {
  const { isReady, isAuthenticated } = useAuth();

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex h-dvh items-center justify-center bg-background">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex h-dvh overflow-hidden bg-background">
      <RealtimeProvider />
      <a
        href="#workspace-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:shadow"
      >
        Skip to content
      </a>
      <WorkspaceRail />
      <main
        id="workspace-content"
        className="flex min-w-0 flex-1 flex-col overflow-hidden pb-14 md:pb-0"
      >
        {children}
      </main>
      <MobileTabBar />
    </div>
  );
}
