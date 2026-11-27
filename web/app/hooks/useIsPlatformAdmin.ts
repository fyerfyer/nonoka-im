"use client";

import { useQuery } from "@tanstack/react-query";
import { agentApi } from "@/lib/api/agents";
import { useAuthStore } from "@/stores/authStore";

function useAdminProbe() {
  const token = useAuthStore((state) => state.token);
  return useQuery({
    queryKey: ["me", "platform-admin"],
    queryFn: async () => (await agentApi.listModels()).canManage,
    enabled: !!token,
    staleTime: 5 * 60_000,
    retry: 0,
  });
}

// Model management is a platform-admin capability, so the model catalog's
// canManage flag doubles as a cheap admin probe without adding a whoami route.
export function useIsPlatformAdmin() {
  return useAdminProbe().data === true;
}

export function usePlatformAdminState() {
  const query = useAdminProbe();
  return {
    isAdmin: query.data === true,
    isReady: !query.isPending,
    error: query.error,
  };
}
