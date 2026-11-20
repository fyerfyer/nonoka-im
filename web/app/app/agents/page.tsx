"use client";

import { useCallback, useEffect, useMemo, useState, Fragment } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { ArrowLeft, Bot, Check, Loader2, MessageSquare, Plus, RefreshCw, Save, Send, Trash2 } from "lucide-react";
import { agentApi, groupApi, userApi, AppError, type AgentInput, type AgentRun, type AgentShare, type AgentTool, type ManagedAgent } from "@/lib/api";
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

const toolNames: { id: AgentTool; label: string }[] = [
  { id: "search_messages", label: "Search messages" },
  { id: "get_conversation_context", label: "Read conversation" },
  { id: "read_file", label: "Read attachments" },
  { id: "get_topic_info", label: "Conversation info" },
  { id: "send_message", label: "Send message" },
  { id: "get_my_info", label: "My profile" },
  { id: "list_my_groups", label: "My groups" },
  { id: "get_group_members", label: "Group members" },
];

const emptyInput: AgentInput = { name: "", description: "", instruction: "", model: "deepseek-chat", tools: ["search_messages", "get_conversation_context"], base_url: "", api_key: "", max_tokens: 4096, mcp_bindings: [], skills: [] };

const MCP_ADMIN_NOTICE =
  "MCP servers are managed by platform administrators (/v1/mcp/servers requires admin privileges), so MCP tools cannot be bound here.";

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

