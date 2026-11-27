"use client";

import { Combobox } from "@base-ui/react/combobox";
import { Check, ChevronDown, Cpu } from "lucide-react";
import type { Model } from "@/lib/api/agents";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

function modelLabel(model: Model): string {
  return model.displayName || model.modelKey;
}

function capabilityBadges(model: Model) {
  return (
    <span className="flex flex-wrap gap-1">
      {model.supportsVision && (
        <Badge variant="secondary" className="text-[9px]">
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
          recommended
        </Badge>
      )}
    </span>
  );
}

export function ModelCombobox({
  models,
  value,
  onChange,
  disabled,
}: {
  models: Model[];
  value: Model | null;
  onChange: (model: Model) => void;
  disabled?: boolean;
}) {
  const selectable = models
    .filter((model) => model.enabled && model.curated)
    .sort((a, b) => {
      if (a.recommended !== b.recommended) return a.recommended ? -1 : 1;
      if (a.provider !== b.provider) return a.provider.localeCompare(b.provider);
      return modelLabel(a).localeCompare(modelLabel(b));
    });

  return (
    <div className="space-y-2">
      <Combobox.Root
        items={selectable}
        value={value}
        onValueChange={(next) => {
          if (next) onChange(next);
        }}
        itemToStringLabel={modelLabel}
        itemToStringValue={(model) => String(model.id)}
        isItemEqualToValue={(a, b) => a.id === b.id}
        autoHighlight
        disabled={disabled || selectable.length === 0}
      >
        <div className="relative">
          <Combobox.Input
            aria-label="Model"
            placeholder={
              selectable.length === 0
                ? "No curated models available"
                : "Search models by name"
            }
            className="h-9 w-full rounded-lg border border-input bg-background pr-8 pl-9 text-sm outline-none transition-shadow placeholder:text-muted-foreground focus-visible:ring-3 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-60"
          />
          <Cpu className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
          <ChevronDown className="pointer-events-none absolute right-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        </div>
        <Combobox.Portal>
          <Combobox.Positioner sideOffset={6} className="z-50">
            <Combobox.Popup className="max-h-80 w-[min(36rem,calc(100vw-2rem))] overflow-y-auto rounded-xl border bg-popover p-1 text-popover-foreground shadow-xl outline-none">
              <Combobox.Empty className="px-3 py-6 text-center text-xs text-muted-foreground">
                No models match your search.
              </Combobox.Empty>
              <Combobox.List>
                {(model: Model) => (
                  <Combobox.Item
                    key={model.id}
                    value={model}
                    className={cn(
                      "flex cursor-default flex-col gap-1 rounded-lg px-3 py-2 text-sm outline-none select-none",
                      "data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
                    )}
                  >
                    <span className="flex items-center gap-2">
                      {model.icon && <span aria-hidden>{model.icon}</span>}
                      <span className="font-medium">{modelLabel(model)}</span>
                      <span className="text-[10px] text-muted-foreground">
                        {model.provider}
                      </span>
                      {value?.id === model.id && (
                        <Check className="ml-auto h-4 w-4 text-primary" />
                      )}
                    </span>
                    <span className="flex flex-wrap items-center gap-1.5">
                      {capabilityBadges(model)}
                      <span className="ml-auto text-[10px] text-muted-foreground">
                        {model.contextWindow
                          ? `${Math.round(model.contextWindow / 1000)}K ctx`
                          : ""}
                        {" · "}${model.inputPricePerMtok.toFixed(2)} / $
                        {model.outputPricePerMtok.toFixed(2)} Mtok
                      </span>
                    </span>
                  </Combobox.Item>
                )}
              </Combobox.List>
            </Combobox.Popup>
          </Combobox.Positioner>
        </Combobox.Portal>
      </Combobox.Root>

      {value ? (
        <div className="rounded-lg border bg-muted/30 px-3 py-2 text-xs">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{modelLabel(value)}</span>
            <span className="text-muted-foreground">{value.provider}</span>
            {capabilityBadges(value)}
            <span className="ml-auto text-muted-foreground">
              {value.contextWindow
                ? `${Math.round(value.contextWindow / 1000)}K context`
                : "context unknown"}
              {" · "}max {value.maxOutputTokens || 4096} output
            </span>
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          Ask a platform admin to enable and curate models in the Operator
          console.
        </p>
      )}
    </div>
  );
}
