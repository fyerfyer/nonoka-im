"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, Bot, Loader2, Plus } from "lucide-react";
import { agentApi } from "@/lib/api/agents";
import { agentFormSchema, type AgentFormValues } from "@/lib/schemas/agent";
import { agentKeys, catalogQuery } from "@/lib/queries/agents";
import { useAuthStore } from "@/stores/authStore";
import { invalidateAgentNames } from "@/hooks/useAgentNames";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/workspace/PageHeader";
import { AgentBuilder } from "@/components/agents/AgentBuilder";
import { TemplateStrip } from "@/components/agents/TemplateStrip";
import {
  defaultAgentForm,
  emptyCatalog,
  errorText,
  toAgentInput,
} from "@/components/agents/agent-utils";
import type { AgentTemplate } from "@/lib/agent-templates";

export default function NewAgentPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const catalog = useQuery(catalogQuery);
  const [activeTemplate, setActiveTemplate] = useState<string | undefined>();
  const catalogData = catalog.data || emptyCatalog;

  const form = useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: defaultAgentForm(emptyCatalog),
  });

  useEffect(() => {
    if (!catalog.data) return;
    const defaults = defaultAgentForm(catalog.data);
    const current = form.getValues();
    if (!current.modelId) {
      form.reset({
        ...defaults,
        name: current.name,
        description: current.description,
      });
    }
  }, [catalog.data, form]);

  const create = useMutation({
    mutationFn: (values: AgentFormValues) =>
      agentApi.create(toAgentInput(values)),
    onSuccess: async (agent) => {
      invalidateAgentNames();
      await queryClient.invalidateQueries({ queryKey: agentKeys.list() });
      toast.success("Agent created");
      router.replace(`/agents/${agent.id}?tab=build`);
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const applyTemplate = (template: AgentTemplate | null) => {
    if (
      form.formState.isDirty &&
      !window.confirm("Replace the current draft with this template?")
    ) {
      return;
    }
    const defaults = defaultAgentForm(catalogData);
    if (!template) {
      form.reset({
        ...defaults,
        name: "",
        description: "",
        instruction: "",
      });
      setActiveTemplate(undefined);
      return;
    }
    form.setValue("name", template.defaultName, { shouldDirty: true });
    form.setValue("description", template.defaultDescription, {
      shouldDirty: true,
    });
    form.setValue("instruction", template.prompt, {
      shouldDirty: true,
      shouldValidate: true,
    });
    form.setValue("capabilityPresets", template.capabilityPresets, {
      shouldDirty: true,
      shouldValidate: true,
    });
    form.setValue("toolPolicies", template.toolPolicies ?? {}, {
      shouldDirty: true,
    });
    setActiveTemplate(template.id);
  };

  const submit = form.handleSubmit((values) => create.mutate(values));

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader
        title="New agent"
        description="Start from a template, then tune the model, instructions and capabilities."
        icon={Bot}
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push("/agents")}
            >
              <ArrowLeft />
              Back
            </Button>
            <Button
              size="sm"
              disabled={create.isPending || catalog.isPending}
              onClick={() => void submit()}
            >
              {create.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <Plus />
              )}
              Create agent
            </Button>
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-7xl px-4 py-5 sm:px-6">
          {catalog.isPending ? (
            <div className="flex min-h-96 items-center justify-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading catalog…
            </div>
          ) : (
            <AgentBuilder
              form={form}
              catalog={catalogData}
              currentUserId={Number(user?.userId)}
              mode="create"
              templateStrip={
                <TemplateStrip
                  activeId={activeTemplate}
                  onSelect={applyTemplate}
                />
              }
            />
          )}
        </div>
      </div>
    </div>
  );
}
