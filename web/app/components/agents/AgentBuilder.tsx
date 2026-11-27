"use client";

import type { ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Bot, Cpu, FileText, Sparkles } from "lucide-react";
import type { AgentFormValues } from "@/lib/schemas/agent";
import type { Catalog } from "@/lib/api/agents";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { ModelCombobox } from "./ModelCombobox";
import { CapabilityCards } from "./CapabilityCards";
import { AgentBuilderSummary } from "./AgentBuilderSummary";

function Section({
  icon: Icon,
  title,
  description,
  children,
}: {
  icon: typeof Bot;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-xl border bg-card p-4 sm:p-5">
      <div className="flex items-start gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <Icon className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{title}</h2>
          <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
            {description}
          </p>
        </div>
      </div>
      <div className="mt-4">{children}</div>
    </section>
  );
}

export function AgentBuilder({
  form,
  catalog,
  currentUserId,
  mode,
  disabled,
  templateStrip,
}: {
  form: UseFormReturn<AgentFormValues>;
  catalog: Catalog;
  currentUserId: number;
  mode: "create" | "edit";
  disabled?: boolean;
  templateStrip?: ReactNode;
}) {
  const {
    register,
    watch,
    setValue,
    formState: { errors },
  } = form;
  const model = catalog.models.find((item) => item.id === watch("modelId")) || null;
  const instruction = watch("instruction") || "";
  const estimate = Math.ceil(instruction.length / 4);

  return (
    <div className="space-y-5">
      {templateStrip}
      <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-4">
          <Section
            icon={Bot}
            title="Identity"
            description="How this agent appears to people in conversations."
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="agent-name">Name</Label>
                <Input
                  id="agent-name"
                  placeholder="Research assistant"
                  maxLength={100}
                  disabled={disabled}
                  {...register("name")}
                />
                {errors.name && (
                  <p className="text-xs text-destructive">{errors.name.message}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="agent-max-tokens">Max output tokens</Label>
                <Input
                  id="agent-max-tokens"
                  type="number"
                  min={1}
                  max={128000}
                  disabled={disabled}
                  {...register("maxTokens", { valueAsNumber: true })}
                />
                {errors.maxTokens && (
                  <p className="text-xs text-destructive">
                    {errors.maxTokens.message}
                  </p>
                )}
              </div>
            </div>
            <div className="mt-4 space-y-1.5">
              <Label htmlFor="agent-description">Description</Label>
              <Input
                id="agent-description"
                placeholder="What this agent is good at"
                maxLength={4000}
                disabled={disabled}
                {...register("description")}
              />
            </div>
          </Section>

          <Section
            icon={Cpu}
            title="Model"
            description="Models are provided by the platform through Bifrost. Provider credentials never leave the gateway."
          >
            <ModelCombobox
              models={catalog.models}
              value={model}
              disabled={disabled}
              onChange={(next) => {
                setValue("modelId", next.id, { shouldValidate: true });
                if (next.maxOutputTokens) {
                  setValue("maxTokens", Math.min(next.maxOutputTokens, 4096), {
                    shouldDirty: true,
                  });
                }
              }}
            />
            {errors.modelId && (
              <p className="mt-1 text-xs text-destructive">
                {errors.modelId.message}
              </p>
            )}
            {mode === "edit" && (
              <p className="mt-2 text-[11px] text-muted-foreground">
                Published versions freeze their model. Changing this draft does
                not affect the live version until you publish again.
              </p>
            )}
          </Section>

          <Section
            icon={FileText}
            title="Instructions"
            description="Role, workflow, constraints and output requirements. Platform safety rules are applied outside these instructions."
          >
            <div className="flex items-end justify-between gap-3">
              <Label htmlFor="agent-instruction" className="sr-only">
                Instructions
              </Label>
              <span className="ml-auto text-[11px] text-muted-foreground">
                ~{estimate.toLocaleString()} tokens
              </span>
            </div>
            <textarea
              id="agent-instruction"
              className={cn(
                "mt-2 min-h-56 w-full resize-y rounded-lg border bg-background px-3 py-2 font-mono text-sm outline-none transition-shadow focus-visible:ring-3 focus-visible:ring-ring/30",
                errors.instruction && "border-destructive"
              )}
              maxLength={20000}
              placeholder="You are a concise research assistant for this workspace. Search the conversation, cite the messages you used, and say what you could not verify."
              disabled={disabled}
              {...register("instruction")}
            />
            <div className="mt-1 flex justify-between text-xs text-muted-foreground">
              <span>
                {errors.instruction ? (
                  <span className="text-destructive">
                    {errors.instruction.message}
                  </span>
                ) : (
                  "Be specific about when to use tools and how to format answers."
                )}
              </span>
              <span>{instruction.length.toLocaleString()} / 20,000</span>
            </div>
          </Section>

          <Section
            icon={Sparkles}
            title="Capabilities"
            description="Grant tools by capability. Individual tool policies can be tightened after enabling a preset."
          >
            <CapabilityCards
              form={form}
              catalog={catalog}
              currentUserId={currentUserId}
              disabled={disabled}
            />
          </Section>
        </div>

        <div className="lg:sticky lg:top-4 lg:self-start">
          <AgentBuilderSummary form={form} catalog={catalog} />
        </div>
      </div>
    </div>
  );
}
