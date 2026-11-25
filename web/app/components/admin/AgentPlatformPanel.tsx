"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import {
  CirclePlus,
  Cpu,
  ExternalLink,
  PackagePlus,
  Power,
  PowerOff,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import {
  agentApi,
  type Model,
  type MCPServer,
  type Skill,
  type SkillVersion,
  type CapabilityPreset,
  type ToolDefinition,
} from "@/lib/api/agents";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

const BIFROST_DASHBOARD_URL =
  process.env.NEXT_PUBLIC_BIFROST_DASHBOARD_URL || "http://localhost:8088";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

interface PlatformPanelProps {
  catalog: {
    models: Model[];
    presets: CapabilityPreset[];
    tools: ToolDefinition[];
    mcps: MCPServer[];
    skills: Skill[];
  };
  onChanged?: () => void;
}

export function AgentPlatformPanel({ catalog, onChanged }: PlatformPanelProps) {
  const [models, setModels] = useState<Model[]>(catalog.models);
  const [canManage, setCanManage] = useState(false);
  const [servers, setServers] = useState<MCPServer[]>(catalog.mcps);
  const [skills, setSkills] = useState<Skill[]>(catalog.skills);
  const [busy, setBusy] = useState(false);

  const [serverName, setServerName] = useState("");
  const [serverDisplay, setServerDisplay] = useState("");
  const [serverURL, setServerURL] = useState("");
  const [serverAuth, setServerAuth] = useState<"none" | "bearer" | "header">("none");
  const [serverHeader, setServerHeader] = useState("");
  const [serverCredential, setServerCredential] = useState("");
  const [serverVisibility, setServerVisibility] = useState<"private" | "shared" | "public">("private");
  const [connectionInputs, setConnectionInputs] = useState<Record<number, string>>({});

  const [skillSlug, setSkillSlug] = useState("");
  const [skillDisplay, setSkillDisplay] = useState("");
  const [skillVisibility, setSkillVisibility] = useState<"private" | "shared" | "public">("private");
  const [skillContent, setSkillContent] = useState("");
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [skillVersions, setSkillVersions] = useState<SkillVersion[]>([]);
  const [newSkillContent, setNewSkillContent] = useState("");

  const load = useCallback(async () => {
    const [svc, mcp, sk] = await Promise.allSettled([
      agentApi.listModels(),
      agentApi.listMCPServers(),
      agentApi.listSkills(),
    ]);
    if (svc.status === "fulfilled") {
      setModels(svc.value.models);
      setCanManage(svc.value.canManage);
    }
    if (mcp.status === "fulfilled") setServers(mcp.value.servers);
    if (sk.status === "fulfilled") setSkills(sk.value.skills);
  }, []);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { load(); }, [load]);

  const refresh = async () => { await load(); onChanged?.(); };

  const registerServer = async () => {
    setBusy(true);
    try {
      await agentApi.registerMCPServer({
        name: serverName.trim(), displayName: serverDisplay.trim() || undefined, url: serverURL.trim(),
        transport: "streamable_http", authType: serverAuth,
        headerName: serverAuth === "header" ? serverHeader.trim() : undefined,
        credential: serverAuth === "none" ? undefined : serverCredential,
        visibility: serverVisibility, enabled: true,
      });
      toast.success("MCP server registered");
      setServerName(""); setServerDisplay(""); setServerURL(""); setServerCredential("");
      await refresh();
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const act = async (task: () => Promise<unknown>, success: string) => {
    setBusy(true);
    try { await task(); toast.success(success); await refresh(); }
    catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const connectServer = async (server: MCPServer) => {
    const credential = connectionInputs[server.id] ?? "";
    if (!credential) { toast.error("Credential is required"); return; }
    await act(() => agentApi.connectMCPServer(server.id, { authType: server.authType, credential }), "MCP connection saved");
  };

  const createSkill = async () => {
    setBusy(true);
    try {
      await agentApi.createSkill({ slug: skillSlug.trim(), displayName: skillDisplay.trim() || undefined, visibility: skillVisibility, content: skillContent });
      toast.success("Skill created");
      setSkillSlug(""); setSkillDisplay(""); setSkillContent("");
      await refresh();
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const openSkill = async (skill: Skill) => {
    try {
      const detail = await agentApi.getSkill(skill.id);
      setSelectedSkill(detail);
      setSkillVersions(detail.versions ?? []);
      setNewSkillContent("");
    } catch (err) { toast.error(errorText(err)); }
  };

  const addSkillVersion = async () => {
    if (!selectedSkill) return;
    setBusy(true);
    try {
      await agentApi.addSkillVersion(selectedSkill.id, newSkillContent);
      toast.success("Skill version added");
      setNewSkillContent("");
      await openSkill(selectedSkill);
      await refresh();
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  return (
    <div className="max-w-4xl space-y-10">
      <section className="space-y-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><Cpu className="h-4 w-4 text-primary" />LLM gateway</h3>
        <div className="flex flex-wrap items-start justify-between gap-3 rounded-lg border p-3 text-sm">
          <div className="space-y-1">
            <p className="font-medium">Bifrost is the single model gateway.</p>
            <p className="text-xs text-muted-foreground">
              Provider keys, routing, budgets and virtual keys are managed in
              Bifrost. This console only shows the model catalog Nonoka can use.
            </p>
          </div>
          <a
            href={BIFROST_DASHBOARD_URL}
            target="_blank"
            rel="noopener noreferrer"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            <ExternalLink /> Open Bifrost
          </a>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">
            Only enabled + curated models can be selected by agents. Unknown
            models stay disabled until an operator curates them.
          </span>
          {canManage && (
            <Button
              size="xs"
              variant="outline"
              disabled={busy}
              onClick={() => act(async () => { await agentApi.syncModels(); }, "Model catalog synced")}
            >
              <RefreshCw /> Sync from Bifrost
            </Button>
          )}
        </div>
        <div className="divide-y rounded-lg border">
          {models.length === 0 ? (
            <p className="py-4 text-center text-xs text-muted-foreground">
              No models available. Configure a provider in Bifrost, then sync the
              catalog.
            </p>
          ) : (
            models.map((model) => (
              <div key={model.id} className="flex flex-wrap items-center gap-2 px-3 py-2 text-sm">
                {model.icon && <span aria-hidden>{model.icon}</span>}
                <span className="font-medium">{model.displayName || model.modelKey}</span>
                <span className="text-[10px] text-muted-foreground">{model.provider}</span>
                <Badge variant="secondary" className={cn("text-[10px]", model.enabled ? "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400" : "")}>
                  {model.enabled ? "enabled" : "disabled"}
                </Badge>
                <Badge variant="secondary" className="text-[10px]">{model.curated ? "curated" : "uncurated"}</Badge>
                {model.supportsVision && <Badge variant="secondary" className="text-[10px]">vision</Badge>}
                {model.recommended && <Badge variant="secondary" className="text-[10px]">recommended</Badge>}
                <span className="ml-auto text-[10px] text-muted-foreground">
                  ${model.inputPricePerMtok.toFixed(2)} / ${model.outputPricePerMtok.toFixed(2)} per Mtok
                </span>
                {canManage && (
                  <div className="flex gap-1">
                    <Button
                      size="xs"
                      variant={model.enabled ? "ghost" : "outline"}
                      disabled={busy}
                      onClick={() => act(() => agentApi.updateModel(model.id, { enabled: !model.enabled }), model.enabled ? "Model disabled" : "Model enabled")}
                    >
                      {model.enabled ? <PowerOff /> : <Power />}
                      {model.enabled ? "Disable" : "Enable"}
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => act(() => agentApi.updateModel(model.id, { supportsVision: !model.supportsVision }), "Vision capability updated")}
                    >
                      {model.supportsVision ? "No vision" : "Vision"}
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => {
                        const name = window.prompt("Model display name", model.displayName);
                        if (name && name.trim()) {
                          void act(() => agentApi.updateModel(model.id, { displayName: name.trim() }), "Model renamed");
                        }
                      }}
                    >
                      Rename
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => {
                        const icon = window.prompt("Model icon (emoji or short label)", model.icon || "");
                        if (icon !== null) {
                          void act(() => agentApi.updateModel(model.id, { icon }), "Model icon updated");
                        }
                      }}
                    >
                      Icon
                    </Button>
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </section>

      <section className="space-y-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><CirclePlus className="h-4 w-4 text-primary" />MCP servers</h3>
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">Quick add:</span>
          <Button size="xs" variant="outline" onClick={() => { setServerName("deepwiki"); setServerDisplay("DeepWiki"); setServerURL("https://mcp.deepwiki.com/mcp"); setServerAuth("none"); setServerVisibility("private"); }}>DeepWiki</Button>
          <Button size="xs" variant="outline" onClick={() => { setServerName("context7"); setServerDisplay("Context7"); setServerURL("https://mcp.context7.com/mcp"); setServerAuth("none"); setServerVisibility("private"); }}>Context7</Button>
          <span className="text-muted-foreground">Context7 works without a key at low limits; add a header credential for higher limits.</span>
        </div>
        <div className="space-y-2 rounded-lg border p-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block text-xs font-medium">Name<Input className="mt-1" value={serverName} onChange={(e) => setServerName(e.target.value)} placeholder="my-search" /></label>
            <label className="block text-xs font-medium">Display name<Input className="mt-1" value={serverDisplay} onChange={(e) => setServerDisplay(e.target.value)} /></label>
          </div>
          <label className="block text-xs font-medium">URL<Input className="mt-1" value={serverURL} onChange={(e) => setServerURL(e.target.value)} placeholder="https://mcp.example.com/mcp" /></label>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block text-xs font-medium">Auth
              <select className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm" value={serverAuth} onChange={(e) => setServerAuth(e.target.value as typeof serverAuth)}>
                <option value="none">none</option><option value="bearer">bearer</option><option value="header">header</option>
              </select>
            </label>
            <label className="block text-xs font-medium">Visibility
              <select className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm" value={serverVisibility} onChange={(e) => setServerVisibility(e.target.value as typeof serverVisibility)}>
                <option value="private">private</option><option value="shared">shared</option><option value="public">public</option>
              </select>
            </label>
            {serverAuth === "header" && <label className="block text-xs font-medium">Header name<Input className="mt-1" value={serverHeader} onChange={(e) => setServerHeader(e.target.value)} placeholder="X-API-Key" /></label>}
          </div>
          {serverAuth !== "none" && <label className="block text-xs font-medium">Credential (write-only)<Input className="mt-1" type="password" value={serverCredential} onChange={(e) => setServerCredential(e.target.value)} /></label>}
          <div className="flex justify-end"><Button size="sm" disabled={busy || !serverName.trim() || !serverURL.trim()} onClick={() => void registerServer()}><CirclePlus />Register server</Button></div>
        </div>
        <div className="divide-y rounded-lg border">
          {servers.length === 0 ? <p className="py-4 text-center text-xs text-muted-foreground">No MCP servers</p> : servers.map((server) => (
            <div key={server.id} className="space-y-2 px-3 py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{server.displayName || server.name}</span>
                <Badge variant="secondary" className="text-[10px]">{server.visibility}</Badge>
                <Badge variant="secondary" className={cn("text-[10px]", server.reviewStatus === "approved" && "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400")}>{server.reviewStatus}</Badge>
                <Badge variant="secondary" className="text-[10px]">{server.healthStatus}</Badge>
                <span className="truncate text-xs text-muted-foreground" title={server.url}>{server.url}</span>
                <div className="ml-auto flex gap-1">
                  <Button size="xs" variant="outline" disabled={busy} onClick={() => void act(() => agentApi.discoverMCPServer(server.id), "Tools discovered")}><RefreshCw />Discover</Button>
                  <Button size="xs" variant="outline" disabled={busy} onClick={() => void act(() => agentApi.testMCPServer(server.id), "Health check passed")}><ShieldCheck />Test</Button>
                  <Button size="xs" variant="outline" title={server.enabled ? "Disable server" : "Enable server"} disabled={busy} onClick={() => void act(() => agentApi.setMCPServerStatus(server.id, !server.enabled), server.enabled ? "Server disabled" : "Server enabled")}>{server.enabled ? <PowerOff /> : <Power />}</Button>
                  {canManage && server.reviewStatus === "pending" && <>
                    <Button size="xs" disabled={busy} onClick={() => void act(() => agentApi.reviewMCPServer(server.id, "approved"), "Server approved")}>Approve</Button>
                    <Button size="xs" variant="outline" disabled={busy} onClick={() => void act(() => agentApi.reviewMCPServer(server.id, "rejected"), "Server rejected")}>Reject</Button>
                  </>}
                  <Button size="xs" variant="ghost" title="Delete server" disabled={busy} onClick={() => void act(() => agentApi.deleteMCPServer(server.id), "Server deleted")}><Trash2 /></Button>
                </div>
              </div>
              {server.tools && server.tools.length > 0 && <div className="flex flex-wrap gap-1">{server.tools.map((tool) => <Badge key={tool.name} variant="secondary" className="text-[10px]">{tool.name} · {tool.risk}</Badge>)}</div>}
              {server.authType !== "none" && (
                <div className="flex items-end gap-2">
                  <label className="block text-xs font-medium">My connection credential<Input className="mt-1" type="password" value={connectionInputs[server.id] ?? ""} onChange={(e) => setConnectionInputs((old) => ({ ...old, [server.id]: e.target.value }))} /></label>
                  <Button size="xs" variant="outline" disabled={busy} onClick={() => void connectServer(server)}>Save connection</Button>
                  <Button size="xs" variant="ghost" disabled={busy} onClick={() => void act(() => agentApi.disconnectMCPServer(server.id), "Connection removed")}>Disconnect</Button>
                </div>
              )}
            </div>
          ))}
        </div>
      </section>

      <section className="space-y-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><PackagePlus className="h-4 w-4 text-primary" />Skills</h3>
        <div className="space-y-2 rounded-lg border p-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block text-xs font-medium">Slug<Input className="mt-1" value={skillSlug} onChange={(e) => setSkillSlug(e.target.value)} placeholder="code-review" /></label>
            <label className="block text-xs font-medium">Display name<Input className="mt-1" value={skillDisplay} onChange={(e) => setSkillDisplay(e.target.value)} /></label>
            <label className="block text-xs font-medium">Visibility
              <select className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm" value={skillVisibility} onChange={(e) => setSkillVisibility(e.target.value as typeof skillVisibility)}>
                <option value="private">private</option><option value="shared">shared</option><option value="public">public</option>
              </select>
            </label>
          </div>
          <label className="block text-xs font-medium">Content (SKILL.md)
            <textarea className="mt-1 min-h-32 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring" value={skillContent} onChange={(e) => setSkillContent(e.target.value)} placeholder={"---\nname: code-review\ndescription: Review changed code\n---\n\nReview the changes…"} />
          </label>
          <div className="flex justify-end"><Button size="sm" disabled={busy || !skillSlug.trim() || !skillContent.trim()} onClick={() => void createSkill()}><PackagePlus />Create skill</Button></div>
        </div>
        <div className="divide-y rounded-lg border">
          {skills.length === 0 ? <p className="py-4 text-center text-xs text-muted-foreground">No skills</p> : skills.map((skill) => (
            <div key={skill.id} className="px-3 py-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <button className="font-medium hover:underline" onClick={() => void openSkill(skill)}>{skill.displayName || skill.slug}</button>
                <Badge variant="secondary" className="text-[10px]">v{skill.latestVersion} · {skill.visibility}</Badge>
                <Badge variant="secondary" className={cn("text-[10px]", skill.reviewStatus === "approved" && "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400")}>{skill.reviewStatus}</Badge>
                {canManage && skill.reviewStatus === "pending" && <>
                  <Button size="xs" disabled={busy} onClick={() => void act(() => agentApi.reviewSkill(skill.id, true), "Skill approved")}>Approve skill</Button>
                  <Button size="xs" variant="outline" disabled={busy} onClick={() => void act(() => agentApi.reviewSkill(skill.id, false), "Skill rejected")}>Reject</Button>
                </>}
                <Button size="xs" variant="ghost" className="ml-auto" disabled={busy} onClick={() => void act(() => agentApi.deleteSkill(skill.id), "Skill deleted")}><Trash2 /></Button>
              </div>
            </div>
          ))}
        </div>
        {selectedSkill && (
          <div className="space-y-3 rounded-lg border p-3">
            <div className="flex items-center justify-between"><h4 className="text-sm font-semibold">{selectedSkill.displayName || selectedSkill.slug} versions</h4><Button size="xs" variant="ghost" onClick={() => setSelectedSkill(null)}>Close</Button></div>
            <div className="divide-y rounded-md border">
              {skillVersions.map((version) => (
                <div key={version.id} className="flex flex-wrap items-center gap-2 px-2 py-1.5 text-xs">
                  <span className="font-medium">v{version.version}</span>
                  <span className="text-muted-foreground">{version.checksum.slice(0, 10)}…</span>
                  <Badge variant="secondary" className="text-[10px]">{version.reviewStatus}</Badge>
                  {canManage && version.reviewStatus === "pending" && <>
                    <Button size="xs" onClick={() => void act(() => agentApi.reviewSkillVersion(selectedSkill.id, version.version, true), "Version approved")}>Approve</Button>
                    <Button size="xs" variant="outline" onClick={() => void act(() => agentApi.reviewSkillVersion(selectedSkill.id, version.version, false), "Version rejected")}>Reject</Button>
                  </>}
                  {version.content && <details className="w-full"><summary className="cursor-pointer text-muted-foreground">Preview content</summary><pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap text-[11px]">{version.content}</pre></details>}
                </div>
              ))}
            </div>
            <label className="block text-xs font-medium">New version content
              <textarea className="mt-1 min-h-28 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring" value={newSkillContent} onChange={(e) => setNewSkillContent(e.target.value)} />
            </label>
            <div className="flex justify-end"><Button size="sm" disabled={busy || !newSkillContent.trim()} onClick={() => void addSkillVersion()}>Add version</Button></div>
          </div>
        )}
      </section>
    </div>
  );
}
