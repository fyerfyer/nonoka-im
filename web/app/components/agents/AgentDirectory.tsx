"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Bot, Code2, MessageSquare, Plus, Search, Settings2, Sparkles } from "lucide-react";
import { agentsQuery, catalogQuery } from "@/lib/queries/agents";
import { useAuthStore } from "@/stores/authStore";
import { useAuth } from "@/hooks/useAuth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { AgentApprovalInbox } from "./AgentApprovalInbox";

type Scope = "mine" | "available";

export function AgentDirectory() {
  const router = useRouter();
  const user = useAuthStore((state) => state.user);
  const { isReady, isAuthenticated } = useAuth();
  const [scope, setScope] = useState<Scope>("mine");
  const [search, setSearch] = useState("");
  const agents = useQuery(agentsQuery);
  const catalog = useQuery(catalogQuery);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return (agents.data?.items ?? []).filter((agent) => {
      const mine = agent.ownerUserId === Number(user?.userId);
      if (scope === "mine" && !mine) return false;
      if (scope === "available" && mine) return false;
      if (!query) return true;
      return (
        agent.name.toLowerCase().includes(query) ||
        agent.description.toLowerCase().includes(query)
      );
    });
  }, [agents.data?.items, scope, search, user?.userId]);

  const openAgent = (id: number) => router.push(`/agents/${id}?tab=overview`);

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <span className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-10 flex h-14 items-center justify-between border-b bg-background/95 px-4 backdrop-blur sm:px-6">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={() => router.push("/chat")} title="Back to chat">
            <MessageSquare />
          </Button>
          <Bot className="h-5 w-5 text-primary" />
          <h1 className="text-base font-semibold">Agents</h1>
        </div>
        <div className="flex items-center gap-2">
          {catalog.data?.canManageModels && (
            <Button variant="ghost" size="sm" onClick={() => router.push("/admin")}>
              <Settings2 />
              <span className="hidden sm:inline">Platform</span>
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={() => router.push("/developer")}>
            <Code2 />
            <span className="hidden sm:inline">Developer</span>
          </Button>
          <Button size="sm" onClick={() => router.push("/agents/new")}>
            <Plus />
            New agent
          </Button>
        </div>
      </header>

      <div className="mx-auto grid max-w-7xl gap-6 px-4 py-6 lg:grid-cols-[minmax(0,1fr)_320px] sm:px-6">
        <main className="min-w-0 space-y-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex rounded-lg border p-1">
              {(["mine", "available"] as const).map((item) => (
                <button
                  key={item}
                  className={cn(
                    "rounded-md px-3 py-1.5 text-sm capitalize transition-colors",
                    scope === item
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                  onClick={() => setScope(item)}
                >
                  {item === "mine" ? "My agents" : "Available"}
                </button>
              ))}
            </div>
            <div className="relative w-full sm:w-64">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder="Search agents"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          </div>

          {agents.isPending ? (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {Array.from({ length: 4 }).map((_, index) => (
                <div key={index} className="h-40 animate-pulse rounded-xl border bg-muted/40" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed py-20 text-center">
              <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Sparkles className="h-6 w-6 text-primary" />
              </div>
              <div>
                <p className="text-sm font-medium">No agents here yet</p>
                <p className="text-xs text-muted-foreground">
                  Create one, or ask an owner to share an agent with you.
                </p>
              </div>
              <Button size="sm" onClick={() => router.push("/agents/new")}>
                <Plus />
                Create agent
              </Button>
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {filtered.map((agent) => (
                <button
                  key={agent.id}
                  className="flex min-h-40 flex-col rounded-xl border bg-card p-4 text-left transition-shadow hover:shadow-md"
                  onClick={() => openAgent(agent.id)}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
                      <Bot className="h-4 w-4 text-primary" />
                    </div>
                    <Badge variant={agent.status === "active" ? "default" : "secondary"}>
                      {agent.status === "active"
                        ? `v${agent.currentVersion}`
                        : agent.status}
                    </Badge>
                  </div>
                  <p className="mt-3 truncate text-sm font-semibold">{agent.name}</p>
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                    {agent.description || "No description"}
                  </p>
                  <div className="mt-auto flex items-center justify-between pt-3 text-[10px] text-muted-foreground">
                    <span>Bot #{agent.botUserId}</span>
                    <span>{agent.canOwn ? "Owner" : agent.canManage ? "Can edit" : "Shared"}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
          {agents.isError && (
            <p className="text-sm text-destructive">
              Failed to load agents: {agents.error.message}
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            Select an agent to configure, test or publish it.
          </p>
        </main>

        <aside className="space-y-4">
          <AgentApprovalInbox />
        </aside>
      </div>
    </div>
  );
}
