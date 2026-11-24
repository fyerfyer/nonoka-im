"use client";

import { useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import type { AgentFormValues } from "@/lib/schemas/agent";
import type { Catalog, Policy } from "@/lib/api/agents";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { bindableMCP, bindableSkill } from "./agent-utils";
import { Wrench, ShieldCheck } from "lucide-react";

interface CapabilityPickerProps {
  form: UseFormReturn<AgentFormValues>;
  catalog: Catalog;
  currentUserId: number;
  disabled?: boolean;
}

export function CapabilityPicker({
  form,
  catalog,
  currentUserId,
  disabled,
}: CapabilityPickerProps) {
  const [advanced, setAdvanced] = useState(false);
  const { watch, setValue } = form;
  const presets = watch("capabilityPresets") || [];
  const policies = watch("toolPolicies") || {};
  const mcpBindings = watch("mcpBindings") || [];
  const skills = watch("skills") || [];

  const visiblePresets = catalog.presets.filter((preset) => preset.key !== "system");
  const toolsByPreset = new Map<string, typeof catalog.tools>();
  for (const tool of catalog.tools) {
    if (tool.hidden || tool.system) continue;
    for (const preset of tool.presets ?? []) {
      const list = toolsByPreset.get(preset) ?? [];
      list.push(tool);
      toolsByPreset.set(preset, list);
    }
  }

  const togglePreset = (key: string) => {
    const next = presets.includes(key)
      ? presets.filter((item) => item !== key)
      : [...presets, key];
    setValue("capabilityPresets", next, { shouldDirty: true, shouldValidate: true });
  };

  const setPolicy = (tool: string, policy: Policy, fallback: Policy) => {
    const next = { ...policies };
    if (policy === fallback) delete next[tool];
    else next[tool] = policy;
    setValue("toolPolicies", next, { shouldDirty: true });
  };

  const toggleMCPTool = (serverId: number, tool: string, risk: string) => {
    const exists = mcpBindings.some(
      (binding) => binding.serverId === serverId && binding.tool === tool
    );
    const next = exists
      ? mcpBindings.filter(
          (binding) => !(binding.serverId === serverId && binding.tool === tool)
        )
      : [
          ...mcpBindings,
          {
            serverId,
            tool,
            policy: (risk === "write" ? "ask" : "auto") as Policy,
          },
        ];
    setValue("mcpBindings", next, { shouldDirty: true, shouldValidate: true });
  };

  const setMCPPolicy = (serverId: number, tool: string, policy: Policy) => {
    setValue(
      "mcpBindings",
      mcpBindings.map((binding) =>
        binding.serverId === serverId && binding.tool === tool
          ? { ...binding, policy }
          : binding
      ),
      { shouldDirty: true }
    );
  };

  const toggleSkill = (skillId: number, version: number) => {
    const exists = skills.some((binding) => binding.skillId === skillId);
    const next = exists
      ? skills.filter((binding) => binding.skillId !== skillId)
      : [...skills, { skillId, version }];
    setValue("skills", next, { shouldDirty: true, shouldValidate: true });
  };

  const bindableMCPs = catalog.mcps.filter((server) =>
    bindableMCP(server, currentUserId)
  );
  const bindableSkills = catalog.skills.filter((skill) =>
    bindableSkill(skill, currentUserId)
  );

  return (
    <div className="space-y-6">
      <section className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">Built-in capabilities</h3>
            <p className="text-xs text-muted-foreground">
              Presets group related IM tools. Write tools default to approval.
            </p>
          </div>
          <Button
            type="button"
            size="xs"
            variant="ghost"
            onClick={() => setAdvanced((value) => !value)}
          >
            {advanced ? "Hide individual tools" : "Show individual tools"}
          </Button>
        </div>
        <div className="space-y-2">
          {visiblePresets.map((preset) => {
            const enabled = presets.includes(preset.key);
            const tools = toolsByPreset.get(preset.key) ?? [];
            return (
              <div key={preset.key} className={cn("rounded-lg border p-3", enabled && "border-primary/40")}>
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={enabled}
                    disabled={disabled}
                    onChange={() => togglePreset(preset.key)}
                  />
                  <span>
                    <span className="font-medium">{preset.name}</span>
                    <span className="block text-xs text-muted-foreground">
                      {preset.description}
                    </span>
                  </span>
                </label>
                {advanced && enabled && tools.length > 0 && (
                  <div className="mt-3 space-y-2 border-t pt-2">
                    {tools.map((tool) => {
                      const toolPolicy = policies[tool.name] ?? tool.defaultPolicy;
                      return (
                        <div key={tool.name} className="flex items-start justify-between gap-3 text-xs">
                          <div className="min-w-0">
                            <div className="flex items-center gap-1.5">
                              <Wrench className="h-3 w-3 text-muted-foreground" />
                              <span className="font-mono">{tool.name}</span>
                              <Badge variant="secondary" className="text-[9px]">
                                {tool.risk}
                              </Badge>
                              {tool.sideEffect && (
                                <Badge variant="secondary" className="text-[9px]">
                                  writes
                                </Badge>
                              )}
                            </div>
                            <p className="mt-0.5 text-muted-foreground">{tool.description}</p>
                          </div>
                          <select
                            className="h-7 shrink-0 rounded-md border bg-background px-1"
                            value={toolPolicy}
                            disabled={disabled || tool.system}
                            onChange={(event) =>
                              setPolicy(
                                tool.name,
                                event.target.value as Policy,
                                tool.defaultPolicy
                              )
                            }
                          >
                            <option value="auto">auto</option>
                            <option value="ask">ask</option>
                            <option value="deny">deny</option>
                          </select>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </section>

      {bindableMCPs.length > 0 && (
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold">MCP tools</h3>
            <p className="text-xs text-muted-foreground">
              Bind individual discovered tools. Write tools are wrapped in an approval gate.
            </p>
          </div>
          <div className="space-y-3">
            {bindableMCPs.map((server) => (
              <div key={server.id} className="rounded-lg border p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">
                    {server.displayName || server.name}
                  </span>
                  <Badge variant="secondary" className="text-[10px]">
                    {server.visibility}
                  </Badge>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px]",
                      server.healthStatus === "healthy" &&
                        "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-400"
                    )}
                  >
                    {server.healthStatus}
                  </Badge>
                  <span className="ml-auto text-[10px] text-muted-foreground">
                    {server.tools?.length ?? 0} tools
                  </span>
                </div>
                <div className="mt-2 space-y-1.5">
                  {(server.tools ?? []).map((tool) => {
                    const binding = mcpBindings.find(
                      (item) =>
                        item.serverId === server.id && item.tool === tool.name
                    );
                    return (
                      <div key={tool.name} className="flex items-start justify-between gap-2 text-xs">
                        <label className="flex min-w-0 items-start gap-2">
                          <input
                            type="checkbox"
                            className="mt-0.5"
                            checked={!!binding}
                            disabled={disabled}
                            onChange={() =>
                              toggleMCPTool(server.id, tool.name, tool.risk)
                            }
                          />
                          <span className="min-w-0">
                            <span className="font-mono">{tool.name}</span>
                            <Badge variant="secondary" className="ml-1 text-[9px]">
                              {tool.risk}
                            </Badge>
                            <span className="block truncate text-muted-foreground">
                              {tool.description}
                            </span>
                          </span>
                        </label>
                        {binding && (
                          <select
                            className="h-7 shrink-0 rounded-md border bg-background px-1"
                            value={binding.policy ?? (tool.risk === "write" ? "ask" : "auto")}
                            disabled={disabled}
                            onChange={(event) =>
                              setMCPPolicy(
                                server.id,
                                tool.name,
                                event.target.value as Policy
                              )
                            }
                          >
                            <option value="auto">auto</option>
                            <option value="ask">ask</option>
                            <option value="deny">deny</option>
                          </select>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {bindableSkills.length > 0 && (
        <section className="space-y-3">
          <div>
            <h3 className="flex items-center gap-1.5 text-sm font-semibold">
              <ShieldCheck className="h-4 w-4" />
              Skills
            </h3>
            <p className="text-xs text-muted-foreground">
              Skills add behavior instructions; they cannot grant tools.
            </p>
          </div>
          <div className="space-y-2 rounded-lg border p-3">
            {bindableSkills.map((skill) => {
              const enabled = skills.some((binding) => binding.skillId === skill.id);
              return (
                <label key={skill.id} className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={enabled}
                    disabled={disabled}
                    onChange={() => toggleSkill(skill.id, skill.latestVersion)}
                  />
                  <span>
                    <span className="font-medium">
                      {skill.displayName || skill.slug}
                    </span>
                    <span className="ml-2 text-xs text-muted-foreground">
                      v{skill.latestVersion} · {skill.visibility}
                    </span>
                    {skill.description && (
                      <span className="block text-xs text-muted-foreground">
                        {skill.description}
                      </span>
                    )}
                  </span>
                </label>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}
