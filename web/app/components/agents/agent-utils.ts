import type { AgentFormValues } from "@/lib/schemas/agent";
import type { AgentInput, Catalog } from "@/lib/api/agents";

export const emptyCatalog: Catalog = {
  models: [],
  canManageModels: false,
  presets: [],
  tools: [],
  mcps: [],
  skills: [],
};

export function defaultAgentForm(catalog: Catalog): AgentFormValues {
  const model = catalog.models.find((m) => m.enabled && m.curated) ??
    catalog.models.find((m) => m.enabled);
  return {
    name: "",
    description: "",
    instruction: "",
    modelId: model?.id ?? 0,
    tools: [],
    capabilityPresets: catalog.presets
      .filter((p) => p.defaultEnabled && p.key !== "system")
      .map((p) => p.key),
    toolPolicies: {},
    maxTokens: Math.min(model?.maxOutputTokens || 4096, 4096),
    mcpBindings: [],
    skills: [],
  };
}

export function toAgentInput(values: AgentFormValues): AgentInput {
  return {
    name: values.name.trim(),
    description: values.description.trim(),
    instruction: values.instruction,
    modelId: values.modelId,
    tools: values.tools,
    capabilityPresets: values.capabilityPresets,
    toolPolicies: values.toolPolicies,
    maxTokens: values.maxTokens,
    mcpBindings: values.mcpBindings,
    skills: values.skills,
  };
}

export function formatDuration(start?: string | null, end?: string | null): string {
  if (!start) return "-";
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return "-";
  const sec = Math.round((e - s) / 1000);
  if (sec < 1) return "<1s";
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export function formatTime(value?: string | null): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString();
}

export function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

export function bindableMCP(server: Catalog["mcps"][number], currentUserId: number): boolean {
  return (
    server.enabled &&
    (server.reviewStatus === "approved" ||
      (server.ownerUserId === currentUserId && server.visibility === "private"))
  );
}

export function bindableSkill(skill: Catalog["skills"][number], currentUserId: number): boolean {
  return (
    skill.reviewStatus === "approved" ||
    (skill.ownerUserId === currentUserId && skill.visibility === "private")
  );
}
