"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Bot, Code2, Plus, Search, Sparkles } from "lucide-react";
import { agentsQuery } from "@/lib/queries/agents";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/workspace/PageHeader";
import { EmptyState } from "@/components/workspace/EmptyState";

export function AgentDirectory() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const agents = useQuery(agentsQuery);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return (agents.data?.items ?? []).filter((agent) => {
      if (!query) return true;
      return (
        agent.name.toLowerCase().includes(query) ||
        agent.description.toLowerCase().includes(query)
      );
    });
  }, [agents.data?.items, search]);

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader
        title="Agents"
        description="Build, publish and operate agents that live in your conversations."
        icon={Bot}
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push("/developer")}
            >
              <Code2 />
              <span className="hidden sm:inline">Developer</span>
            </Button>
            <Button size="sm" onClick={() => router.push("/agents/new")}>
              <Plus />
              New agent
            </Button>
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-7xl space-y-5 px-4 py-6 sm:px-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-sm font-medium">
                {filtered.length} agent{filtered.length === 1 ? "" : "s"}
              </p>
              <p className="text-xs text-muted-foreground">
                Shared agents appear here when someone grants you access.
              </p>
            </div>
            <div className="relative w-full sm:w-72">
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
                <div
                  key={index}
                  className="h-40 animate-pulse rounded-xl border bg-muted/40"
                />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="rounded-xl border border-dashed bg-card/40">
              <EmptyState
                icon={Sparkles}
                title={
                  (agents.data?.items ?? []).length === 0
                    ? "No agents yet"
                    : "No matching agents"
                }
                description={
                  (agents.data?.items ?? []).length === 0
                    ? "Start from a template or create a blank agent, then publish it to chat."
                    : "Try a different name or description."
                }
                action={
                  <Button size="sm" onClick={() => router.push("/agents/new")}>
                    <Plus />
                    Browse templates
                  </Button>
                }
                secondaryAction={
                  (agents.data?.items ?? []).length > 0 ? (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setSearch("")}
                    >
                      Clear search
                    </Button>
                  ) : undefined
                }
              />
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {filtered.map((agent) => (
                <button
                  key={agent.id}
                  className="group flex min-h-40 flex-col rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
                  onClick={() =>
                    router.push(`/agents/${agent.id}?tab=overview`)
                  }
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
                      <Bot className="h-4 w-4 text-primary" />
                    </div>
                    <Badge
                      variant={agent.status === "active" ? "default" : "secondary"}
                    >
                      {agent.status === "active"
                        ? `v${agent.currentVersion}`
                        : agent.status}
                    </Badge>
                  </div>
                  <p className="mt-3 truncate text-sm font-semibold">
                    {agent.name}
                  </p>
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                    {agent.description || "No description"}
                  </p>
                  <div className="mt-auto flex items-center justify-between pt-3 text-[10px] text-muted-foreground">
                    <span>Bot #{agent.botUserId}</span>
                    <span>
                      {agent.canOwn
                        ? "Owner"
                        : agent.canManage
                          ? "Can edit"
                          : "Shared"}
                    </span>
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
        </div>
      </div>
    </div>
  );
}
