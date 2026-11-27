"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Eye,
  Loader2,
  Pencil,
  Power,
  PowerOff,
  RefreshCw,
  Search,
  Sparkles,
  Star,
} from "lucide-react";
import { agentApi, type Model } from "@/lib/api/agents";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

function compactNumber(value: number): string {
  if (!value) return "-";
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${Math.round(value / 1_000)}K`;
  return String(value);
}

export function AdminModels() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | "enabled" | "disabled">("all");
  const [editing, setEditing] = useState<Model | null>(null);
  const [editName, setEditName] = useState("");
  const [editIcon, setEditIcon] = useState("");

  const modelsQuery = useQuery({
    queryKey: ["admin", "models"],
    queryFn: () => agentApi.listModels(),
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["admin", "models"] });
    void queryClient.invalidateQueries({ queryKey: ["agents", "catalog"] });
  };

  const sync = useMutation({
    mutationFn: () => agentApi.syncModels(),
    onSuccess: (reply) => {
      toast.success(`Synced ${reply.synced} models from Bifrost`);
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const update = useMutation({
    mutationFn: ({
      model,
      patch,
    }: {
      model: Model;
      patch: Parameters<typeof agentApi.updateModel>[1];
      message: string;
    }) => agentApi.updateModel(model.id, patch),
    onSuccess: (_model, variables) => {
      toast.success(variables.message);
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const openEdit = (model: Model) => {
    setEditing(model);
    setEditName(model.displayName || model.modelKey);
    setEditIcon(model.icon || "");
  };

  const saveEdit = () => {
    if (!editing) return;
    update.mutate(
      {
        model: editing,
        patch: {
          displayName: editName.trim() || editing.modelKey,
          icon: editIcon.trim(),
        },
        message: "Model updated",
      },
      { onSuccess: () => setEditing(null) }
    );
  };

  const models = useMemo(
    () => modelsQuery.data?.models ?? [],
    [modelsQuery.data?.models]
  );
  const canManage = modelsQuery.data?.canManage ?? false;
  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return models.filter((model) => {
      if (filter === "enabled" && !model.enabled) return false;
      if (filter === "disabled" && model.enabled) return false;
      if (!query) return true;
      return (
        model.displayName.toLowerCase().includes(query) ||
        model.modelKey.toLowerCase().includes(query) ||
        model.provider.toLowerCase().includes(query)
      );
    });
  }, [filter, models, search]);

  return (
    <div className="space-y-4">
      <div className="rounded-xl border bg-card p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">Model catalog</h2>
            <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
              Models are synced from Bifrost. Provider keys, base URLs, routing
              and budgets stay in Bifrost; this console only decides which
              models agents may select and how they are presented.
            </p>
          </div>
          {canManage && (
            <Button
              size="sm"
              variant="outline"
              disabled={sync.isPending}
              onClick={() => sync.mutate()}
            >
              {sync.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <RefreshCw />
              )}
              Sync from Bifrost
            </Button>
          )}
        </div>
        <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex rounded-lg border p-1">
            {(["all", "enabled", "disabled"] as const).map((key) => (
              <button
                key={key}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs capitalize transition-colors",
                  filter === key
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
                onClick={() => setFilter(key)}
              >
                {key}
              </button>
            ))}
          </div>
          <div className="relative w-full sm:w-64">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-8"
              placeholder="Search models"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
        </div>
      </div>

      {modelsQuery.isError ? (
        <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          Failed to load models: {modelsQuery.error.message}
        </div>
      ) : null}

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[920px] text-left text-sm">
            <thead className="border-b bg-muted/30 text-xs text-muted-foreground">
              <tr>
                <th className="px-4 py-2.5">Model</th>
                <th>Provider</th>
                <th>Capabilities</th>
                <th>Context</th>
                <th>Price / Mtok</th>
                <th>Status</th>
                <th className="pr-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((model) => (
                <tr key={model.id} className="border-b last:border-0">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <span className="text-base" aria-hidden>
                        {model.icon || "•"}
                      </span>
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">
                          {model.displayName || model.modelKey}
                        </p>
                        <p className="truncate font-mono text-[10px] text-muted-foreground">
                          {model.modelKey}
                        </p>
                      </div>
                    </div>
                  </td>
                  <td className="text-xs text-muted-foreground">
                    {model.provider || "-"}
                  </td>
                  <td>
                    <div className="flex flex-wrap gap-1">
                      {model.supportsVision && (
                        <Badge variant="secondary" className="text-[9px]">
                          <Eye className="mr-0.5 h-2.5 w-2.5" />
                          vision
                        </Badge>
                      )}
                      {model.supportsTools && (
                        <Badge variant="secondary" className="text-[9px]">
                          tools
                        </Badge>
                      )}
                      {model.supportsReasoning && (
                        <Badge variant="secondary" className="text-[9px]">
                          reasoning
                        </Badge>
                      )}
                      {model.recommended && (
                        <Badge variant="secondary" className="text-[9px]">
                          <Star className="mr-0.5 h-2.5 w-2.5" />
                          recommended
                        </Badge>
                      )}
                    </div>
                  </td>
                  <td className="text-xs text-muted-foreground">
                    {compactNumber(model.contextWindow)}
                  </td>
                  <td className="whitespace-nowrap text-xs text-muted-foreground">
                    ${model.inputPricePerMtok.toFixed(2)} / $
                    {model.outputPricePerMtok.toFixed(2)}
                  </td>
                  <td>
                    <div className="flex flex-wrap gap-1">
                      <Badge
                        variant="secondary"
                        className={cn(
                          "text-[9px]",
                          model.enabled && "bg-success/10 text-success"
                        )}
                      >
                        {model.enabled ? "enabled" : "disabled"}
                      </Badge>
                      <Badge variant="secondary" className="text-[9px]">
                        {model.curated ? "curated" : "uncurated"}
                      </Badge>
                    </div>
                  </td>
                  <td className="pr-4">
                    {canManage ? (
                      <div className="flex flex-wrap justify-end gap-1">
                        <Button
                          size="xs"
                          variant={model.enabled ? "ghost" : "outline"}
                          disabled={update.isPending}
                          title={model.enabled ? "Disable" : "Enable"}
                          onClick={() =>
                            update.mutate({
                              model,
                              patch: { enabled: !model.enabled },
                              message: model.enabled
                                ? "Model disabled"
                                : "Model enabled",
                            })
                          }
                        >
                          {model.enabled ? <PowerOff /> : <Power />}
                          {model.enabled ? "Disable" : "Enable"}
                        </Button>
                        <Button
                          size="xs"
                          variant="ghost"
                          disabled={update.isPending}
                          title="Toggle vision capability"
                          onClick={() =>
                            update.mutate({
                              model,
                              patch: { supportsVision: !model.supportsVision },
                              message: "Vision capability updated",
                            })
                          }
                        >
                          <Eye />
                        </Button>
                        <Button
                          size="xs"
                          variant="ghost"
                          disabled={update.isPending}
                          title="Toggle recommended"
                          onClick={() =>
                            update.mutate({
                              model,
                              patch: { recommended: !model.recommended },
                              message: "Recommended flag updated",
                            })
                          }
                        >
                          <Sparkles />
                        </Button>
                        <Button
                          size="xs"
                          variant="ghost"
                          title="Edit presentation"
                          onClick={() => openEdit(model)}
                        >
                          <Pencil />
                        </Button>
                      </div>
                    ) : (
                      <span className="block text-right text-xs text-muted-foreground">
                        read only
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {modelsQuery.isPending && (
          <div className="flex justify-center p-6">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
        {!modelsQuery.isPending && filtered.length === 0 && (
          <p className="p-6 text-center text-sm text-muted-foreground">
            {models.length === 0
              ? "No models synced yet. Configure a provider in Bifrost, then sync the catalog."
              : "No models match this filter."}
          </p>
        )}
      </div>

      <Dialog open={!!editing} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Model presentation</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <label className="block text-xs font-medium">
              Display name
              <Input
                className="mt-1"
                value={editName}
                onChange={(event) => setEditName(event.target.value)}
                placeholder={editing?.modelKey}
              />
            </label>
            <label className="block text-xs font-medium">
              Icon (emoji or short label)
              <Input
                className="mt-1"
                value={editIcon}
                onChange={(event) => setEditIcon(event.target.value)}
                placeholder="🧠"
              />
            </label>
            <p className="text-xs text-muted-foreground">
              Model key, provider and gateway routing are owned by Bifrost and
              cannot be edited here.
            </p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setEditing(null)}>
                Cancel
              </Button>
              <Button disabled={update.isPending} onClick={saveEdit}>
                {update.isPending && <Loader2 className="animate-spin" />}
                Save
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
