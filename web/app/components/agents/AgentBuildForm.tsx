"use client";

import type { UseFormReturn } from "react-hook-form";
import type { AgentFormValues } from "@/lib/schemas/agent";
import type { Catalog } from "@/lib/api/agents";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

type Section = "identity" | "model" | "instruction" | "advanced";

interface AgentBuildFormProps {
  form: UseFormReturn<AgentFormValues>;
  catalog: Catalog;
  sections?: Section[];
  disabled?: boolean;
}

export function AgentBuildForm({
  form,
  catalog,
  sections = ["identity", "model", "instruction", "advanced"],
  disabled,
}: AgentBuildFormProps) {
  const {
    register,
    watch,
    setValue,
    formState: { errors },
  } = form;
  const serviceId = watch("modelServiceId");
  const service = catalog.services.find((item) => item.id === serviceId);
  const models = service?.models.filter((model) => model.enabled) ?? [];
  const instruction = watch("instruction") || "";
  const estimate = Math.ceil(instruction.length / 4);

  return (
    <div className="space-y-6">
      {sections.includes("identity") && (
        <section className="space-y-4">
          <div>
            <h3 className="text-sm font-semibold">Identity</h3>
            <p className="text-xs text-muted-foreground">
              How the agent appears to people in chats.
            </p>
          </div>
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
          <div className="space-y-1.5">
            <Label htmlFor="agent-description">Description</Label>
            <Input
              id="agent-description"
              placeholder="What this agent is good at"
              maxLength={4000}
              disabled={disabled}
              {...register("description")}
            />
          </div>
        </section>
      )}

      {sections.includes("model") && (
        <section className="space-y-4">
          <div>
            <h3 className="text-sm font-semibold">Model</h3>
            <p className="text-xs text-muted-foreground">
              Models are provided by the platform; credentials never leave the
              gateway.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="agent-model-service">Model service</Label>
              <select
                id="agent-model-service"
                className="h-9 w-full rounded-md border bg-background px-2 text-sm"
                disabled={disabled}
                value={serviceId || ""}
                onChange={(event) => {
                  const nextId = Number(event.target.value);
                  const nextService = catalog.services.find((s) => s.id === nextId);
                  const nextModel = nextService?.models.find((m) => m.enabled);
                  setValue("modelServiceId", nextId, { shouldValidate: true });
                  setValue("modelId", nextModel?.id ?? 0, { shouldValidate: true });
                }}
              >
                <option value="">Select service</option>
                {catalog.services
                  .filter((item) => item.enabled)
                  .map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.displayName || item.name} ({item.gatewayType})
                    </option>
                  ))}
              </select>
              {errors.modelServiceId && (
                <p className="text-xs text-destructive">
                  {errors.modelServiceId.message}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="agent-model">Model</Label>
              <select
                id="agent-model"
                className="h-9 w-full rounded-md border bg-background px-2 text-sm"
                disabled={disabled || models.length === 0}
                value={watch("modelId") || ""}
                onChange={(event) =>
                  setValue("modelId", Number(event.target.value), {
                    shouldValidate: true,
                  })
                }
              >
                <option value="">Select model</option>
                {models.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.displayName || model.modelKey}
                  </option>
                ))}
              </select>
              {errors.modelId && (
                <p className="text-xs text-destructive">{errors.modelId.message}</p>
              )}
            </div>
          </div>
        </section>
      )}

      {sections.includes("instruction") && (
        <section className="space-y-2">
          <div className="flex items-end justify-between">
            <div>
              <h3 className="text-sm font-semibold">Instructions</h3>
              <p className="text-xs text-muted-foreground">
                Role, workflow and output requirements.
              </p>
            </div>
            <span className="text-[11px] text-muted-foreground">
              ~{estimate.toLocaleString()} tokens
            </span>
          </div>
          <textarea
            className={cn(
              "min-h-56 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring",
              errors.instruction && "border-destructive"
            )}
            maxLength={20000}
            placeholder="You are a concise research assistant for this workspace. Cite the tools you used…"
            disabled={disabled}
            {...register("instruction")}
          />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>
              Platform safety rules are applied outside these instructions.
            </span>
            <span>{instruction.length.toLocaleString()} / 20,000</span>
          </div>
          {errors.instruction && (
            <p className="text-xs text-destructive">{errors.instruction.message}</p>
          )}
        </section>
      )}

      {sections.includes("advanced") && (
        <section className="space-y-2 rounded-lg border p-3">
          <h3 className="text-sm font-semibold">Platform guardrails</h3>
          <p className="text-xs text-muted-foreground">
            Tool calls are authorized per actor and topic. Write actions still
            require approval according to the tool policy below. Raw tool
            arguments never leave the run checkpoint.
          </p>
        </section>
      )}
    </div>
  );
}
