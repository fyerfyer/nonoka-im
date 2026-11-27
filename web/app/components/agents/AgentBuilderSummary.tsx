"use client";

import type { UseFormReturn } from "react-hook-form";
import { Check, CircleAlert, Cpu, ShieldCheck } from "lucide-react";
import type { AgentFormValues } from "@/lib/schemas/agent";
import type { Catalog } from "@/lib/api/agents";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export function AgentBuilderSummary({
  form,
  catalog,
}: {
  form: UseFormReturn<AgentFormValues>;
  catalog: Catalog;
}) {
  const values = form.watch();
  const model = catalog.models.find((item) => item.id === values.modelId);
  const instructionTokens = Math.ceil((values.instruction || "").length / 4);
  const presetNames = (values.capabilityPresets || []).map(
    (key) => catalog.presets.find((preset) => preset.key === key)?.name || key
  );

  const checks = [
    {
      label: "Name and description",
      ok: !!values.name.trim(),
    },
    {
      label: "A curated model",
      ok: !!model,
    },
    {
      label: "Instructions",
      ok: !!values.instruction.trim(),
    },
    {
      label: "At least one capability",
      ok:
        (values.capabilityPresets || []).length > 0 ||
        (values.mcpBindings || []).length > 0 ||
        (values.skills || []).length > 0,
    },
  ];

  return (
    <aside className="space-y-4 rounded-xl border bg-card p-4">
      <div>
        <h2 className="text-sm font-semibold">Draft summary</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          A quick read before you save or publish.
        </p>
      </div>

      <div className="space-y-2 text-xs">
        <div className="flex items-start justify-between gap-3">
          <span className="text-muted-foreground">Model</span>
          <span className="text-right font-medium">
            {model ? model.displayName || model.modelKey : "Not selected"}
          </span>
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground">Instructions</span>
          <span className="font-medium">
            ~{instructionTokens.toLocaleString()} tokens
          </span>
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="text-muted-foreground">Max output</span>
          <span className="font-medium">{values.maxTokens || 4096}</span>
        </div>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium text-muted-foreground">
          Capabilities
        </p>
        <div className="flex flex-wrap gap-1">
          {presetNames.map((name) => (
            <Badge key={name} variant="secondary" className="text-[10px]">
              {name}
            </Badge>
          ))}
          {(values.mcpBindings || []).map((binding) => (
            <Badge
              key={`${binding.serverId}/${binding.tool}`}
              variant="secondary"
              className="text-[10px]"
            >
              {binding.tool}
            </Badge>
          ))}
          {(values.skills || []).map((skill) => {
            const catalogSkill = catalog.skills.find(
              (item) => item.id === skill.skillId
            );
            return (
              <Badge
                key={skill.skillId}
                variant="secondary"
                className="text-[10px]"
              >
                {catalogSkill?.displayName || catalogSkill?.slug || `skill #${skill.skillId}`}
              </Badge>
            );
          })}
          {presetNames.length === 0 &&
            (values.mcpBindings || []).length === 0 &&
            (values.skills || []).length === 0 && (
              <span className="text-xs text-muted-foreground">
                No capabilities selected
              </span>
            )}
        </div>
      </div>

      <div className="space-y-1.5 border-t pt-3">
        {checks.map((check) => (
          <div key={check.label} className="flex items-center gap-2 text-xs">
            {check.ok ? (
              <Check className="h-3.5 w-3.5 text-success" />
            ) : (
              <CircleAlert className="h-3.5 w-3.5 text-warning" />
            )}
            <span className={cn(check.ok ? "text-foreground" : "text-muted-foreground")}>
              {check.label}
            </span>
          </div>
        ))}
      </div>

      <div className="flex items-start gap-2 rounded-lg bg-muted/40 p-2.5 text-[11px] text-muted-foreground">
        {values.modelId ? (
          <Cpu className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        ) : (
          <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        )}
        <span>
          Provider credentials stay in Bifrost. Tool calls are authorized per
          actor and topic, and write actions follow the policies above.
        </span>
      </div>
    </aside>
  );
}
