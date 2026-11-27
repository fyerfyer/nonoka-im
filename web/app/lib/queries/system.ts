import { queryOptions } from "@tanstack/react-query";
import { systemApi } from "@/lib/api/system";

export const systemKeys = {
  all: ["system"] as const,
  gateways: () => [...systemKeys.all, "gateways"] as const,
};

export const gatewaysQuery = queryOptions({
  queryKey: systemKeys.gateways(),
  queryFn: () => systemApi.gateways(),
  refetchInterval: 10_000,
  staleTime: 5_000,
});
