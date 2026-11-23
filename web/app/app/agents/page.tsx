"use client";

import { useCallback, useEffect, useMemo, useState, Fragment } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { ArrowLeft, Bot, Check, Loader2, MessageSquare, Plus, RefreshCw, Save, Send, Trash2 } from "lucide-react";
import {
  agentApi,
  groupApi,
  userApi,
  AppError,
  type AgentInput,
  type AgentRun,
  type AgentShare,
  type CapabilityPreset,
  type ManagedAgent,
  type MCPServer,
  type ModelService,
  type Skill,
  type ToolDefinition,
} from "@/lib/api";
import { getP2PTopic } from "@/lib/topic";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { invalidateAgentNames } from "@/hooks/useAgentNames";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { PlatformPanel } from "@/components/agents/PlatformPanel";

type Policy = "auto" | "ask" | "deny";

interface Catalog {
  services: ModelService[];
  presets: CapabilityPreset[];
  tools: ToolDefinition[];
  mcps: MCPServer[];
  skills: Skill[];
}

const emptyCatalog: Catalog = { services: [], presets: [], tools: [], mcps: [], skills: [] };

function errorText(err: unknown) { return err instanceof Error ? err.message : "Request failed"; }

function formatDuration(start?: string | null, end?: string | null): string {
  if (!start) return "-";
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return "-";
  const sec = Math.round((e - s) / 1000);
  if (sec < 1) return "<1s";
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

function defaultInput(catalog: Catalog): AgentInput {
  const service = catalog.services.find((s) => s.enabled);
  const model = service?.models.find((m) => m.enabled);
  return {
    name: "",
    description: "",
    instruction: "",
    modelServiceId: service?.id ?? 0,
    modelId: model?.id ?? 0,
    tools: [],
    capabilityPresets: catalog.presets.filter((p) => p.defaultEnabled && p.key !== "system").map((p) => p.key),
    toolPolicies: {},
    maxTokens: 4096,
    mcpBindings: [],
    skills: [],
  };
}

export default function AgentsPage() {
  const router = useRouter();
  const token = useAuthStore((s) => s.token);
  const currentUserId = useAuthStore((s) => s.user?.userId) || 0;
  const [agents, setAgents] = useState<ManagedAgent[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [selected, setSelected] = useState<ManagedAgent | null>(null);
  const [tab, setTab] = useState<"config" | "sharing" | "runs" | "platform">("config");
  const [catalog, setCatalog] = useState<Catalog>(emptyCatalog);
  const [draft, setDraft] = useState<AgentInput>(() => defaultInput(emptyCatalog));
  const [createOpen, setCreateOpen] = useState(false);
  const [createInput, setCreateInput] = useState<AgentInput>(() => defaultInput(emptyCatalog));
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [advanced, setAdvanced] = useState(false);
  const [shares, setShares] = useState<AgentShare[]>([]);
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [shareKind, setShareKind] = useState<AgentShare["kind"]>("user");
  const [shareTarget, setShareTarget] = useState("");
  const [userResults, setUserResults] = useState<{ userId: number; username: string }[]>([]);
  const [groups, setGroups] = useState<{ groupId: string; name: string }[]>([]);
  const [shareUserNames, setShareUserNames] = useState<Map<number, string>>(new Map());
  const [shareGroupNames, setShareGroupNames] = useState<Map<string, string>>(new Map());
  const [deleteTarget, setDeleteTarget] = useState<ManagedAgent | null>(null);
  const [expandedRun, setExpandedRun] = useState<number | null>(null);
  const [runDetail, setRunDetail] = useState<AgentRun | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const refreshList = useCallback(() => {
    return agentApi.list().then((data) => {
      setAgents(data.items);
      setSelectedId((current) => current ?? data.items[0]?.id ?? null);
    });
  }, []);

  const refreshCatalog = useCallback(() => {
    return Promise.allSettled([
      agentApi.listModelServices(),
      agentApi.agentTools(),
      agentApi.listMCPServers(),
      agentApi.listSkills(),
    ]).then(([services, tools, mcps, skills]) => {
      setCatalog({
        services: services.status === "fulfilled" ? services.value.services : [],
        presets: tools.status === "fulfilled" ? tools.value.presets : [],
        tools: tools.status === "fulfilled" ? tools.value.tools : [],
        mcps: mcps.status === "fulfilled" ? mcps.value.servers.filter((s) => s.enabled) : [],
        skills: skills.status === "fulfilled" ? skills.value.skills : [],
      });
    });
  }, []);

  const refreshSelected = useCallback((id: number) => {
    return agentApi.detail(id).then((detail) => {
      setSelected(detail);
      const source = detail.draft;
      if (source) {
        setDraft({
          name: detail.name,
          description: detail.description,
          instruction: source.instruction,
          modelServiceId: source.modelServiceId,
          modelId: source.modelId,
          tools: source.tools ?? [],
          capabilityPresets: source.capabilityPresets ?? [],
          toolPolicies: source.toolPolicies ?? {},
          maxTokens: source.maxTokens || 4096,
          mcpBindings: source.mcpBindings ?? [],
          skills: source.skills ?? [],
        });
      }
    });
  }, []);

  useEffect(() => {
    refreshCatalog();
  }, [refreshCatalog]);

  useEffect(() => {
    if (!token && !localStorage.getItem("token")) { router.replace("/login"); return; }
    Promise.all([refreshList(), refreshCatalog()]).catch((err) => toast.error(errorText(err))).finally(() => setLoading(false));
  }, [router, token, refreshList, refreshCatalog]);

  useEffect(() => {
    if (selectedId === null) {
      const clear = setTimeout(() => setSelected(null), 0);
      return () => clearTimeout(clear);
    }
    refreshSelected(selectedId).catch((err) => toast.error(errorText(err)));
  }, [selectedId, refreshSelected]);

  useEffect(() => {
    if (!selectedId || !selected?.canManage) return;
    if (tab === "sharing") {
      agentApi.shares(selectedId).then((data) => setShares(data.shares)).catch((err) => toast.error(errorText(err)));
      groupApi.listMyGroups().then((data) => setGroups(data.groups.map((g) => ({ groupId: g.groupId, name: g.name })))).catch(() => setGroups([]));
    }
  }, [selectedId, selected?.canManage, tab]);

  useEffect(() => {
    if (!selectedId || !selected?.canManage || tab !== "runs") return;
    const load = () => {
      agentApi.runs(selectedId).then((data) => setRuns(data.items)).catch((err) => toast.error(errorText(err)));
      if (expandedRun !== null) {
        agentApi.runDetail(expandedRun).then(setRunDetail).catch(() => setRunDetail(null));
      }
    };
    load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [selectedId, selected?.canManage, tab, expandedRun]);

  const conversations = useChatStore((s) => s.conversations);
  const allMemberNames = useChatStore((s) => s.memberNames);
  const baseShareUserNames = useMemo(() => {
    const users = new Map<number, string>();
    if (tab !== "sharing") return users;
    for (const conv of conversations.values()) {
      if (conv.type === "p2p" && conv.peerUsername) users.set(Number(conv.peerId), conv.peerUsername);
    }
    for (const names of allMemberNames.values()) {
      for (const [id, name] of names) users.set(Number(id), name);
    }
    return users;
  }, [tab, conversations, allMemberNames]);

  useEffect(() => {
    if (tab !== "sharing") return;
    for (const share of shares) {
      if (share.kind !== "group") continue;
      if (groups.some((g) => g.groupId === share.target) || shareGroupNames.has(share.target)) continue;
      groupApi.getGroup(share.target).then((g) => setShareGroupNames((old) => new Map(old).set(share.target, g.name))).catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, groups, shares]);

  useEffect(() => {
    if (shareKind !== "user" || shareTarget.trim().length < 2) {
      const clear = setTimeout(() => setUserResults([]), 0);
      return () => clearTimeout(clear);
    }
    const timer = setTimeout(() => userApi.searchUsers(shareTarget.trim()).then((r) => setUserResults(r.users)).catch(() => setUserResults([])), 300);
    return () => clearTimeout(timer);
  }, [shareKind, shareTarget]);

  const act = async (task: () => Promise<unknown>, success: string) => {
    setBusy(true);
    try {
      await task();
      toast.success(success);
      await Promise.all([refreshList(), refreshCatalog()]);
      if (selectedId) await refreshSelected(selectedId);
    } catch (err) {
      toast.error(errorText(err));
    } finally { setBusy(false); }
  };

  const create = () => act(async () => {
    const row = await agentApi.create(createInput);
    invalidateAgentNames();
    setCreateOpen(false);
    setCreateInput(defaultInput(catalog));
    setSelectedId(row.id);
  }, "Agent created");

  const saveDraftAction = async (publish: boolean) => {
    if (!selected?.draft) return;
    setBusy(true);
    try {
      const updated = await agentApi.saveDraft(selected.id, { ...draft, revision: selected.draft.revision });
      if (publish) await agentApi.publish(selected.id, updated.draft!.revision);
      toast.success(publish ? "Version published" : "Draft saved");
      await refreshList();
      await refreshSelected(selected.id);
    } catch (err) {
      if (err instanceof AppError && err.code === "DRAFT_CONFLICT") {
        toast.error("Draft was modified by another session — refresh the page and try again");
      } else {
        toast.error(errorText(err));
      }
    } finally { setBusy(false); }
  };

  const doDelete = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      await agentApi.delete(deleteTarget.id);
      invalidateAgentNames();
      toast.success("Agent deleted");
      setDeleteTarget(null);
      const data = await agentApi.list();
      setAgents(data.items);
      if (selectedId === deleteTarget.id) setSelectedId(data.items[0]?.id ?? null);
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const toggleRun = async (id: number) => {
    if (expandedRun === id) { setExpandedRun(null); setRunDetail(null); return; }
    setExpandedRun(id);
    setRunDetail(null);
    setDetailLoading(true);
    try { setRunDetail(await agentApi.runDetail(id)); }
    catch (err) { toast.error(errorText(err)); setExpandedRun(null); }
    finally { setDetailLoading(false); }
  };

  const chatWithBot = () => {
    if (!selected) return;
    if (selected.currentVersion === 0) {
      toast.error("Publish the agent first to start chatting");
      return;
    }
    const topic = getP2PTopic(currentUserId, selected.botUserId);
    useChatStore.getState().upsertConversation({
      topic,
      type: "p2p",
      peerId: selected.botUserId,
      peerUsername: selected.name,
      peerIsBot: true,
    });
    router.push(`/chat/${encodeURIComponent(topic)}`);
  };

  const togglePreset = (input: AgentInput, set: (next: AgentInput) => void, key: string) => {
    const presets = input.capabilityPresets ?? [];
    set({ ...input, capabilityPresets: presets.includes(key) ? presets.filter((p) => p !== key) : [...presets, key] });
  };

  const setPolicy = (input: AgentInput, set: (next: AgentInput) => void, tool: string, policy: Policy) => {
    const next = { ...(input.toolPolicies ?? {}) };
    if (policy === "auto") delete next[tool];
    else next[tool] = policy;
    set({ ...input, toolPolicies: next });
  };

  const toggleMCPTool = (input: AgentInput, set: (next: AgentInput) => void, serverId: number, tool: string, risk: string) => {
    const bindings = input.mcpBindings ?? [];
    const exists = bindings.some((b) => b.serverId === serverId && b.tool === tool);
    const next = exists
      ? bindings.filter((b) => !(b.serverId === serverId && b.tool === tool))
      : [...bindings, { serverId, tool, policy: risk === "write" ? ("ask" as Policy) : ("auto" as Policy) }];
    set({ ...input, mcpBindings: next });
  };

  const setMCPPolicy = (input: AgentInput, set: (next: AgentInput) => void, serverId: number, tool: string, policy: Policy) => {
    set({
      ...input,
      mcpBindings: (input.mcpBindings ?? []).map((b) => (b.serverId === serverId && b.tool === tool ? { ...b, policy } : b)),
    });
  };

  const toggleSkill = (input: AgentInput, set: (next: AgentInput) => void, skillId: number, version: number) => {
    const bindings = input.skills ?? [];
    const exists = bindings.some((b) => b.skillId === skillId);
    set({ ...input, skills: exists ? bindings.filter((b) => b.skillId !== skillId) : [...bindings.filter((b) => b.skillId !== skillId), { skillId, version }] });
  };

  const editor = (input: AgentInput, set: (next: AgentInput) => void) => {
    const service = catalog.services.find((s) => s.id === input.modelServiceId);
    const models = service?.models.filter((m) => m.enabled) ?? [];
    const visiblePresets = catalog.presets.filter((p) => p.key !== "system");
    const toolsByPreset = new Map<string, ToolDefinition[]>();
    for (const tool of catalog.tools) {
      if (tool.hidden || tool.system) continue;
      for (const preset of tool.presets ?? []) {
        const list = toolsByPreset.get(preset) ?? [];
        list.push(tool);
        toolsByPreset.set(preset, list);
      }
    }
    const bindableMCPs = catalog.mcps.filter((s) => s.reviewStatus === "approved" || (s.ownerUserId === currentUserId && s.visibility === "private"));
    const bindableSkills = catalog.skills.filter((s) => s.reviewStatus === "approved" || (s.ownerUserId === currentUserId && s.visibility === "private"));
    return (
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block text-sm font-medium">Name<Input className="mt-1" value={input.name} maxLength={100} onChange={(e) => set({ ...input, name: e.target.value })} /></label>
          <label className="block text-sm font-medium">Max output tokens<Input className="mt-1" type="number" min={1} max={128000} value={input.maxTokens ?? 4096} onChange={(e) => set({ ...input, maxTokens: Number(e.target.value) || 0 })} /></label>
        </div>
        <label className="block text-sm font-medium">Description<Input className="mt-1" value={input.description} maxLength={4000} onChange={(e) => set({ ...input, description: e.target.value })} /></label>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block text-sm font-medium">Model service
            <select
              aria-label="Model service"
              className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm"
              value={input.modelServiceId || ""}
              onChange={(e) => {
                const serviceId = Number(e.target.value);
                const nextService = catalog.services.find((s) => s.id === serviceId);
                const modelId = nextService?.models.find((m) => m.enabled)?.id ?? 0;
                set({ ...input, modelServiceId: serviceId, modelId });
              }}
            >
              <option value="">Select service</option>
              {catalog.services.filter((s) => s.enabled).map((s) => <option key={s.id} value={s.id}>{s.displayName || s.name} ({s.gatewayType})</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium">Model
            <select
              aria-label="Model"
              className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm"
              value={input.modelId || ""}
              onChange={(e) => set({ ...input, modelId: Number(e.target.value) })}
            >
              <option value="">Select model</option>
              {models.map((m) => <option key={m.id} value={m.id}>{m.displayName || m.modelKey}</option>)}
            </select>
          </label>
        </div>
        <label className="block text-sm font-medium">Instructions<textarea className="mt-1 min-h-48 w-full resize-y rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring" value={input.instruction} maxLength={20000} onChange={(e) => set({ ...input, instruction: e.target.value })} /></label>

        <section className="space-y-3 rounded-lg border p-3">
          <div className="flex items-center justify-between">
            <div><h3 className="text-sm font-semibold">Capabilities</h3><p className="text-xs text-muted-foreground">Presets enable a group of tools. Writes default to approval.</p></div>
            <Button type="button" size="xs" variant="ghost" onClick={() => setAdvanced((v) => !v)}>{advanced ? "Hide advanced" : "Advanced"}</Button>
          </div>
          {visiblePresets.map((preset) => {
            const enabled = (input.capabilityPresets ?? []).includes(preset.key);
            const tools = toolsByPreset.get(preset.key) ?? [];
            return (
              <div key={preset.key} className="rounded-md border p-2">
                <label className="flex items-start gap-2 text-sm">
                  <input className="mt-1" type="checkbox" checked={enabled} onChange={() => togglePreset(input, set, preset.key)} />
                  <span><span className="font-medium">{preset.name}</span><span className="block text-xs text-muted-foreground">{preset.description}</span></span>
                </label>
                {advanced && enabled && tools.length > 0 && (
                  <div className="mt-2 space-y-1 pl-6">
                    {tools.map((tool) => (
                      <div key={tool.name} className="flex items-center justify-between gap-2 text-xs">
                        <span className="min-w-0 truncate" title={tool.description}>{tool.name} <span className="text-muted-foreground">({tool.risk})</span></span>
                        <select
                          aria-label={`${tool.name} policy`}
                          className="h-7 rounded-md border bg-background px-1"
                          value={input.toolPolicies?.[tool.name] ?? tool.defaultPolicy}
                          onChange={(e) => setPolicy(input, set, tool.name, e.target.value as Policy)}
                        >
                          <option value="auto">auto</option>
                          <option value="ask">ask</option>
                          <option value="deny">deny</option>
                        </select>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </section>

        {bindableMCPs.length > 0 && (
          <section className="space-y-2 rounded-lg border p-3">
            <h3 className="text-sm font-semibold">MCP tools</h3>
            {bindableMCPs.map((server) => (
              <div key={server.id} className="text-sm">
                <div className="flex items-center gap-2 font-medium">
                  {server.displayName || server.name}
                  <Badge variant="secondary" className="text-[10px]">{server.visibility}</Badge>
                  <Badge variant="secondary" className={cn("text-[10px]", server.healthStatus === "healthy" && "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400")}>{server.healthStatus}</Badge>
                </div>
                {(server.tools ?? []).map((tool) => {
                  const binding = (input.mcpBindings ?? []).find((b) => b.serverId === server.id && b.tool === tool.name);
                  return (
                    <div key={tool.name} className="flex items-center justify-between gap-2 pl-3">
                      <label className="flex min-w-0 items-center gap-2 text-xs">
                        <input type="checkbox" checked={!!binding} onChange={() => toggleMCPTool(input, set, server.id, tool.name, tool.risk)} />
                        <span className="truncate" title={tool.description}>{tool.name} <span className="text-muted-foreground">({tool.risk})</span></span>
                      </label>
                      {binding && (
                        <select aria-label={`${tool.name} policy`} className="h-7 rounded-md border bg-background px-1 text-xs" value={binding.policy ?? (tool.risk === "write" ? "ask" : "auto")} onChange={(e) => setMCPPolicy(input, set, server.id, tool.name, e.target.value as Policy)}>
                          <option value="auto">auto</option><option value="ask">ask</option><option value="deny">deny</option>
                        </select>
                      )}
                    </div>
                  );
                })}
              </div>
            ))}
          </section>
        )}

        {bindableSkills.length > 0 && (
          <section className="space-y-2 rounded-lg border p-3">
            <h3 className="text-sm font-semibold">Skills</h3>
            {bindableSkills.map((skill) => (
              <label key={skill.id} className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={(input.skills ?? []).some((b) => b.skillId === skill.id)} onChange={() => toggleSkill(input, set, skill.id, skill.latestVersion)} />
                <span>{skill.displayName || skill.slug}</span>
                <Badge variant="secondary" className="text-[10px]">v{skill.latestVersion} · {skill.visibility}</Badge>
              </label>
            ))}
          </section>
        )}
      </div>
    );
  };

  if (loading) return <div className="flex min-h-screen items-center justify-center"><Loader2 className="h-5 w-5 animate-spin" /></div>;

  return (
    <div className="min-h-screen bg-background">
      <header className="flex h-14 items-center justify-between border-b px-4 sm:px-6">
        <div className="flex items-center gap-3"><Button variant="ghost" size="icon" title="Back to chat" onClick={() => router.push("/chat")}><ArrowLeft /></Button><Bot className="h-5 w-5 text-primary" /><h1 className="text-base font-semibold">Agents</h1></div>
        <Button size="sm" onClick={() => { setCreateInput(defaultInput(catalog)); setCreateOpen(true); }}><Plus /> New agent</Button>
      </header>
      <div className="mx-auto grid max-w-7xl md:min-h-[calc(100vh-3.5rem)] md:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="border-b p-3 md:border-b-0 md:border-r">
          <div className="mb-2 px-2 text-xs font-semibold uppercase text-muted-foreground">Your agents</div>
          <div className="flex gap-1 overflow-x-auto md:flex-col">
            {agents.map((item) => (
              <div key={item.id} className={cn("flex min-w-36 items-center gap-1 rounded-md md:w-full", selectedId === item.id ? "bg-accent text-accent-foreground" : "hover:bg-muted")}>
                <button onClick={() => { setSelectedId(item.id); setTab("config"); }} className="flex min-w-0 flex-1 items-center justify-between gap-2 px-3 py-2 text-left text-sm">
                  <span className="truncate">{item.name}</span>
                  <span className="text-xs text-muted-foreground">{item.status === "active" ? "On" : "Off"}</span>
                </button>
                {item.canManage && (
                  <button title={`Delete ${item.name}`} className="shrink-0 rounded-md p-1.5 text-muted-foreground opacity-70 hover:text-destructive" onClick={() => setDeleteTarget(item)}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        </aside>
        <main className="min-w-0 px-4 py-5 sm:px-8">
          <div className="mb-5 flex gap-5 border-b text-sm">{(["config", "sharing", "runs", "platform"] as const).filter((item) => item === "config" || item === "platform" || selected?.canManage).map((item) => <button key={item} onClick={() => setTab(item)} className={`border-b-2 pb-2 capitalize ${tab === item ? "border-primary font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground"}`}>{item}</button>)}</div>
          {tab === "platform" ? <PlatformPanel catalog={catalog} onChanged={() => { refreshCatalog(); refreshList(); }} /> : !selected ? <div className="py-20 text-center text-sm text-muted-foreground">No agents yet</div> : <>
            <div className="mb-5 flex flex-wrap items-start justify-between gap-3"><div><div className="flex items-center gap-2"><h2 className="text-xl font-semibold">{selected.name}</h2><Badge variant="secondary">{selected.status}</Badge><Button size="sm" variant="outline" aria-disabled={selected.currentVersion === 0} className={cn(selected.currentVersion === 0 && "cursor-not-allowed opacity-50")} onClick={chatWithBot}><MessageSquare /> Chat</Button></div><p className="mt-1 text-sm text-muted-foreground">Bot #{selected.botUserId} · Version {selected.currentVersion || "unpublished"}</p></div>{selected.canManage && selected.currentVersion > 0 && <Button size="sm" variant={selected.status === "active" ? "outline" : "default"} disabled={busy} onClick={() => act(() => agentApi.setStatus(selected.id, selected.status === "active" ? "disabled" : "active"), selected.status === "active" ? "Agent disabled" : "Agent enabled")}>{selected.status === "active" ? "Disable" : "Enable"}</Button>}</div>
            {tab === "config" && (selected.canManage && selected.draft ? <div className="max-w-3xl">{editor(draft, setDraft)}<div className="mt-5 flex flex-wrap gap-2"><Button variant="outline" disabled={busy || !draft.name.trim() || !draft.instruction.trim() || !draft.modelId} onClick={() => saveDraftAction(false)}><Save /> Save draft</Button><Button disabled={busy || !draft.name.trim() || !draft.instruction.trim() || !draft.modelId} onClick={() => saveDraftAction(true)}><Send /> Publish</Button></div></div> : <div className="max-w-3xl space-y-3 text-sm"><p>{selected.description || "No description"}</p><div><div className="mb-1 font-medium">Published instructions</div><p className="whitespace-pre-wrap text-muted-foreground">{selected.published?.instruction || "No published version"}</p></div><p className="text-xs text-muted-foreground">Model: {selected.published?.modelKey || "-"}</p></div>)}
            {tab === "sharing" && selected.canManage && <div className="max-w-2xl space-y-5"><div className="flex flex-wrap gap-2"><select aria-label="Share type" value={shareKind} onChange={(e) => { setShareKind(e.target.value as AgentShare["kind"]); setShareTarget(""); }} className="h-8 rounded-md border bg-background px-2 text-sm"><option value="user">User</option><option value="group">Group</option></select>{shareKind === "group" ? <select aria-label="Group" value={shareTarget} onChange={(e) => setShareTarget(e.target.value)} className="h-8 min-w-52 rounded-md border bg-background px-2 text-sm"><option value="">Select group</option>{groups.map((group) => <option key={group.groupId} value={group.groupId}>{group.name}</option>)}</select> : <div className="relative min-w-52 flex-1"><Input placeholder="Search users" value={shareTarget} onChange={(e) => setShareTarget(e.target.value)} />{userResults.length > 0 && <div className="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-md border bg-background shadow-sm">{userResults.map((user) => <button key={user.userId} className="block w-full px-3 py-2 text-left text-sm hover:bg-muted" onClick={() => { setShareTarget(String(user.userId)); setShareUserNames((old) => new Map(old).set(Number(user.userId), user.username)); setUserResults([]); }}>{user.username} · #{user.userId}</button>)}</div>}</div>}<Button disabled={busy || !shareTarget} onClick={() => act(async () => { await agentApi.share(selected.id, shareKind, shareTarget, true); setShares((old) => [...old.filter((s) => !(s.kind === shareKind && s.target === shareTarget)), { kind: shareKind, target: shareTarget }]); setShareTarget(""); }, "Access granted")}><Check /> Share</Button></div><div className="divide-y border-y">{shares.length === 0 && <p className="py-4 text-sm text-muted-foreground">Private · owner only</p>}{shares.map((share) => <div key={`${share.kind}:${share.target}`} className="flex items-center justify-between py-2 text-sm"><span>{share.kind === "group" ? (shareGroupNames.get(share.target) || share.target) : (baseShareUserNames.get(Number(share.target)) || shareUserNames.get(Number(share.target)) || `User #${share.target}`)}<span className="ml-2 text-xs text-muted-foreground">{share.kind}</span></span><Button size="xs" variant="ghost" onClick={() => act(() => agentApi.share(selected.id, share.kind, share.target, false), "Access revoked")}>Remove</Button></div>)}</div></div>}
            {tab === "runs" && selected.canManage && <div><div className="mb-3 flex justify-end"><Button variant="ghost" size="icon" title="Refresh runs" onClick={() => agentApi.runs(selected.id).then((data) => setRuns(data.items)).catch((err) => toast.error(errorText(err)))}><RefreshCw /></Button></div><div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="border-b text-xs text-muted-foreground"><tr><th className="py-2">Run</th><th>Actor</th><th>Topic</th><th>Status</th><th>Error</th><th>Tokens</th><th>Duration</th><th>Created</th><th className="text-right">Action</th></tr></thead><tbody>{runs.map((run) => <Fragment key={run.id}>
              <tr className="cursor-pointer border-b hover:bg-muted/40" onClick={() => void toggleRun(run.id)}><td className="py-3">#{run.id}</td><td>#{run.actorUserId}</td><td className="max-w-40 truncate" title={run.topic}>{run.topic}</td><td><Badge variant={run.status === "failed" ? "destructive" : "secondary"}>{run.status}</Badge></td><td>{run.errorCode ? <span className="text-destructive">{run.errorCode}</span> : "-"}</td><td>{run.totalTokens}</td><td>{formatDuration(run.startedAt, run.finishedAt)}</td><td>{new Date(run.createdAt).toLocaleString()}</td><td className="text-right" onClick={(e) => e.stopPropagation()}>{run.status === "failed" && <div className="flex justify-end gap-1"><Button size="xs" variant="outline" disabled={busy} onClick={() => act(async () => { await agentApi.runAction(run.id, "retry"); setRuns((await agentApi.runs(selected.id)).items); }, "Run queued")}>Retry</Button><Button size="xs" variant="ghost" disabled={busy} onClick={() => act(async () => { await agentApi.runAction(run.id, "skip"); setRuns((await agentApi.runs(selected.id)).items); }, "Run skipped")}>Skip</Button></div>}</td></tr>
              {expandedRun === run.id && <tr className="border-b bg-muted/40"><td colSpan={9} className="px-4 py-3 text-xs">{detailLoading && !runDetail ? <span className="flex items-center gap-2"><Loader2 className="h-3.5 w-3.5 animate-spin" />Loading run details…</span> : <dl className="grid grid-cols-2 gap-x-6 gap-y-1 sm:grid-cols-4"><div><dt className="text-muted-foreground">Attempts</dt><dd>{runDetail?.attempts ?? "-"}</dd></div><div><dt className="text-muted-foreground">Agent version</dt><dd>{runDetail?.agentVersion ? `v${runDetail.agentVersion}` : "-"}</dd></div><div><dt className="text-muted-foreground">Reply message</dt><dd>{runDetail?.replyMsgId ? `#${runDetail.replyMsgId}` : "-"}</dd></div><div><dt className="text-muted-foreground">Error code</dt><dd>{runDetail?.errorCode || run.errorCode || "-"}</dd></div><div><dt className="text-muted-foreground">Input tokens</dt><dd>{runDetail?.inputTokens ?? run.inputTokens}</dd></div><div><dt className="text-muted-foreground">Output tokens</dt><dd>{runDetail?.outputTokens ?? run.outputTokens}</dd></div><div><dt className="text-muted-foreground">Started</dt><dd>{runDetail?.startedAt ? new Date(runDetail.startedAt).toLocaleString() : "-"}</dd></div><div><dt className="text-muted-foreground">Finished</dt><dd>{runDetail?.finishedAt ? new Date(runDetail.finishedAt).toLocaleString() : "-"}</dd></div></dl>}</td></tr>}
            </Fragment>)}</tbody></table>{runs.length === 0 && <p className="py-8 text-center text-sm text-muted-foreground">No runs</p>}</div></div>}
          </>}
        </main>
      </div>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogContent className="max-h-[90vh] overflow-auto sm:max-w-2xl"><DialogHeader><DialogTitle>New agent</DialogTitle></DialogHeader>{editor(createInput, setCreateInput)}<div className="flex justify-end"><Button disabled={busy || !createInput.name.trim() || !createInput.instruction.trim() || !createInput.modelId} onClick={create}>{busy ? <Loader2 className="animate-spin" /> : <Plus />} Create agent</Button></div></DialogContent></Dialog>
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}><DialogContent><DialogHeader><DialogTitle>Delete agent</DialogTitle></DialogHeader><p className="text-sm text-muted-foreground">Delete <span className="font-medium text-foreground">{deleteTarget?.name}</span>? Its versions and shares are removed; the bot account stays as an inert user. This cannot be undone.</p><div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancel</Button><Button variant="destructive" disabled={busy} onClick={() => void doDelete()}>{busy ? <Loader2 className="animate-spin" /> : <Trash2 />}Delete</Button></div></DialogContent></Dialog>
    </div>
  );
}
