"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, ArrowRight, Bot, Check, Loader2, Plus } from "lucide-react";
import { agentApi, type Catalog } from "@/lib/api/agents";
import { agentFormSchema, type AgentFormValues } from "@/lib/schemas/agent";
import { agentKeys, catalogQuery } from "@/lib/queries/agents";
import { useAuth } from "@/hooks/useAuth";
import { useAuthStore } from "@/stores/authStore";
import { invalidateAgentNames } from "@/hooks/useAgentNames";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AgentBuildForm } from "@/components/agents/AgentBuildForm";
import { CapabilityPicker } from "@/components/agents/CapabilityPicker";
import {
  defaultAgentForm,
  emptyCatalog,
  errorText,
  toAgentInput,
} from "@/components/agents/agent-utils";
import { cn } from "@/lib/utils";

const STEPS = ["Identity", "Model & instructions", "Capabilities", "Review"];

export default function NewAgentPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const { isReady, isAuthenticated } = useAuth();
  const catalog = useQuery(catalogQuery);
  const [step, setStep] = useState(0);
  const form = useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: defaultAgentForm(emptyCatalog),
  });

  useEffect(() => {
    if (catalog.data) {
      const defaults = defaultAgentForm(catalog.data);
      const current = form.getValues();
      if (!current.modelServiceId) {
        form.reset({ ...defaults, name: current.name, description: current.description });
      }
    }
  }, [catalog.data, form]);

  const create = useMutation({
    mutationFn: (values: AgentFormValues) => agentApi.create(toAgentInput(values)),
    onSuccess: async (agent) => {
      invalidateAgentNames();
      await queryClient.invalidateQueries({ queryKey: agentKeys.list() });
      toast.success("Agent created");
      router.replace(`/agents/${agent.id}?tab=build`);
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const next = async () => {
    const fields: (keyof AgentFormValues)[] =
      step === 0
        ? ["name", "description"]
        : step === 1
          ? ["modelServiceId", "modelId", "instruction", "maxTokens"]
          : [];
    if (fields.length > 0 && !(await form.trigger(fields))) return;
    setStep((current) => Math.min(current + 1, STEPS.length - 1));
  };

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  const catalogData: Catalog = catalog.data || emptyCatalog;
  const values = form.getValues();

  return (
    <div className="min-h-screen bg-background">
      <header className="flex h-14 items-center gap-3 border-b px-4 sm:px-6">
        <Button variant="ghost" size="icon" onClick={() => router.push("/agents")}>
          <ArrowLeft />
        </Button>
        <Bot className="h-5 w-5 text-primary" />
        <h1 className="text-base font-semibold">New agent</h1>
      </header>

      <main className="mx-auto max-w-4xl px-4 py-6 sm:px-6">
        <ol className="mb-6 flex flex-wrap gap-2 text-xs">
          {STEPS.map((label, index) => (
            <li
              key={label}
              className={cn(
                "flex items-center gap-1.5 rounded-full border px-3 py-1.5",
                index === step
                  ? "border-primary bg-primary/10 text-primary"
                  : index < step
                    ? "border-emerald-300 text-emerald-700 dark:border-emerald-500/40 dark:text-emerald-400"
                    : "text-muted-foreground"
              )}
            >
              {index < step ? (
                <Check className="h-3 w-3" />
              ) : (
                <span className="font-mono">{index + 1}</span>
              )}
              {label}
            </li>
          ))}
        </ol>

        <div className="rounded-xl border bg-card p-5">
          {step === 0 && (
            <AgentBuildForm
              form={form}
              catalog={catalogData}
              sections={["identity"]}
            />
          )}
          {step === 1 && (
            <AgentBuildForm
              form={form}
              catalog={catalogData}
              sections={["model", "instruction", "advanced"]}
            />
          )}
          {step === 2 && (
            <CapabilityPicker
              form={form}
              catalog={catalogData}
              currentUserId={Number(user?.userId)}
            />
          )}
          {step === 3 && (
            <div className="space-y-4">
              <h2 className="text-sm font-semibold">Review</h2>
              <div className="grid gap-3 text-sm sm:grid-cols-2">
                <Summary label="Name" value={values.name} />
                <Summary label="Description" value={values.description || "-"} />
                <Summary
                  label="Model service"
                  value={
                    catalogData.services.find((s) => s.id === values.modelServiceId)
                      ?.displayName || "-"
                  }
                />
                <Summary label="Max tokens" value={String(values.maxTokens)} />
                <Summary
                  label="Capabilities"
                  value={
                    values.capabilityPresets.join(", ") || "No presets selected"
                  }
                />
                <Summary
                  label="MCP tools"
                  value={
                    values.mcpBindings
                      .map((binding) => binding.tool)
                      .join(", ") || "None"
                  }
                />
              </div>
              <div className="rounded-lg border bg-muted/30 p-3">
                <Label className="text-xs">Instructions preview</Label>
                <p className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap text-sm text-muted-foreground">
                  {values.instruction || "No instructions yet."}
                </p>
              </div>
            </div>
          )}
        </div>

        <div className="mt-4 flex justify-between">
          <Button
            variant="outline"
            disabled={step === 0}
            onClick={() => setStep((current) => Math.max(0, current - 1))}
          >
            Back
          </Button>
          <div className="flex gap-2">
            {step < STEPS.length - 1 ? (
              <Button onClick={() => void next()}>
                Next
                <ArrowRight />
              </Button>
            ) : (
              <Button
                disabled={create.isPending || !form.formState.isValid}
                onClick={() => void form.handleSubmit((data) => create.mutate(data))()}
              >
                {create.isPending ? <Loader2 className="animate-spin" /> : <Plus />}
                Create agent
              </Button>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-0.5 truncate text-sm">{value}</p>
    </div>
  );
}
