import { queryOptions } from "@tanstack/react-query";
import { agentApi } from "@/lib/api/agents";

export const agentKeys = {
  all: ["agents"] as const,
  list: () => [...agentKeys.all, "list"] as const,
  detail: (id: number) => [...agentKeys.all, "detail", id] as const,
  versions: (id: number) => [...agentKeys.all, "versions", id] as const,
  version: (id: number, version: number) =>
    [...agentKeys.all, "version", id, version] as const,
  runs: (id: number) => [...agentKeys.all, "runs", id] as const,
  run: (id: number) => [...agentKeys.all, "run", id] as const,
  shares: (id: number) => [...agentKeys.all, "shares", id] as const,
  approvals: (status: string) => [...agentKeys.all, "approvals", status] as const,
  catalog: () => [...agentKeys.all, "catalog"] as const,
  modelServices: () => [...agentKeys.all, "model-services"] as const,
  mcpServers: () => [...agentKeys.all, "mcp-servers"] as const,
  skills: () => [...agentKeys.all, "skills"] as const,
};

export const agentsQuery = queryOptions({
  queryKey: agentKeys.list(),
  queryFn: () => agentApi.list(),
});

export const agentDetailQuery = (id: number) =>
  queryOptions({
    queryKey: agentKeys.detail(id),
    queryFn: () => agentApi.detail(id),
    enabled: id > 0,
  });

export const agentVersionsQuery = (id: number) =>
  queryOptions({
    queryKey: agentKeys.versions(id),
    queryFn: () => agentApi.versions(id),
    enabled: id > 0,
  });

export const agentVersionQuery = (id: number, version: number) =>
  queryOptions({
    queryKey: agentKeys.version(id, version),
    queryFn: () => agentApi.version(id, version),
    enabled: id > 0 && version > 0,
  });

export const agentRunsQuery = (id: number) =>
  queryOptions({
    queryKey: agentKeys.runs(id),
    queryFn: () => agentApi.runs(id),
    enabled: id > 0,
    refetchInterval: 4000,
  });

export const agentRunQuery = (runId: number) =>
  queryOptions({
    queryKey: agentKeys.run(runId),
    queryFn: () => agentApi.runDetail(runId),
    enabled: runId > 0,
    refetchInterval: 2000,
  });

export const agentSharesQuery = (id: number) =>
  queryOptions({
    queryKey: agentKeys.shares(id),
    queryFn: () => agentApi.shares(id),
    enabled: id > 0,
  });

export const approvalsQuery = (status = "pending") =>
  queryOptions({
    queryKey: agentKeys.approvals(status),
    queryFn: () => agentApi.approvals(status),
    refetchInterval: 8000,
  });

export const catalogQuery = queryOptions({
  queryKey: agentKeys.catalog(),
  queryFn: async () => {
    const [services, tools, mcps, skills] = await Promise.all([
      agentApi.listModelServices(),
      agentApi.agentTools(),
      agentApi.listMCPServers(),
      agentApi.listSkills(),
    ]);
    return {
      services: services.services,
      canManageModels: services.canManage,
      presets: tools.presets,
      tools: tools.tools,
      mcps: mcps.servers,
      skills: skills.skills,
    };
  },
});
