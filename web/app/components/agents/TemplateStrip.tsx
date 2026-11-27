"use client";

import { Plus } from "lucide-react";
import { AGENT_TEMPLATES, type AgentTemplate } from "@/lib/agent-templates";
import { cn } from "@/lib/utils";

export function TemplateStrip({
  activeId,
  onSelect,
}: {
  activeId?: string;
  onSelect: (template: AgentTemplate | null) => void;
}) {
  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">Start from a template</h2>
        <p className="text-xs text-muted-foreground">
          Templates prefill the instructions and capabilities. You can change
          everything before publishing.
        </p>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        <button
          type="button"
          aria-pressed={!activeId}
          className={cn(
            "flex min-h-24 items-start gap-3 rounded-xl border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-accent/30",
            !activeId && "border-primary/50 bg-accent/30"
          )}
          onClick={() => onSelect(null)}
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-muted">
            <Plus className="h-4 w-4" />
          </span>
          <span>
            <span className="block text-sm font-medium">Blank agent</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              Start with a clean draft.
            </span>
          </span>
        </button>
        {AGENT_TEMPLATES.map((template) => (
          <button
            key={template.id}
            type="button"
            aria-pressed={activeId === template.id}
            className={cn(
              "flex min-h-24 items-start gap-3 rounded-xl border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-accent/30",
              activeId === template.id &&
                "border-primary/50 bg-accent/30"
            )}
            onClick={() => onSelect(template)}
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <template.icon className="h-4 w-4" />
            </span>
            <span className="min-w-0">
              <span className="block text-sm font-medium">{template.name}</span>
              <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">
                {template.description}
              </span>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
