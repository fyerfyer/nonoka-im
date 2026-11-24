"use client";

import { useRouter } from "next/navigation";
import { ArrowLeft, Loader2, Settings2 } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { AgentPlatformPanel } from "@/components/admin/AgentPlatformPanel";
import { Button } from "@/components/ui/button";
import { emptyCatalog } from "@/components/agents/agent-utils";

export default function AdminAgentsPage() {
  const router = useRouter();
  const { isReady, isAuthenticated } = useAuth();

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-6">
        <Button variant="ghost" size="icon" onClick={() => router.push("/agents")}>
          <ArrowLeft />
        </Button>
        <Settings2 className="h-5 w-5 text-primary" />
        <h1 className="text-base font-semibold">Agent platform</h1>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
        <AgentPlatformPanel catalog={emptyCatalog} />
      </main>
    </div>
  );
}
