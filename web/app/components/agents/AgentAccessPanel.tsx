"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Check, Loader2, Lock, Search, Trash2, Users } from "lucide-react";
import {
  agentApi,
  type AgentShare,
  type ShareRole,
} from "@/lib/api/agents";
import { groupApi, userApi } from "@/lib/api";
import { agentKeys, agentSharesQuery } from "@/lib/queries/agents";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type ShareKind = AgentShare["kind"];

export function AgentAccessPanel({
  agentId,
  canOwn,
}: {
  agentId: number;
  canOwn: boolean;
}) {
  const queryClient = useQueryClient();
  const shares = useQuery(agentSharesQuery(agentId));
  const groups = useQuery({
    queryKey: ["groups", "mine"],
    queryFn: () => groupApi.listMyGroups(),
  });
  const [kind, setKind] = useState<ShareKind>("user");
  const [target, setTarget] = useState("");
  const [role, setRole] = useState<ShareRole>("viewer");
  const [search, setSearch] = useState("");
  const [userResults, setUserResults] = useState<{ userId: number; username: string }[]>([]);
  const [names, setNames] = useState<Map<string, string>>(new Map());

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: agentKeys.shares(agentId) });

  const share = useMutation({
    mutationFn: (input: { kind: ShareKind; target: string; role: ShareRole; enabled: boolean }) =>
      agentApi.share(agentId, input.kind, input.target, input.role, input.enabled),
    onSuccess: () => {
      toast.success("Access updated");
      setTarget("");
      setSearch("");
      void invalidate();
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "Access update failed"),
  });

  const groupNames = useMemo(() => {
    const map = new Map<string, string>();
    for (const group of groups.data?.groups ?? []) {
      map.set(group.groupId, group.name);
    }
    return map;
  }, [groups.data?.groups]);

  const targetLabel = (item: AgentShare) => {
    const key = `${item.kind}:${item.target}`;
    if (names.has(key)) return names.get(key)!;
    if (item.kind === "group") return groupNames.get(item.target) || `Group ${item.target}`;
    return `User #${item.target}`;
  };

  const searchUsers = async (value: string) => {
    setSearch(value);
    if (value.trim().length < 2) {
      setUserResults([]);
      return;
    }
    try {
      const reply = await userApi.searchUsers(value.trim());
      setUserResults(reply.users);
    } catch {
      setUserResults([]);
    }
  };

  const pickTarget = (id: string, name: string) => {
    setTarget(id);
    setSearch(name);
    setUserResults([]);
    setNames((current) => new Map(current).set(`user:${id}`, name));
  };

  return (
    <div className="max-w-3xl space-y-5">
      <div className="rounded-xl border bg-card p-4">
        <h3 className="text-sm font-semibold">Who can use this agent</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Viewers can chat with the agent. Editors can change configuration and
          publish. Only the owner can delete or change sharing.
        </p>

        {canOwn ? (
          <div className="mt-4 space-y-3 rounded-lg border p-3">
            <div className="flex flex-wrap gap-2">
              <select
                className="h-9 rounded-md border bg-background px-2 text-sm"
                value={kind}
                onChange={(event) => {
                  setKind(event.target.value as ShareKind);
                  setTarget("");
                  setSearch("");
                  setUserResults([]);
                }}
              >
                <option value="user">User</option>
                <option value="group">Group</option>
              </select>
              <select
                className="h-9 rounded-md border bg-background px-2 text-sm"
                value={role}
                onChange={(event) => setRole(event.target.value as ShareRole)}
              >
                <option value="viewer">Viewer</option>
                <option value="editor">Editor</option>
              </select>
              {kind === "group" ? (
                <select
                  className="h-9 min-w-56 flex-1 rounded-md border bg-background px-2 text-sm"
                  value={target}
                  onChange={(event) => setTarget(event.target.value)}
                >
                  <option value="">Select a group</option>
                  {(groups.data?.groups ?? []).map((group) => (
                    <option key={group.groupId} value={group.groupId}>
                      {group.name}
                    </option>
                  ))}
                </select>
              ) : (
                <div className="relative min-w-56 flex-1">
                  <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    className="pl-8"
                    placeholder="Search users"
                    value={search}
                    onChange={(event) => void searchUsers(event.target.value)}
                  />
                  {userResults.length > 0 && (
                    <div className="absolute z-20 mt-1 max-h-48 w-full overflow-auto rounded-md border bg-background shadow-sm">
                      {userResults.map((user) => (
                        <button
                          key={user.userId}
                          className="block w-full px-3 py-2 text-left text-sm hover:bg-muted"
                          onClick={() => pickTarget(String(user.userId), user.username)}
                        >
                          {user.username} · #{user.userId}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )}
              <Button
                disabled={!target || share.isPending}
                onClick={() =>
                  share.mutate({ kind, target, role, enabled: true })
                }
              >
                {share.isPending ? <Loader2 className="animate-spin" /> : <Check />}
                Share
              </Button>
            </div>
          </div>
        ) : (
          <p className="mt-3 flex items-center gap-1.5 rounded-lg border bg-muted/30 p-3 text-xs text-muted-foreground">
            <Lock className="h-3.5 w-3.5" />
            Only the owner can change sharing.
          </p>
        )}
      </div>

      <div className="rounded-xl border bg-card">
        <div className="flex items-center gap-2 border-b px-4 py-3">
          <Users className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">Shared with</h3>
          <Badge variant="secondary">{shares.data?.shares.length ?? 0}</Badge>
        </div>
        <div className="divide-y">
          {shares.isPending && (
            <div className="p-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}
          {!shares.isPending && (shares.data?.shares.length ?? 0) === 0 && (
            <p className="p-4 text-sm text-muted-foreground">
              Private · only the owner can access this agent.
            </p>
          )}
          {(shares.data?.shares ?? []).map((item) => (
            <div
              key={`${item.kind}:${item.target}:${item.role}`}
              className="flex items-center justify-between gap-3 px-4 py-3 text-sm"
            >
              <div className="flex min-w-0 items-center gap-2">
                <Badge variant="secondary" className="text-[10px]">
                  {item.kind}
                </Badge>
                <span className="truncate">{targetLabel(item)}</span>
                <Badge
                  variant="outline"
                  className={cn(
                    "text-[10px] capitalize",
                    item.role === "editor" && "border-primary/40 text-primary"
                  )}
                >
                  {item.role || "viewer"}
                </Badge>
              </div>
              {canOwn && (
                <Button
                  size="xs"
                  variant="ghost"
                  disabled={share.isPending}
                  onClick={() =>
                    share.mutate({
                      kind: item.kind,
                      target: item.target,
                      role: item.role || "viewer",
                      enabled: false,
                    })
                  }
                >
                  <Trash2 />
                  Remove
                </Button>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