export default function AgentsPage() {
  const router = useRouter();
  const token = useAuthStore((s) => s.token);
  const currentUserId = useAuthStore((s) => s.user?.userId) || 0;
  const [agents, setAgents] = useState<ManagedAgent[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [selected, setSelected] = useState<ManagedAgent | null>(null);
  const [tab, setTab] = useState<"config" | "sharing" | "runs" | "platform">("config");
  const [draft, setDraft] = useState<AgentInput>(emptyInput);
  const [createOpen, setCreateOpen] = useState(false);
  const [createInput, setCreateInput] = useState<AgentInput>(emptyInput);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [shares, setShares] = useState<AgentShare[]>([]);
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [shareKind, setShareKind] = useState<AgentShare["kind"]>("user");
  const [shareTarget, setShareTarget] = useState("");
  const [userResults, setUserResults] = useState<{ userId: number; username: string }[]>([]);
  const [groups, setGroups] = useState<{ groupId: string; name: string }[]>([]);
  // Display-name maps for share targets. There is no user-by-id lookup API,
  // so user names come from locally known conversations/members plus names
  // captured at grant time; group names resolve via the groups API.
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

  const refreshSelected = useCallback((id: number) => {
    return agentApi.detail(id).then((detail) => {
      setSelected(detail);
      const source = detail.draft;
      if (source) setDraft({ name: detail.name, description: detail.description, instruction: source.instruction, model: source.model, tools: source.tools ?? [], base_url: source.baseUrl ?? "", api_key: "", max_tokens: source.maxTokens || 4096, mcp_bindings: source.mcpBindings ?? [], skills: source.skills ?? [] });
    });
  }, []);

  const [mcpServers, setMcpServers] = useState<{ name: string; tools: string[] }[]>([]);
  const [mcpForbidden, setMcpForbidden] = useState(false);
  const [publishedSkills, setPublishedSkills] = useState<{ name: string; version: number }[]>([]);

  const refreshExtensions = useCallback(() => {
    agentApi.listSkills().then((data) => setPublishedSkills(data.skills.map((s) => ({ name: s.name, version: s.version })))).catch(() => setPublishedSkills([]));
    agentApi.listMCPServers().then((data) => { setMcpForbidden(false); setMcpServers(data.servers.filter((s) => s.enabled).map((s) => ({ name: s.name, tools: s.tools }))); }).catch((err) => {
      if (err instanceof AppError && err.status === 403) setMcpForbidden(true);
      else setMcpServers([]);
    });
  }, []);

  useEffect(() => {
    refreshExtensions();
  }, [refreshExtensions]);

  useEffect(() => {
    if (!token && !localStorage.getItem("token")) { router.replace("/login"); return; }
    refreshList().catch((err) => toast.error(errorText(err))).finally(() => setLoading(false));
  }, [router, token, refreshList]);

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

  // Runs poll every 5s while the tab is active; unmount/tab switch clears it.
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

  // Share display-name resolution. There is no user-by-id lookup API, so
  // user names are derived from data the client already holds (p2p
  // conversations, group member tables) plus names captured at grant time;
  // group names come from the groups API (local list + per-id fallback).
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
    try { await task(); toast.success(success); await refreshList(); if (selectedId) await refreshSelected(selectedId); }
    catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const create = () => act(async () => {
    const row = await agentApi.create(createInput);
    invalidateAgentNames();
    setCreateOpen(false);
    setCreateInput(emptyInput);
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

  const toggleTool = (input: AgentInput, set: (next: AgentInput) => void, tool: AgentTool) =>
    set({ ...input, tools: input.tools.includes(tool) ? input.tools.filter((item) => item !== tool) : [...input.tools, tool] });

  const toggleMCPTool = (input: AgentInput, set: (next: AgentInput) => void, server: string, tool: string) => {
    const bindings = input.mcp_bindings ?? [];
    const exists = bindings.some((b) => b.server === server && b.tool === tool);
    set({ ...input, mcp_bindings: exists ? bindings.filter((b) => !(b.server === server && b.tool === tool)) : [...bindings, { server, tool }] });
  };

  const toggleSkill = (input: AgentInput, set: (next: AgentInput) => void, name: string, version: number) => {
    const bindings = input.skills ?? [];
    const exists = bindings.some((b) => b.name === name);
    set({ ...input, skills: exists ? bindings.filter((b) => b.name !== name) : [...bindings.filter((b) => b.name !== name), { name, version }] });
  };

  const editor = (input: AgentInput, set: (next: AgentInput) => void, options?: { isCreate?: boolean; hasApiKey?: boolean }) => (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block text-sm font-medium">Name<Input className="mt-1" value={input.name} maxLength={100} onChange={(e) => set({ ...input, name: e.target.value })} /></label>
        <label className="block text-sm font-medium">Model<Input className="mt-1" value={input.model} maxLength={100} placeholder="deepseek-chat" onChange={(e) => set({ ...input, model: e.target.value })} /></label>
      </div>
      <label className="block text-sm font-medium">Description<Input className="mt-1" value={input.description} maxLength={4000} onChange={(e) => set({ ...input, description: e.target.value })} /></label>
      <div className="grid gap-4 sm:grid-cols-3">
        <label className="block text-sm font-medium">Base URL<Input className="mt-1" value={input.base_url ?? ""} maxLength={200} placeholder="https://api.deepseek.com/v1" onChange={(e) => set({ ...input, base_url: e.target.value })} /></label>
        <label className="block text-sm font-medium">API Key<Input className="mt-1" type="password" value={input.api_key ?? ""} maxLength={512} placeholder={options?.isCreate ? "Optional: model API key (encrypted at rest)" : "Leave blank to keep unchanged"} onChange={(e) => set({ ...input, api_key: e.target.value })} />
          {!options?.isCreate && (
            <span className="mt-1 block text-xs font-normal text-muted-foreground">
              {options?.hasApiKey ? "API key stored — leave blank to keep it" : "No API key set"}
            </span>
          )}
        </label>
        <label className="block text-sm font-medium">Max output tokens<Input className="mt-1" type="number" min={1} max={128000} value={input.max_tokens ?? 4096} onChange={(e) => set({ ...input, max_tokens: Number(e.target.value) || 0 })} /></label>
      </div>
      <label className="block text-sm font-medium">Instructions<textarea className="mt-1 min-h-48 w-full resize-y rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring" value={input.instruction} maxLength={20000} onChange={(e) => set({ ...input, instruction: e.target.value })} /></label>
      <fieldset className="space-y-2"><legend className="text-sm font-medium">Tools</legend>{toolNames.map((tool) => <label key={tool.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={input.tools.includes(tool.id)} onChange={() => toggleTool(input, set, tool.id)} />{tool.label}</label>)}</fieldset>
      {mcpServers.length > 0 && <fieldset className="space-y-2"><legend className="text-sm font-medium">MCP tools</legend>{mcpServers.map((server) => <div key={server.name} className="text-sm"><div className="font-medium">{server.name}</div>{server.tools.map((tool) => <label key={tool} className="flex items-center gap-2 pl-3"><input type="checkbox" checked={(input.mcp_bindings ?? []).some((b) => b.server === server.name && b.tool === tool)} onChange={() => toggleMCPTool(input, set, server.name, tool)} />{tool}</label>)}</div>)}</fieldset>}
      {mcpForbidden && <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-400">{MCP_ADMIN_NOTICE}</p>}
      {publishedSkills.length > 0 && <fieldset className="space-y-2"><legend className="text-sm font-medium">Skills</legend>{publishedSkills.map((skill) => <label key={`${skill.name}@${skill.version}`} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={(input.skills ?? []).some((b) => b.name === skill.name)} onChange={() => toggleSkill(input, set, skill.name, skill.version)} />{skill.name} v{skill.version}</label>)}</fieldset>}
    </div>
  );

  if (loading) return <div className="flex min-h-screen items-center justify-center"><Loader2 className="h-5 w-5 animate-spin" /></div>;

  return (
    <div className="min-h-screen bg-background">
      <header className="flex h-14 items-center justify-between border-b px-4 sm:px-6">
        <div className="flex items-center gap-3"><Button variant="ghost" size="icon" title="Back to chat" onClick={() => router.push("/chat")}><ArrowLeft /></Button><Bot className="h-5 w-5 text-primary" /><h1 className="text-base font-semibold">Agents</h1></div>
        <Button size="sm" onClick={() => setCreateOpen(true)}><Plus /> New agent</Button>
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
          <div className="mb-5 flex gap-5 border-b text-sm">{(["config", "sharing", "runs", "platform"] as const).filter((item) => item === "config" || item === "platform" || selected?.canManage).map((item) => <button key={item} onClick={() => setTab(item)} className={`border-b-2 pb-2 capitalize ${tab === item ? "border-primary font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground"}`}>{item === "config" ? "Configuration" : item === "sharing" ? "Sharing" : item === "runs" ? "Runs" : "Platform"}</button>)}</div>
          {tab === "platform" ? <PlatformPanel onChanged={refreshExtensions} /> : !selected ? <div className="py-20 text-center text-sm text-muted-foreground">No agents yet</div> : <>
            <div className="mb-5 flex flex-wrap items-start justify-between gap-3"><div><div className="flex items-center gap-2"><h2 className="text-xl font-semibold">{selected.name}</h2><Badge variant="secondary">{selected.status}</Badge><Button size="sm" variant="outline" aria-disabled={selected.currentVersion === 0} className={cn(selected.currentVersion === 0 && "cursor-not-allowed opacity-50")} onClick={chatWithBot}><MessageSquare /> Chat</Button></div><p className="mt-1 text-sm text-muted-foreground">Bot #{selected.botUserId} · Version {selected.currentVersion || "unpublished"}</p></div>{selected.canManage && selected.currentVersion > 0 && <Button size="sm" variant={selected.status === "active" ? "outline" : "default"} disabled={busy} onClick={() => act(() => agentApi.setStatus(selected.id, selected.status === "active" ? "disabled" : "active"), selected.status === "active" ? "Agent disabled" : "Agent enabled")}>{selected.status === "active" ? "Disable" : "Enable"}</Button>}</div>
            {tab === "config" && (selected.canManage && selected.draft ? <div className="max-w-3xl">{editor(draft, setDraft, { hasApiKey: selected.draft.hasApiKey ?? false })}<div className="mt-5 flex flex-wrap gap-2"><Button variant="outline" disabled={busy || !draft.name.trim() || !draft.instruction.trim()} onClick={() => saveDraftAction(false)}><Save /> Save draft</Button><Button disabled={busy || !draft.name.trim() || !draft.instruction.trim()} onClick={() => saveDraftAction(true)}><Send /> Publish</Button></div></div> : <div className="max-w-3xl space-y-3 text-sm"><p>{selected.description || "No description"}</p><div><div className="mb-1 font-medium">Published instructions</div><p className="whitespace-pre-wrap text-muted-foreground">{selected.published?.instruction || "No published version"}</p></div><p className="text-xs text-muted-foreground">Model: {selected.published?.model || "-"} · API key: {selected.published?.hasApiKey ? "set" : "not set"}</p></div>)}
            {tab === "sharing" && selected.canManage && <div className="max-w-2xl space-y-5"><div className="flex flex-wrap gap-2"><select aria-label="Share type" value={shareKind} onChange={(e) => { setShareKind(e.target.value as AgentShare["kind"]); setShareTarget(""); }} className="h-8 rounded-md border bg-background px-2 text-sm"><option value="user">User</option><option value="group">Group</option></select>{shareKind === "group" ? <select aria-label="Group" value={shareTarget} onChange={(e) => setShareTarget(e.target.value)} className="h-8 min-w-52 rounded-md border bg-background px-2 text-sm"><option value="">Select group</option>{groups.map((group) => <option key={group.groupId} value={group.groupId}>{group.name}</option>)}</select> : <div className="relative min-w-52 flex-1"><Input placeholder="Search users" value={shareTarget} onChange={(e) => setShareTarget(e.target.value)} />{userResults.length > 0 && <div className="absolute z-10 mt-1 max-h-40 w-full overflow-auto rounded-md border bg-background shadow-sm">{userResults.map((user) => <button key={user.userId} className="block w-full px-3 py-2 text-left text-sm hover:bg-muted" onClick={() => { setShareTarget(String(user.userId)); setShareUserNames((old) => new Map(old).set(Number(user.userId), user.username)); setUserResults([]); }}>{user.username} · #{user.userId}</button>)}</div>}</div>}<Button disabled={busy || !shareTarget} onClick={() => act(async () => { await agentApi.share(selected.id, shareKind, shareTarget, true); setShares((old) => [...old.filter((s) => !(s.kind === shareKind && s.target === shareTarget)), { kind: shareKind, target: shareTarget }]); setShareTarget(""); }, "Access granted")}><Check /> Share</Button></div><div className="divide-y border-y">{shares.length === 0 && <p className="py-4 text-sm text-muted-foreground">Private · owner only</p>}{shares.map((share) => <div key={`${share.kind}:${share.target}`} className="flex items-center justify-between py-2 text-sm"><span>{share.kind === "group" ? (shareGroupNames.get(share.target) ?? groups.find((g) => g.groupId === share.target)?.name ?? `Group · ${share.target}`) : (shareUserNames.get(Number(share.target)) ?? baseShareUserNames.get(Number(share.target)) ?? `User · ${share.target}`)}</span><Button variant="ghost" size="icon" title="Remove access" disabled={busy} onClick={() => act(async () => { await agentApi.share(selected.id, share.kind, share.target, false); setShares((old) => old.filter((s) => s !== share)); }, "Access removed")}><Trash2 /></Button></div>)}</div></div>}
            {tab === "runs" && selected.canManage && <div><div className="mb-3 flex justify-end"><Button variant="ghost" size="icon" title="Refresh runs" onClick={() => agentApi.runs(selected.id).then((data) => setRuns(data.items)).catch((err) => toast.error(errorText(err)))}><RefreshCw /></Button></div><div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="border-b text-xs text-muted-foreground"><tr><th className="py-2">Run</th><th>Actor</th><th>Topic</th><th>Status</th><th>Error</th><th>Tokens</th><th>Duration</th><th>Created</th><th className="text-right">Action</th></tr></thead><tbody>{runs.map((run) => <Fragment key={run.id}>
              <tr className="cursor-pointer border-b hover:bg-muted/40" onClick={() => void toggleRun(run.id)}><td className="py-3">#{run.id}</td><td>#{run.actorUserId}</td><td className="max-w-40 truncate" title={run.topic}>{run.topic}</td><td><Badge variant={run.status === "failed" ? "destructive" : "secondary"}>{run.status}</Badge></td><td>{run.errorCode ? <span className="text-destructive">{run.errorCode}</span> : "-"}</td><td>{run.totalTokens}</td><td>{formatDuration(run.startedAt, run.finishedAt)}</td><td>{new Date(run.createdAt).toLocaleString()}</td><td className="text-right" onClick={(e) => e.stopPropagation()}>{run.status === "failed" && <div className="flex justify-end gap-1"><Button size="xs" variant="outline" disabled={busy} onClick={() => act(async () => { await agentApi.runAction(run.id, "retry"); setRuns((await agentApi.runs(selected.id)).items); }, "Run queued")}>Retry</Button><Button size="xs" variant="ghost" disabled={busy} onClick={() => act(async () => { await agentApi.runAction(run.id, "skip"); setRuns((await agentApi.runs(selected.id)).items); }, "Run skipped")}>Skip</Button></div>}</td></tr>
              {expandedRun === run.id && <tr className="border-b bg-muted/40"><td colSpan={9} className="px-4 py-3 text-xs">{detailLoading && !runDetail ? <span className="flex items-center gap-2"><Loader2 className="h-3.5 w-3.5 animate-spin" />Loading run details…</span> : <dl className="grid grid-cols-2 gap-x-6 gap-y-1 sm:grid-cols-4"><div><dt className="text-muted-foreground">Attempts</dt><dd>{runDetail?.attempts ?? "-"}</dd></div><div><dt className="text-muted-foreground">Agent version</dt><dd>{runDetail?.agentVersion ? `v${runDetail.agentVersion}` : "-"}</dd></div><div><dt className="text-muted-foreground">Reply message</dt><dd>{runDetail?.replyMsgId ? `#${runDetail.replyMsgId}` : "-"}</dd></div><div><dt className="text-muted-foreground">Error code</dt><dd>{runDetail?.errorCode || run.errorCode || "-"}</dd></div><div><dt className="text-muted-foreground">Input tokens</dt><dd>{runDetail?.inputTokens ?? run.inputTokens}</dd></div><div><dt className="text-muted-foreground">Output tokens</dt><dd>{runDetail?.outputTokens ?? run.outputTokens}</dd></div><div><dt className="text-muted-foreground">Started</dt><dd>{runDetail?.startedAt ? new Date(runDetail.startedAt).toLocaleString() : "-"}</dd></div><div><dt className="text-muted-foreground">Finished</dt><dd>{runDetail?.finishedAt ? new Date(runDetail.finishedAt).toLocaleString() : "-"}</dd></div></dl>}</td></tr>}
            </Fragment>)}</tbody></table>{runs.length === 0 && <p className="py-8 text-center text-sm text-muted-foreground">No runs</p>}</div></div>}
          </>}
        </main>
      </div>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}><DialogContent className="max-h-[90vh] overflow-auto sm:max-w-2xl"><DialogHeader><DialogTitle>New agent</DialogTitle></DialogHeader>{editor(createInput, setCreateInput, { isCreate: true })}<div className="flex justify-end"><Button disabled={busy || !createInput.name.trim() || !createInput.instruction.trim()} onClick={create}>{busy ? <Loader2 className="animate-spin" /> : <Plus />} Create agent</Button></div></DialogContent></Dialog>
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}><DialogContent><DialogHeader><DialogTitle>Delete agent</DialogTitle></DialogHeader><p className="text-sm text-muted-foreground">Delete <span className="font-medium text-foreground">{deleteTarget?.name}</span>? Its versions and shares are removed; the bot account stays as an inert user. This cannot be undone.</p><div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancel</Button><Button variant="destructive" disabled={busy} onClick={() => void doDelete()}>{busy ? <Loader2 className="animate-spin" /> : <Trash2 />}Delete</Button></div></DialogContent></Dialog>
    </div>
  );
}
