"use client";

import { useMemo } from "react";
import type { UseFormReturn } from "react-hook-form";
import {
  BookOpen,
  Package,
  Plug,
  Send,
  ShieldCheck,
  Wrench,
} from "lucide-react";
import type { AgentFormValues } from "@/lib/schemas/agent";
import type { Catalog, Policy } from "@/lib/api/agents";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { bindableMCP, bindableSkill } from "./agent-utils";

const PRESET_ICONS: Record<string, typeof BookOpen> = {
  "im.read": BookOpen,
  "im.write": Send,
};

function Toggle({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={onChange}
      className={cn(
        "relative h-5 w-9 shrink-0 rounded-full border transition-colors",
        checked ? "border-primary bg-primary" : "border-input bg-muted",
        disabled && "cursor-not-allowed opacity-50"
      )}
    >
      <span
        className={cn(
          "absolute left-0 top-0.5 h-3.5 w-3.5 rounded-full bg-background shadow-sm transition-transform",
          checked ? "translate-x-4" : "translate-x-0.5"
        )}
      />
    </button>
  );
}

export function CapabilityCards({
  form,
  catalog,
  currentUserId,
  disabled,
}: {
  form: UseFormReturn<AgentFormValues>;
  catalog: Catalog;
  currentUserId: number;
  disabled?: boolean;
}) {
  const { watch, setValue } = form;
  const presets = watch("capabilityPresets") || [];
  const policies = watch("toolPolicies") || {};
  const mcpBindings = watch("mcpBindings") || [];
  const skills = watch("skills") || [];

  const visiblePresets = catalog.presets.filter((preset) => preset.key !== "system");
  const toolsByPreset = useMemo(() => {
    const map = new Map<string, typeof catalog.tools>();
    for (const tool of catalog.tools) {
      if (tool.hidden || tool.system) continue;
      for (const preset of tool.presets ?? []) {
        const list = map.get(preset) ?? [];
        list.push(tool);
        map.set(preset, list);
      }
    }
    return map;
  }, [catalog]);

  const bindableMCPs = catalog.mcps.filter((server) =>
    bindableMCP(server, currentUserId)
  );
  const bindableSkills = catalog.skills.filter((skill) =>
    bindableSkill(skill, currentUserId)
  );

  const togglePreset = (key: string) => {
    const next = presets.includes(key)
      ? presets.filter((item) => item !== key)
      : [...presets, key];
    setValue("capabilityPresets", next, {
      shouldDirty: true,
      shouldValidate: true,
    });
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
          (binding) =>
            !(binding.serverId === serverId && binding.tool === tool)
        )
      : [
          ...mcpBindings,
          {
            serverId,
            tool,
            policy: (risk === "write" ? "ask" : "auto") as Policy,
          },
        ];
    setValue("mcpBindings", next, {
      shouldDirty: true,
      shouldValidate: true,
    });
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
    setValue("skills", next, {
      shouldDirty: true,
      shouldValidate: true,
    });
  };

  return (
    <div className="space-y-5">
      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-semibold">Built-in capabilities</h3>
          <p className="text-xs text-muted-foreground">
            Capability presets group IM tools. Write tools require approval by
            default and can be tightened per tool.
          </p>
        </div>

        <div className="space-y-2">
          {visiblePresets.map((preset) => {
            const enabled = presets.includes(preset.key);
            const Icon = PRESET_ICONS[preset.key] ?? Wrench;
            const tools = toolsByPreset.get(preset.key) ?? [];
            const writeTools = tools.filter(
              (tool) => tool.risk === "write" || tool.sideEffect
            ).length;
            return (
              <div
                key={preset.key}
                className={cn(
                  "rounded-xl border bg-card p-4 transition-colors",
                  enabled && "border-primary/40"
                )}
              >
                <div className="flex items-start gap-3">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Icon className="h-4 w-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <h4 className="text-sm font-medium">{preset.name}</h4>
                      <Badge variant="secondary" className="text-[9px]">
                        {tools.length} tools
                      </Badge>
                      {writeTools > 0 && (
                        <Badge variant="secondary" className="text-[9px]">
                          {writeTools} write
                        </Badge>
                      )}
                    </div>
                    <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                      {preset.description}
                    </p>
                  </div>
                  <Toggle
                    checked={enabled}
                    disabled={disabled}
                    label={`Enable ${preset.name}`}
                    onChange={() => togglePreset(preset.key)}
                  />
                </div>

                <div className="mt-3 flex flex-wrap gap-1">
                  {tools.map((tool) => (
                    <Badge
                      key={tool.name}
                      variant="secondary"
                      className="font-mono text-[9px]"
                    >
                      {tool.name}
                    </Badge>
                  ))}
                </div>

                {enabled && tools.length > 0 && (
                  <details className="mt-3 border-t pt-2">
                    <summary className="cursor-pointer text-xs text-muted-foreground">
                      Advanced tool policies
                    </summary>
                    <div className="mt-2 space-y-1.5">
                      {tools.map((tool) => {
                        const policy = policies[tool.name] ?? tool.defaultPolicy;
                        return (
                          <div
                            key={tool.name}
                            className="flex items-center justify-between gap-3 text-xs"
                          >
                            <div className="flex min-w-0 items-center gap-1.5">
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
                            <select
                              className="h-7 shrink-0 rounded-md border bg-background px-1 text-xs"
                              value={policy}
                              disabled={disabled}
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
                  </details>
                )}
              </div>
            );
          })}
        </div>
      </section>

      <section className="space-y-2">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold">
            <Plug className="h-4 w-4" />
            MCP tools
          </h3>
          <p className="text-xs text-muted-foreground">
            Bind individual tools from approved servers. Write tools are wrapped
            in an approval gate.
          </p>
        </div>
        {bindableMCPs.length === 0 ? (
          <p className="rounded-lg border border-dashed p-3 text-xs text-muted-foreground">
            No bindable MCP servers. Ask a platform admin to register and review
            a server first.
          </p>
        ) : (
          <div className="space-y-2">
            {bindableMCPs.map((server) => (
              <div key={server.id} className="rounded-xl border bg-card p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">
                    {server.displayName || server.name}
                  </span>
                  <Badge variant="secondary" className="text-[9px]">
                    {server.visibility}
                  </Badge>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[9px]",
                      server.healthStatus === "healthy" &&
                        "bg-success/10 text-success"
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
                      <div
                        key={tool.name}
                        className="flex items-center justify-between gap-2 text-xs"
                      >
                        <label className="flex min-w-0 items-center gap-2">
                          <input
                            type="checkbox"
                            className="h-3.5 w-3.5"
                            checked={!!binding}
                            disabled={disabled}
                            onChange={() =>
                              toggleMCPTool(server.id, tool.name, tool.risk)
                            }
                          />
                          <span className="truncate font-mono">{tool.name}</span>
                          <Badge variant="secondary" className="text-[9px]">
                            {tool.risk}
                          </Badge>
                        </label>
                        {binding && (
                          <select
                            className="h-7 shrink-0 rounded-md border bg-background px-1 text-xs"
                            value={
                              binding.policy ??
                              (tool.risk === "write" ? "ask" : "auto")
                            }
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
        )}
      </section>

      <section className="space-y-2">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold">
            <Package className="h-4 w-4" />
            Skills
          </h3>
          <p className="text-xs text-muted-foreground">
            Skills add behavior instructions. They cannot grant tools.
          </p>
        </div>
        {bindableSkills.length === 0 ? (
          <p className="rounded-lg border border-dashed p-3 text-xs text-muted-foreground">
            No bindable skills yet.
          </p>
        ) : (
          <div className="space-y-2 rounded-xl border bg-card p-3">
            {bindableSkills.map((skill) => {
              const enabled = skills.some(
                (binding) => binding.skillId === skill.id
              );
              return (
                <label
                  key={skill.id}
                  className="flex items-start gap-2 text-sm"
                >
                  <input
                    type="checkbox"
                    className="mt-1 h-3.5 w-3.5"
                    checked={enabled}
                    disabled={disabled}
                    onChange={() =>
                      toggleSkill(skill.id, skill.latestVersion)
                    }
                  />
                  <span className="min-w-0">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">
                        {skill.displayName || skill.slug}
                      </span>
                      <Badge variant="secondary" className="text-[9px]">
                        v{skill.latestVersion} · {skill.visibility}
                      </Badge>
                      <ShieldCheck className="h-3 w-3 text-muted-foreground" />
                    </span>
                    {skill.description && (
                      <span className="mt-0.5 block text-xs text-muted-foreground">
                        {skill.description}
                      </span>
                    )}
                  </span>
                </label>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
