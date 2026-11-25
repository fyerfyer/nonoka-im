"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { agentApi, type QuotaLimits, type Quota } from "@/lib/api/agents";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { errorText } from "@/components/agents/agent-utils";

const emptyLimits: QuotaLimits = {
  maxConcurrentRuns: 0,
  dailyRuns: 0,
  dailyTokens: 0,
  dailyCostUsd: 0,
  maxToolTurns: 0,
};

export function AdminQuotas() {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<"user" | "agent">("agent");
  const [editing, setEditing] = useState<Quota | null>(null);
  const [form, setForm] = useState<QuotaLimits>(emptyLimits);
  const quotas = useQuery({
    queryKey: ["admin", "quotas", scope],
    queryFn: () => agentApi.listQuotas(scope),
    refetchInterval: 15000,
  });
  const top = useQuery({
    queryKey: ["admin", "usage", scope],
    queryFn: () => agentApi.topUsage(scope, "cost", 10),
    refetchInterval: 15000,
  });
  const save = useMutation({
    mutationFn: async () => {
      if (!editing) return;
      await agentApi.updateQuota(scope, editing.scopeKey, form);
    },
    onSuccess: () => {
      toast.success("Quota saved");
      setEditing(null);
      queryClient.invalidateQueries({ queryKey: ["admin", "quotas", scope] });
      queryClient.invalidateQueries({ queryKey: ["admin", "usage", scope] });
    },
    onError: (err) => toast.error(errorText(err)),
  });
  const reset = useMutation({
    mutationFn: async (q: Quota) => agentApi.resetQuota(scope, q.scopeKey),
    onSuccess: () => {
      toast.success("Quota reset to defaults");
      setEditing(null);
      queryClient.invalidateQueries({ queryKey: ["admin", "quotas", scope] });
    },
    onError: (err) => toast.error(errorText(err)),
  });

  const startEdit = (q: Quota) => {
    setEditing(q);
    setForm({
      maxConcurrentRuns: q.maxConcurrentRuns ?? 0,
      dailyRuns: q.dailyRuns ?? 0,
      dailyTokens: q.dailyTokens ?? 0,
      dailyCostUsd: q.dailyCostUsd ?? 0,
      maxToolTurns: q.maxToolTurns ?? 0,
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        {(["agent", "user"] as const).map((s) => (
          <Button key={s} size="sm" variant={scope === s ? "default" : "outline"} onClick={() => { setScope(s); setEditing(null); }}>
            {s === "agent" ? "Agent quotas" : "User quotas"}
          </Button>
        ))}
        <span className="text-xs text-muted-foreground">
          0 = unlimited. Use Reset to return to platform defaults. Changes are audited.
        </span>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <table className="w-full text-left text-sm">
          <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2">Scope key</th>
              <th>Concurrency</th>
              <th>Daily runs</th>
              <th>Daily tokens</th>
              <th>Daily cost</th>
              <th>Tool turns</th>
              <th className="pr-4 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {(quotas.data?.items ?? []).map((q) => (
              <tr key={`${q.scope}:${q.scopeKey}`} className="border-b last:border-0">
                <td className="px-4 py-2 font-mono text-xs">{q.scopeKey}</td>
                <td className="text-xs">{q.maxConcurrentRuns ?? "-"}</td>
                <td className="text-xs">{q.dailyRuns ?? "-"}</td>
                <td className="text-xs">{q.dailyTokens ?? "-"}</td>
                <td className="text-xs">{q.dailyCostUsd != null ? `$${q.dailyCostUsd}` : "-"}</td>
                <td className="text-xs">{q.maxToolTurns ?? "-"}</td>
                <td className="pr-4 text-right">
                  <Button size="xs" variant="outline" onClick={() => startEdit(q)}>Edit</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!quotas.isPending && (quotas.data?.items ?? []).length === 0 && (
          <p className="p-4 text-sm text-muted-foreground">No explicit quota rows. Configure a scope key below or use defaults.</p>
        )}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="rounded-xl border bg-card p-4">
          <h3 className="mb-3 text-sm font-semibold">{editing ? `Edit ${editing.scopeKey}` : "Add / edit quota"}</h3>
          <div className="space-y-2">
            <label className="block text-xs font-medium">Scope key (user id or agent id)
              <Input className="mt-1" value={editing?.scopeKey ?? ""} onChange={(e) => setEditing({ ...(editing ?? { scope, scopeKey: "", updatedAt: "" }), scopeKey: e.target.value })} placeholder="123" />
            </label>
            <div className="grid grid-cols-2 gap-2">
              {([
                ["maxConcurrentRuns", "Max concurrent runs"],
                ["dailyRuns", "Daily runs"],
                ["dailyTokens", "Daily tokens"],
                ["dailyCostUsd", "Daily cost USD"],
                ["maxToolTurns", "Max tool turns"],
              ] as const).map(([key, label]) => (
                <label key={key} className="block text-xs font-medium">{label}
                  <Input className="mt-1" type="number" min={0} value={form[key]} onChange={(e) => setForm({ ...form, [key]: Math.max(0, Number(e.target.value)) })} />
                </label>
              ))}
            </div>
            <div className="flex gap-2 pt-2">
              <Button size="sm" disabled={!editing?.scopeKey || save.isPending} onClick={() => save.mutate()}>Save</Button>
              {editing && <Button size="sm" variant="outline" onClick={() => reset.mutate(editing)}>Reset to defaults</Button>}
            </div>
          </div>
        </div>

        <div className="rounded-xl border bg-card p-4">
          <h3 className="mb-3 text-sm font-semibold">Top consumers today</h3>
          <div className="space-y-2">
            {(top.data?.items ?? []).map((row) => (
              <div key={row.scopeKey} className="flex items-center justify-between gap-2 text-xs">
                <div className="flex items-center gap-2">
                  <Badge variant="secondary" className="text-[10px]">{row.scope}</Badge>
                  <span className="font-mono">{row.scopeKey}</span>
                </div>
                <span className="text-muted-foreground">
                  {row.runs} runs · {row.inputTokens + row.outputTokens} tokens · ${(row.costUsd ?? 0).toFixed(4)}
                </span>
              </div>
            ))}
            {(top.data?.items ?? []).length === 0 && <p className="text-xs text-muted-foreground">No usage today.</p>}
          </div>
        </div>
      </div>
    </div>
  );
}
