"use client";

import { useQuery } from "@tanstack/react-query";
import { approvalsQuery } from "@/lib/queries/agents";
import { useAuthStore } from "@/stores/authStore";

export function usePendingApprovals() {
  const token = useAuthStore((state) => state.token);
  const query = useQuery({
    ...approvalsQuery("pending"),
    enabled: !!token,
  });
  return {
    approvals: query.data?.items ?? [],
    count: query.data?.items.length ?? 0,
    isLoading: query.isPending,
  };
}
