import { api } from "@/lib/api";

export interface GatewayNode {
  nodeId: string;
  url: string;
  grpcAddr: string;
  connections: number;
  heartbeatAt: string;
  alive: boolean;
}

export interface GatewayList {
  strategy: string;
  nodeTtlSeconds: number;
  degraded: boolean;
  warning?: string;
  gateways: GatewayNode[];
}

export const systemApi = {
  gateways: () => api.get<GatewayList>("/v1/admin/gateways"),
};
