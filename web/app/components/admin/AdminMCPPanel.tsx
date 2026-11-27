"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  CirclePlus,
  Loader2,
  Power,
  PowerOff,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import {
  agentApi,
  type MCPServer,
  type Visibility,
} from "@/lib/api/agents";
import { useIsPlatformAdmin } from "@/hooks/useIsPlatformAdmin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

export function AdminMCPPanel() {
  const queryClient = useQueryClient();
  const canManage = useIsPlatformAdmin();
  const [connectionInputs, setConnectionInputs] = useState<Record<number, string>>(
    {}
  );
  const [serverName, setServerName] = useState("");
  const [serverDisplay, setServerDisplay] = useState("");
  const [serverURL, setServerURL] = useState("");
  const [serverAuth, setServerAuth] = useState<"none" | "bearer" | "header">(
    "none"
  );
  const [serverHeader, setServerHeader] = useState("");
  const [serverCredential, setServerCredential] = useState("");
  const [serverVisibility, setServerVisibility] =
    useState<Visibility>("private");

  const serversQuery = useQuery({
    queryKey: ["admin", "mcp-servers"],
    queryFn: () => agentApi.listMCPServers(),
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["admin", "mcp-servers"] });
    void queryClient.invalidateQueries({ queryKey: ["agents", "catalog"] });
  };

  const register = useMutation({
    mutationFn: () =>
      agentApi.registerMCPServer({
        name: serverName.trim(),
        displayName: serverDisplay.trim() || undefined,
        url: serverURL.trim(),
        transport: "streamable_http",
        authType: serverAuth,
        headerName: serverAuth === "header" ? serverHeader.trim() : undefined,
        credential: serverAuth === "none" ? undefined : serverCredential,
        visibility: serverVisibility,
        enabled: true,
      }),
    onSuccess: () => {
      toast.success("MCP server registered");
      setServerName("");
      setServerDisplay("");
      setServerURL("");
      setServerCredential("");
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const act = useMutation({
    mutationFn: async ({
      task,
      success,
    }: {
      task: () => Promise<unknown>;
      success: string;
    }) => {
      await task();
      return success;
    },
    onSuccess: (message) => {
      toast.success(message);
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const connect = (server: MCPServer) => {
    const credential = connectionInputs[server.id] ?? "";
    if (!credential) {
      toast.error("Credential is required");
      return;
    }
    act.mutate({
      task: () =>
        agentApi.connectMCPServer(server.id, {
          authType: server.authType,
          credential,
        }),
      success: "MCP connection saved",
    });
  };

  const servers = serversQuery.data?.servers ?? [];

  return (
    <div className="space-y-5">
      <div className="rounded-xl border bg-card p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">MCP servers</h2>
            <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
              Shared and public servers require platform review before an agent
              can bind them. Private servers may be used by their owner without
              review.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-1 text-xs">
            <span className="text-muted-foreground">Quick add:</span>
            <Button
              size="xs"
              variant="outline"
              onClick={() => {
                setServerName("deepwiki");
                setServerDisplay("DeepWiki");
                setServerURL("https://mcp.deepwiki.com/mcp");
                setServerAuth("none");
                setServerVisibility("private");
              }}
            >
              DeepWiki
            </Button>
            <Button
              size="xs"
              variant="outline"
              onClick={() => {
                setServerName("context7");
                setServerDisplay("Context7");
                setServerURL("https://mcp.context7.com/mcp");
                setServerAuth("none");
                setServerVisibility("private");
              }}
            >
              Context7
            </Button>
          </div>
        </div>

        <div className="mt-4 space-y-3 rounded-lg border p-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block text-xs font-medium">
              Name
              <Input
                className="mt-1"
                value={serverName}
                onChange={(event) => setServerName(event.target.value)}
                placeholder="my-search"
              />
            </label>
            <label className="block text-xs font-medium">
              Display name
              <Input
                className="mt-1"
                value={serverDisplay}
                onChange={(event) => setServerDisplay(event.target.value)}
              />
            </label>
          </div>
          <label className="block text-xs font-medium">
            URL
            <Input
              className="mt-1"
              value={serverURL}
              onChange={(event) => setServerURL(event.target.value)}
              placeholder="https://mcp.example.com/mcp"
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block text-xs font-medium">
              Auth
              <select
                className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm"
                value={serverAuth}
                onChange={(event) =>
                  setServerAuth(event.target.value as typeof serverAuth)
                }
              >
                <option value="none">none</option>
                <option value="bearer">bearer</option>
                <option value="header">header</option>
              </select>
            </label>
            <label className="block text-xs font-medium">
              Visibility
              <select
                className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm"
                value={serverVisibility}
                onChange={(event) =>
                  setServerVisibility(event.target.value as Visibility)
                }
              >
                <option value="private">private</option>
                <option value="shared">shared</option>
                <option value="public">public</option>
              </select>
            </label>
            {serverAuth === "header" && (
              <label className="block text-xs font-medium">
                Header name
                <Input
                  className="mt-1"
                  value={serverHeader}
                  onChange={(event) => setServerHeader(event.target.value)}
                  placeholder="X-API-Key"
                />
              </label>
            )}
          </div>
          {serverAuth !== "none" && (
            <label className="block text-xs font-medium">
              Credential (write-only)
              <Input
                className="mt-1"
                type="password"
                value={serverCredential}
                onChange={(event) => setServerCredential(event.target.value)}
              />
            </label>
          )}
          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={
                register.isPending ||
                !serverName.trim() ||
                !serverURL.trim()
              }
              onClick={() => register.mutate()}
            >
              {register.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <CirclePlus />
              )}
              Register server
            </Button>
          </div>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="text-sm font-semibold">Registered servers</h2>
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={() => void serversQuery.refetch()}
            disabled={serversQuery.isFetching}
          >
            <RefreshCw
              className={cn(serversQuery.isFetching && "animate-spin")}
            />
          </Button>
        </div>
        {servers.length === 0 ? (
          <p className="p-6 text-center text-sm text-muted-foreground">
            No MCP servers registered.
          </p>
        ) : (
          <div className="divide-y">
            {servers.map((server) => (
              <div key={server.id} className="space-y-2 px-4 py-3 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">
                    {server.displayName || server.name}
                  </span>
                  <Badge variant="secondary" className="text-[10px]">
                    {server.visibility}
                  </Badge>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px]",
                      server.reviewStatus === "approved" && "bg-success/10 text-success",
                      server.reviewStatus === "pending" && "bg-warning/10 text-warning"
                    )}
                  >
                    {server.reviewStatus}
                  </Badge>
                  <Badge
                    variant="secondary"
                    className={cn(
                      "text-[10px]",
                      server.healthStatus === "healthy" && "bg-success/10 text-success",
                      server.healthStatus === "error" && "bg-destructive/10 text-destructive"
                    )}
                  >
                    {server.healthStatus}
                  </Badge>
                  <span
                    className="max-w-[22rem] truncate font-mono text-[10px] text-muted-foreground"
                    title={server.url}
                  >
                    {server.url}
                  </span>
                  <div className="ml-auto flex flex-wrap gap-1">
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.discoverMCPServer(server.id),
                          success: "Tools discovered",
                        })
                      }
                    >
                      <RefreshCw />
                      Discover
                    </Button>
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.testMCPServer(server.id),
                          success: "Health check passed",
                        })
                      }
                    >
                      <ShieldCheck />
                      Test
                    </Button>
                    <Button
                      size="xs"
                      variant="outline"
                      title={server.enabled ? "Disable server" : "Enable server"}
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () =>
                            agentApi.setMCPServerStatus(server.id, !server.enabled),
                          success: server.enabled
                            ? "Server disabled"
                            : "Server enabled",
                        })
                      }
                    >
                      {server.enabled ? <PowerOff /> : <Power />}
                    </Button>
                    {canManage && server.reviewStatus === "pending" && (
                      <>
                        <Button
                          size="xs"
                          disabled={act.isPending}
                          onClick={() =>
                            act.mutate({
                              task: () =>
                                agentApi.reviewMCPServer(server.id, "approved"),
                              success: "Server approved",
                            })
                          }
                        >
                          Approve
                        </Button>
                        <Button
                          size="xs"
                          variant="outline"
                          disabled={act.isPending}
                          onClick={() =>
                            act.mutate({
                              task: () =>
                                agentApi.reviewMCPServer(server.id, "rejected"),
                              success: "Server rejected",
                            })
                          }
                        >
                          Reject
                        </Button>
                      </>
                    )}
                    <Button
                      size="xs"
                      variant="ghost"
                      title="Delete server"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.deleteMCPServer(server.id),
                          success: "Server deleted",
                        })
                      }
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </div>
                {server.tools && server.tools.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {server.tools.map((tool) => (
                      <Badge
                        key={tool.name}
                        variant="secondary"
                        className="text-[10px]"
                      >
                        {tool.name} · {tool.risk}
                      </Badge>
                    ))}
                  </div>
                )}
                {server.authType !== "none" && (
                  <div className="flex flex-wrap items-end gap-2">
                    <label className="block text-xs font-medium">
                      My connection credential
                      <Input
                        className="mt-1"
                        type="password"
                        value={connectionInputs[server.id] ?? ""}
                        onChange={(event) =>
                          setConnectionInputs((old) => ({
                            ...old,
                            [server.id]: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() => connect(server)}
                    >
                      Save connection
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.disconnectMCPServer(server.id),
                          success: "Connection removed",
                        })
                      }
                    >
                      Disconnect
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
