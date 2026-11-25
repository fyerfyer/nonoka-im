"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { agentApi, type AuditEvent } from "@/lib/api/agents";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

export function AdminAudit() {
  const [filters, setFilters] = useState({ action: "", resourceType: "", resourceId: "", actor: "" });
  const [selected, setSelected] = useState<AuditEvent | null>(null);
  const events = useQuery({
    queryKey: ["admin", "audit", filters],
    queryFn: () => agentApi.auditEvents(Object.fromEntries(Object.entries(filters).filter(([, v]) => v))),
    refetchInterval: 15000,
  });

  return (
    <div className="space-y-4">
      <div className="grid gap-2 sm:grid-cols-4">
        <Input placeholder="Action (agent.publish)" value={filters.action} onChange={(e) => setFilters({ ...filters, action: e.target.value })} />
        <Input placeholder="Resource type" value={filters.resourceType} onChange={(e) => setFilters({ ...filters, resourceType: e.target.value })} />
        <Input placeholder="Resource id" value={filters.resourceId} onChange={(e) => setFilters({ ...filters, resourceId: e.target.value })} />
        <Input placeholder="Actor user id" value={filters.actor} onChange={(e) => setFilters({ ...filters, actor: e.target.value })} />
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        <table className="w-full text-left text-sm">
          <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
            <tr>
              <th className="px-4 py-2">Time</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Resource</th>
              <th>Outcome</th>
              <th className="pr-4 text-right">Detail</th>
            </tr>
          </thead>
          <tbody>
            {(events.data?.items ?? []).map((ev) => (
              <tr key={ev.id} className="border-b last:border-0">
                <td className="px-4 py-2 text-xs text-muted-foreground">{new Date(ev.createdAt).toLocaleString()}</td>
                <td className="text-xs">#{ev.actorUserId}</td>
                <td className="font-mono text-xs">{ev.action}</td>
                <td className="text-xs">{ev.resourceType}:{ev.resourceId}</td>
                <td>
                  <Badge variant="secondary" className={ev.outcome === "failure" ? "bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-400" : ""}>
                    {ev.outcome}
                  </Badge>
                </td>
                <td className="pr-4 text-right">
                  <Button size="xs" variant="outline" onClick={() => setSelected(ev)}>View</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!events.isPending && (events.data?.items ?? []).length === 0 && (
          <p className="p-4 text-sm text-muted-foreground">No audit events match the filter.</p>
        )}
      </div>

      {selected && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => setSelected(null)}>
          <div className="max-h-[80vh] w-full max-w-2xl overflow-auto rounded-xl border bg-background p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <div className="mb-3 flex items-start justify-between gap-4">
              <div>
                <h3 className="text-sm font-semibold">{selected.action}</h3>
                <p className="text-xs text-muted-foreground">{new Date(selected.createdAt).toLocaleString()} · #{selected.id}</p>
              </div>
              <Button size="xs" variant="ghost" onClick={() => setSelected(null)}>Close</Button>
            </div>
            <dl className="grid gap-2 text-xs sm:grid-cols-2">
              <div><dt className="text-muted-foreground">Actor</dt><dd>#{selected.actorUserId}</dd></div>
              <div><dt className="text-muted-foreground">Resource</dt><dd>{selected.resourceType}:{selected.resourceId}</dd></div>
              <div><dt className="text-muted-foreground">Request ID</dt><dd className="font-mono">{selected.requestId || "-"}</dd></div>
              <div><dt className="text-muted-foreground">Trace ID</dt><dd className="font-mono">{selected.traceId || "-"}</dd></div>
              <div><dt className="text-muted-foreground">IP</dt><dd>{selected.ip || "-"}</dd></div>
              <div><dt className="text-muted-foreground">Outcome</dt><dd>{selected.outcome}{selected.errorCode ? ` · ${selected.errorCode}` : ""}</dd></div>
            </dl>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <div>
                <h4 className="mb-1 text-xs font-semibold">Before</h4>
                <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-2 text-[11px]">{JSON.stringify(selected.before ?? null, null, 2)}</pre>
              </div>
              <div>
                <h4 className="mb-1 text-xs font-semibold">After</h4>
                <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-2 text-[11px]">{JSON.stringify(selected.after ?? null, null, 2)}</pre>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
