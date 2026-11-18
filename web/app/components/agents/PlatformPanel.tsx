"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { CirclePlus, Loader2, PackagePlus, Power, PowerOff } from "lucide-react";
import {
  agentApi,
  AppError,
  type MCPServer,
  type PublishedSkill,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

const ADMIN_NOTICE =
  "MCP servers are managed by platform administrators (/v1/mcp/servers requires admin privileges), so they cannot be registered or bound here.";

interface PlatformPanelProps {
  // onChanged lets the parent refresh the MCP/skill lists shown in the agent
  // config editor after an admin mutation here.
  onChanged?: () => void;
}

export function PlatformPanel({ onChanged }: PlatformPanelProps) {
  const [servers, setServers] = useState<MCPServer[] | null>(null);
  const [skills, setSkills] = useState<PublishedSkill[]>([]);
  const [forbidden, setForbidden] = useState(false);
  const [busy, setBusy] = useState(false);

  const [serverName, setServerName] = useState("");
  const [serverUrl, setServerUrl] = useState("");
  const [serverTools, setServerTools] = useState("[]");
  const [serverRisk, setServerRisk] = useState<"read_only" | "write">("read_only");
  const [skillName, setSkillName] = useState("");
  const [skillVersion, setSkillVersion] = useState(1);
  const [skillContent, setSkillContent] = useState("");

  const load = useCallback(() => {
    agentApi
      .listMCPServers()
      .then((data) => {
        setServers(data.servers);
        setForbidden(false);
      })
      .catch((err) => {
        if (err instanceof AppError && err.status === 403) {
          setForbidden(true);
          setServers(null);
        } else {
          toast.error(errorText(err));
        }
      });
    agentApi
      .listSkills()
      .then((data) => setSkills(data.skills))
      .catch(() => setSkills([]));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const registerServer = async () => {
    let tools: string[];
    try {
      const parsed: unknown = JSON.parse(serverTools);
      if (
        !Array.isArray(parsed) ||
        parsed.length === 0 ||
        parsed.some((t) => typeof t !== "string" || t.trim() === "")
      ) {
        throw new Error("bad");
      }
      tools = parsed as string[];
    } catch {
      toast.error('Allowed tools must be a JSON array of tool names, e.g. ["search", "fetch"]');
      return;
    }
    setBusy(true);
    try {
      await agentApi.registerMCPServer({
        name: serverName.trim(),
        url: serverUrl.trim(),
        tools,
        risk: serverRisk,
        enabled: true,
      });
      toast.success("MCP server registered");
      setServerName("");
      setServerUrl("");
      setServerTools("[]");
      setServerRisk("read_only");
      await load();
      onChanged?.();
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const toggleServer = async (server: MCPServer) => {
    try {
      await agentApi.setMCPServerStatus(server.id, !server.enabled);
      setServers((old) =>
        old
          ? old.map((s) => (s.id === server.id ? { ...s, enabled: !server.enabled } : s))
          : old
      );
      onChanged?.();
    } catch (err) {
      toast.error(errorText(err));
    }
  };

  const publishSkill = async () => {
    if (!skillName.trim() || !skillContent.trim()) return;
    setBusy(true);
    try {
      await agentApi.publishSkill({
        name: skillName.trim(),
        version: skillVersion,
        content: skillContent,
      });
      toast.success("Skill published");
      setSkillName("");
      setSkillVersion(1);
      setSkillContent("");
      await load();
      onChanged?.();
    } catch (err) {
      toast.error(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="max-w-3xl space-y-8">
      <section className="space-y-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <CirclePlus className="h-4 w-4 text-primary" />
          MCP servers
        </h3>
        {forbidden ? (
          <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-400">
            {ADMIN_NOTICE}
          </p>
        ) : (
          <>
            <div className="space-y-2 rounded-lg border p-3">
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="block text-xs font-medium">
                  Name
                  <Input
                    className="mt-1"
                    value={serverName}
                    maxLength={100}
                    placeholder="e.g. internal-search"
                    onChange={(e) => setServerName(e.target.value)}
                  />
                </label>
                <label className="block text-xs font-medium">
                  Transport
                  <select
                    className="mt-1 h-8 w-full rounded-md border bg-background px-2 text-sm"
                    value="http"
                    disabled
                    aria-label="Transport"
                  >
                    <option value="http">http (streamable)</option>
                  </select>
                </label>
              </div>
              <label className="block text-xs font-medium">
                URL
                <Input
                  className="mt-1"
                  value={serverUrl}
                  maxLength={200}
                  placeholder="https://mcp.example.com/mcp"
                  onChange={(e) => setServerUrl(e.target.value)}
                />
              </label>
              <label className="block text-xs font-medium">
                Allowed tools (JSON array)
                <textarea
                  className="mt-1 min-h-16 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={serverTools}
                  onChange={(e) => setServerTools(e.target.value)}
                  placeholder='["search", "fetch"]'
                />
              </label>
              <div className="flex items-end justify-between gap-2">
                <label className="block text-xs font-medium">
                  Risk class
                  <select
                    className="mt-1 h-8 rounded-md border bg-background px-2 text-sm"
                    value={serverRisk}
                    onChange={(e) => setServerRisk(e.target.value as "read_only" | "write")}
                    aria-label="Risk class"
                  >
                    <option value="read_only">read_only</option>
                    <option value="write">write</option>
                  </select>
                </label>
                <Button
                  size="sm"
                  disabled={
                    busy ||
                    !serverName.trim() ||
                    !serverUrl.trim()
                  }
                  onClick={() => void registerServer()}
                >
                  {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CirclePlus />}
                  Register server
                </Button>
              </div>
            </div>
            <div className="divide-y rounded-lg border">
              {servers === null ? (
                <p className="flex justify-center py-4">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </p>
              ) : servers.length === 0 ? (
                <p className="py-4 text-center text-xs text-muted-foreground">
                  No MCP servers registered
                </p>
              ) : (
                servers.map((server) => (
                  <div key={server.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{server.name}</span>
                      <span className="block truncate text-xs text-muted-foreground" title={server.url}>
                        {server.url}
                      </span>
                      <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">
                        {server.tools.join(", ")}
                      </span>
                    </span>
                    <Badge variant="secondary" className="shrink-0 text-[10px]">
                      {server.risk}
                    </Badge>
                    <Badge
                      variant="secondary"
                      className={
                        server.enabled
                          ? "shrink-0 bg-emerald-100 text-[10px] text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400"
                          : "shrink-0 text-[10px]"
                      }
                    >
                      {server.enabled ? "enabled" : "disabled"}
                    </Badge>
                    <Button
                      size="xs"
                      variant="outline"
                      title={server.enabled ? "Disable server" : "Enable server"}
                      onClick={() => void toggleServer(server)}
                    >
                      {server.enabled ? <PowerOff /> : <Power />}
                      {server.enabled ? "Disable" : "Enable"}
                    </Button>
                  </div>
                ))
              )}
            </div>
          </>
        )}
      </section>

      <section className="space-y-3">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <PackagePlus className="h-4 w-4 text-primary" />
          Skills
        </h3>
        <div className="space-y-2 rounded-lg border p-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block text-xs font-medium sm:col-span-2">
              Name
              <Input
                className="mt-1"
                value={skillName}
                maxLength={100}
                placeholder="e.g. code-review"
                onChange={(e) => setSkillName(e.target.value)}
              />
            </label>
            <label className="block text-xs font-medium">
              Version
              <Input
                className="mt-1"
                type="number"
                min={1}
                value={skillVersion}
                onChange={(e) => setSkillVersion(Number(e.target.value) || 1)}
              />
            </label>
          </div>
          <label className="block text-xs font-medium">
            Content (SKILL.md)
            <textarea
              className="mt-1 min-h-32 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={skillContent}
              maxLength={200000}
              placeholder={"---\nname: code-review\n---\n\nReview the changed code…"}
              onChange={(e) => setSkillContent(e.target.value)}
            />
          </label>
          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={busy || !skillName.trim() || !skillContent.trim()}
              onClick={() => void publishSkill()}
            >
              {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PackagePlus />}
              Publish skill
            </Button>
          </div>
        </div>
        <div className="divide-y rounded-lg border">
          {skills.length === 0 ? (
            <p className="py-4 text-center text-xs text-muted-foreground">
              No skills published
            </p>
          ) : (
            skills.map((skill) => (
              <div key={`${skill.name}@${skill.version}`} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="flex-1 font-medium">{skill.name}</span>
                <Badge variant="secondary" className="text-[10px]">
                  v{skill.version}
                </Badge>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
}
