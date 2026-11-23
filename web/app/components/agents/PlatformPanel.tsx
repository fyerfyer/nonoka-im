"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { CirclePlus, PackagePlus, Power, PowerOff, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import {
  agentApi,
  type ModelService,
  type MCPServer,
  type Skill,
  type SkillVersion,
  type CapabilityPreset,
  type ToolDefinition,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

interface PlatformPanelProps {
  catalog: {
    services: ModelService[];
    presets: CapabilityPreset[];
    tools: ToolDefinition[];
    mcps: MCPServer[];
    skills: Skill[];
  };
  onChanged?: () => void;
}

export function PlatformPanel({ catalog, onChanged }: PlatformPanelProps) {
  const [services, setServices] = useState<ModelService[]>(catalog.services);
  const [canManage, setCanManage] = useState(false);
  const [servers, setServers] = useState<MCPServer[]>(catalog.mcps);
  const [skills, setSkills] = useState<Skill[]>(catalog.skills);
  const [busy, setBusy] = useState(false);
  const [gatewayModels, setGatewayModels] = useState<{ name: string; provider: string }[]>([]);
  const [selectedProviderModels, setSelectedProviderModels] = useState<Record<string, string[]>>({});

  const [serviceName, setServiceName] = useState("");
  const [serviceDisplay, setServiceDisplay] = useState("");
  const [gatewayType, setGatewayType] = useState<"bifrost" | "openai_compatible">("bifrost");
  const [serviceBaseURL, setServiceBaseURL] = useState("");
  const [serviceKey, setServiceKey] = useState("");
  const [allowAll, setAllowAll] = useState(true);
  const [modelServiceId, setModelServiceId] = useState<number | null>(null);
  const [modelKey, setModelKey] = useState("");
  const [modelDisplay, setModelDisplay] = useState("");

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
      agentApi.listModelServices(),
      agentApi.listMCPServers(),
      agentApi.listSkills(),
    ]);
    if (svc.status === "fulfilled") {
      setServices(svc.value.services);
      setCanManage(svc.value.canManage);
    }
    if (mcp.status === "fulfilled") setServers(mcp.value.servers);
    if (sk.status === "fulfilled") setSkills(sk.value.skills);
  }, []);

  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { load(); }, [load]);

  const refresh = async () => { await load(); onChanged?.(); };

  const loadGatewayModels = async () => {
    try {
      const data = await agentApi.listGatewayModels();
      setGatewayModels(data.models);
      toast.success(`Loaded ${data.models.length} gateway models`);
    } catch (err) { toast.error(errorText(err)); }
  };

  const toggleProviderModel = (provider: string, name: string) => {
    setAllowAll(false);
    setSelectedProviderModels((old) => {
      const current = new Set(old[provider] ?? []);
      if (current.has(name)) current.delete(name); else current.add(name);
      return { ...old, [provider]: [...current] };
    });
  };

  const createService = async () => {
    setBusy(true);
    try {
      const providerModels = Object.fromEntries(Object.entries(selectedProviderModels).filter(([, models]) => models.length > 0));
      await agentApi.createModelService({
        name: serviceName.trim(),
        displayName: serviceDisplay.trim() || undefined,
        gatewayType,
        baseUrl: gatewayType === "openai_compatible" ? serviceBaseURL.trim() : undefined,
        apiKey: serviceAPIKey(gatewayType, serviceKey),
        allowAll: gatewayType === "bifrost" ? allowAll || Object.keys(providerModels).length === 0 : false,
        providerModels: gatewayType === "bifrost" ? providerModels : undefined,
      });
      toast.success("Model service created");
      setServiceName(""); setServiceDisplay(""); setServiceBaseURL(""); setServiceKey(""); setSelectedProviderModels({});
      await refresh();
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

  const addModel = async (serviceId: number) => {
    if (!modelKey.trim()) return;
    setBusy(true);
    try {
      await agentApi.createModel(serviceId, { modelKey: modelKey.trim(), displayName: modelDisplay.trim() || undefined, maxOutputTokens: 4096, supportsTools: true });
      toast.success("Model added");
      setModelKey(""); setModelDisplay(""); setModelServiceId(null);
      await refresh();
    } catch (err) { toast.error(errorText(err)); }
    finally { setBusy(false); }
  };

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
        <h3 className="flex items-center gap-2 text-sm font-semibold"><CirclePlus className="h-4 w-4 text-primary" />Model services</h3>
        {canManage && (
          <div className="space-y-2 rounded-lg border p-3">
            <div className="grid gap-3 sm:grid-cols-3">
              <label className="block text-xs font-medium">Name<Input className="mt-1" value={serviceName} onChange={(e) => setServiceName(e.target.value)} placeholder="bifrost-prod" /></label>
              <label className="block text-xs font-medium">Display name<Input className="mt-1" value={serviceDisplay} onChange={(e) => setServiceDisplay(e.target.value)} /></label>
              <label className="block text-xs font-medium">Gateway type
                <select className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm" value={gatewayType} onChange={(e) => setGatewayType(e.target.value as typeof gatewayType)}>
                  <option value="bifrost">Bifrost</option>
                  <option value="openai_compatible">OpenAI compatible</option>
                </select>
              </label>
            </div>
            {gatewayType === "openai_compatible" ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="block text-xs font-medium">Base URL<Input className="mt-1" value={serviceBaseURL} onChange={(e) => setServiceBaseURL(e.target.value)} placeholder="https://api.example.com/v1" /></label>
                <label className="block text-xs font-medium">API key<Input className="mt-1" type="password" value={serviceKey} onChange={(e) => setServiceKey(e.target.value)} placeholder="Optional for local endpoints" /></label>
              </div>
            ) : (
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="flex items-center gap-2 text-xs"><input type="checkbox" checked={allowAll} onChange={(e) => setAllowAll(e.target.checked)} />Allow all Bifrost providers/models</label>
                  <Button size="xs" variant="outline" onClick={() => void loadGatewayModels()}><RefreshCw />Load gateway models</Button>
                </div>
                {gatewayModels.length > 0 && (
                  <div className="max-h-48 space-y-2 overflow-auto rounded-md border p-2">
                    {Object.entries(gatewayModels.reduce<Record<string, { name: string; provider: string }[]>>((acc, model) => {
                      (acc[model.provider] ??= []).push(model);
                      return acc;
                    }, {})).map(([provider, models]) => (
                      <div key={provider}>
                        <div className="text-xs font-semibold uppercase text-muted-foreground">{provider}</div>
                        {models.map((model) => (
                          <label key={`${provider}/${model.name}`} className="flex items-center gap-2 text-xs">
                            <input type="checkbox" checked={(selectedProviderModels[provider] ?? []).includes(model.name)} onChange={() => toggleProviderModel(provider, model.name)} />
                            {model.name}
                          </label>
                        ))}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
            <div className="flex justify-end"><Button size="sm" disabled={busy || !serviceName.trim()} onClick={() => void createService()}><CirclePlus />Create service</Button></div>
          </div>
        )}
        <div className="divide-y rounded-lg border">
          {services.length === 0 ? <p className="py-4 text-center text-xs text-muted-foreground">No model services</p> : services.map((service) => (
            <div key={service.id} className="space-y-2 px-3 py-2 text-sm">
              <div className="flex items-center gap-2">
                <span className="font-medium">{service.displayName || service.name}</span>
                <Badge variant="secondary" className="text-[10px]">{service.gatewayType}</Badge>
                <Badge variant="secondary" className={cn("text-[10px]", service.enabled ? "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400" : "")}>{service.enabled ? "enabled" : "disabled"}</Badge>
                {canManage && service.gatewayType === "bifrost" && <Button size="xs" variant="ghost" title="Sync models" onClick={() => void act(() => agentApi.syncModelService(service.id), "Models synced")}><RefreshCw /></Button>}
                {canManage && <Button size="xs" variant="ghost" title="Delete service" onClick={() => void act(() => agentApi.deleteModelService(service.id), "Service deleted")}><Trash2 /></Button>}
              </div>
              <div className="flex flex-wrap gap-1">
                {service.models.map((model) => <Badge key={model.id} variant="secondary" className="text-[10px]">{model.displayName || model.modelKey}</Badge>)}
                {canManage && <Button size="xs" variant="outline" onClick={() => setModelServiceId(modelServiceId === service.id ? null : service.id)}><PackagePlus />Add model</Button>}
              </div>
              {modelServiceId === service.id && (
                <div className="flex flex-wrap items-end gap-2 rounded-md border p-2">
                  <label className="block text-xs font-medium">Model key<Input className="mt-1" value={modelKey} onChange={(e) => setModelKey(e.target.value)} placeholder="provider/model" /></label>
                  <label className="block text-xs font-medium">Display name<Input className="mt-1" value={modelDisplay} onChange={(e) => setModelDisplay(e.target.value)} /></label>
                  <Button size="sm" disabled={busy || !modelKey.trim()} onClick={() => void addModel(service.id)}>Add</Button>
                </div>
              )}
            </div>
          ))}
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

function serviceAPIKey(gatewayType: "bifrost" | "openai_compatible", key: string): string | undefined {
  return gatewayType === "openai_compatible" && key.trim() ? key : undefined;
}
