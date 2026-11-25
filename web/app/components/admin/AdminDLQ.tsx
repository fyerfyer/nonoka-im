"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, RefreshCw, RotateCw } from "lucide-react";
import { toast } from "sonner";
import { agentApi, type FailedOutboxEvent } from "@/lib/api/agents";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

function formatTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString();
}

export function AdminDLQ() {
  const [rows, setRows] = useState<FailedOutboxEvent[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const page = await agentApi.failedOutbox(100);
      setRows(page.items);
      setSelected((current) => {
        const next = new Set<string>();
        for (const row of page.items) {
          if (current.has(row.id)) next.add(row.id);
        }
        return next;
      });
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const first = setTimeout(() => void load(), 0);
    const timer = setInterval(() => void load(), 10000);
    return () => {
      clearTimeout(first);
      clearInterval(timer);
    };
  }, [load]);

  const allSelected = rows.length > 0 && rows.every((row) => selected.has(row.id));

  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(rows.map((row) => row.id)));
  };

  const replay = async (ids: string[]) => {
    if (ids.length === 0) return;
    setBusy(true);
    try {
      const result = ids.length === 1
        ? await agentApi.replayOutbox(ids[0])
        : await agentApi.replayOutboxMany(ids);
      toast.success(`Replayed ${"count" in result ? result.count : 1} event(s)`);
      await load();
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const selectedIDs = useMemo(() => Array.from(selected), [selected]);

  return (
    <div className="max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Agent trigger dead letters</h3>
          <p className="text-xs text-muted-foreground">
            Replay is idempotent on event_id; a row is only requeued while it is
            still failed.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={busy || selectedIDs.length === 0} onClick={() => void replay(selectedIDs)}>
            <RotateCw />
            Replay selected ({selectedIDs.length})
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            {loading ? <Loader2 className="animate-spin" /> : <RefreshCw />}
            Refresh
          </Button>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
              <tr>
                <th className="w-10 px-3 py-2.5">
                  <input type="checkbox" checked={allSelected} onChange={toggleAll} aria-label="Select all" />
                </th>
                <th>Event</th>
                <th>Topic</th>
                <th>Seq</th>
                <th>Attempts</th>
                <th>Error</th>
                <th>Created</th>
                <th className="pr-4 text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id} className="border-b last:border-0">
                  <td className="px-3 py-2">
                    <input
                      type="checkbox"
                      checked={selected.has(row.id)}
                      onChange={() => {
                        setSelected((current) => {
                          const next = new Set(current);
                          if (next.has(row.id)) next.delete(row.id);
                          else next.add(row.id);
                          return next;
                        });
                      }}
                      aria-label={`Select ${row.eventId}`}
                    />
                  </td>
                  <td className="max-w-48 truncate font-mono text-[11px]" title={row.eventId}>{row.eventId}</td>
                  <td className="max-w-40 truncate text-xs" title={row.topic}>{row.topic}</td>
                  <td className="text-xs">{row.topicSeq}</td>
                  <td><Badge variant="secondary" className="text-[10px]">{row.attempts}</Badge></td>
                  <td className="max-w-64 truncate text-xs text-destructive" title={row.lastError}>{row.lastError || "-"}</td>
                  <td className="text-xs text-muted-foreground">{formatTime(row.createdAt)}</td>
                  <td className="pr-4 text-right">
                    <Button size="xs" variant="outline" disabled={busy} onClick={() => void replay([row.id])}>
                      <RotateCw /> Replay
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {loading && (
          <div className="flex justify-center p-6"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
        )}
        {!loading && rows.length === 0 && (
          <p className="p-6 text-center text-sm text-muted-foreground">No failed agent events.</p>
        )}
      </div>
    </div>
  );
}
