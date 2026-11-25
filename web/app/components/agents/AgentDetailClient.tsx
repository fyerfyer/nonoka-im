"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Bot,
  History,
  Loader2,
  MessageSquare,
  Power,
  Save,
  Send,
  Share2,
  Trash2,
} from "lucide-react";
import {
  agentApi,
  type ManagedAgent,
} from "@/lib/api/agents";
import { agentFormSchema, type AgentFormValues } from "@/lib/schemas/agent";
import { agentDetailQuery, agentKeys, catalogQuery } from "@/lib/queries/agents";
import { useAuth } from "@/hooks/useAuth";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import { getP2PTopic } from "@/lib/topic";
import { invalidateAgentNames } from "@/hooks/useAgentNames";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { AgentBuildForm } from "./AgentBuildForm";
import { CapabilityPicker } from "./CapabilityPicker";
import { AgentOverview } from "./AgentOverview";
import { AgentAccessPanel } from "./AgentAccessPanel";
import { AgentPlayground } from "./AgentPlayground";
import { AgentVersions } from "./AgentVersions";
import { AgentRuns } from "./AgentRuns";
import {
  defaultAgentForm,
  emptyCatalog,
  errorText,
  toAgentInput,
} from "./agent-utils";

const TABS = ["overview", "build", "tools", "test", "runs"] as const;
type Tab = (typeof TABS)[number];

function isTab(value: string | undefined): value is Tab {
  return !!value && (TABS as readonly string[]).includes(value);
}

function formFromAgent(agent: ManagedAgent): AgentFormValues {
  const draft = agent.draft;
  return {
    name: agent.name,
    description: agent.description,
    instruction: draft?.instruction || "",
    modelId: draft?.modelId || 0,
    tools: draft?.tools || [],
    capabilityPresets: draft?.capabilityPresets || [],
    toolPolicies: draft?.toolPolicies || {},
    maxTokens: draft?.maxTokens || 4096,
    mcpBindings: draft?.mcpBindings || [],
    skills: draft?.skills || [],
  };
}

export function AgentDetailClient({
  agentId,
  initialTab,
}: {
  agentId: number;
  initialTab?: string;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const { isReady, isAuthenticated } = useAuth();
  const detail = useQuery(agentDetailQuery(agentId));
  const catalog = useQuery(catalogQuery);
  const [tab, setTab] = useState<Tab>(isTab(initialTab) ? initialTab : "overview");
  const [publishOpen, setPublishOpen] = useState(false);
  const [publishNote, setPublishNote] = useState("");
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const initialized = useRef("");

  const form = useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: defaultAgentForm(emptyCatalog),
  });
  const {
    formState: { isDirty },
    reset,
  } = form;

  useEffect(() => {
    const agent = detail.data;
    if (!agent?.draft || !catalog.data) return;
    const key = `${agent.id}:${agent.draft.revision}`;
    if (initialized.current === key) return;
    initialized.current = key;
    reset(formFromAgent(agent));
  }, [detail.data, catalog.data, reset]);

  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => {
      if (!isDirty) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);

  const saveMutation = useMutation({
    mutationFn: async ({ publish, note }: { publish: boolean; note?: string }) => {
      const agent = detail.data;
      if (!agent?.draft) throw new Error("This agent has no editable draft");
      const values = form.getValues();
      const updated = await agentApi.saveDraft(agent.id, {
        ...toAgentInput(values),
        revision: agent.draft.revision,
      });
      if (publish) {
        await agentApi.publish(agent.id, updated.draft!.revision, note || "");
      }
      return updated;
    },
    onSuccess: (updated, variables) => {
      queryClient.setQueryData(agentKeys.detail(agentId), updated);
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(agentId) });
      queryClient.invalidateQueries({ queryKey: agentKeys.list() });
      queryClient.invalidateQueries({ queryKey: agentKeys.versions(agentId) });
      reset(formFromAgent(updated));
      initialized.current = `${updated.id}:${updated.draft?.revision ?? 0}`;
      setPublishOpen(false);
      setPublishNote("");
      toast.success(variables.publish ? "Version published" : "Draft saved");
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const statusMutation = useMutation({
    mutationFn: (status: "active" | "disabled") => agentApi.setStatus(agentId, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(agentId) });
      queryClient.invalidateQueries({ queryKey: agentKeys.list() });
      toast.success("Agent status updated");
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const deleteMutation = useMutation({
    mutationFn: () => agentApi.delete(agentId),
    onSuccess: () => {
      invalidateAgentNames();
      queryClient.invalidateQueries({ queryKey: agentKeys.list() });
      toast.success("Agent deleted");
      router.replace("/agents");
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const agent = detail.data;
  const canManage = agent?.canManage ?? false;
  const canOwn = agent?.canOwn ?? false;
  const visibleTabs = useMemo(
    () => TABS.filter((item) => item === "overview" || canManage),
    [canManage]
  );

  const changeTab = (next: Tab) => {
    if (isDirty && !window.confirm("Discard unsaved changes?")) return;
    reset(agent ? formFromAgent(agent) : form.getValues());
    setTab(next);
    router.replace(`/agents/${agentId}?tab=${next}`, { scroll: false });
  };

  const chatWithBot = () => {
    if (!agent) return;
    if (agent.currentVersion === 0) {
      toast.error("Publish the agent first to start chatting");
      return;
    }
    const topic = getP2PTopic(Number(user?.userId), agent.botUserId);
    useChatStore.getState().upsertConversation({
      topic,
      type: "p2p",
      peerId: agent.botUserId,
      peerUsername: agent.name,
      peerIsBot: true,
    });
    router.push(`/chat/${encodeURIComponent(topic)}`);
  };

  if (!isReady || !isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  if (detail.isPending || catalog.isPending) {
    return (
      <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading agent…
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3">
        <p className="text-sm text-muted-foreground">Agent not found.</p>
        <Button onClick={() => router.push("/agents")}>Back to agents</Button>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-20 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
          <Button
            variant="ghost"
            size="icon"
            title="Back to agents"
            onClick={() => router.push("/agents")}
          >
            <ArrowLeft />
          </Button>
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10">
            <Bot className="h-4 w-4 text-primary" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-base font-semibold">{agent.name}</h1>
              <Badge variant={agent.status === "active" ? "default" : "secondary"}>
                {agent.status}
              </Badge>
              {agent.draftChanged && (
                <Badge variant="outline" className="border-amber-400 text-amber-700 dark:text-amber-400">
                  draft changed
                </Badge>
              )}
            </div>
            <p className="truncate text-xs text-muted-foreground">
              Bot #{agent.botUserId} · version{" "}
              {agent.currentVersion ? `v${agent.currentVersion}` : "unpublished"}
              {agent.published?.modelKey ? ` · ${agent.published.modelKey}` : ""}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            {canManage && (
              <Button
                variant="outline"
                size="sm"
                disabled={saveMutation.isPending}
                onClick={() => saveMutation.mutate({ publish: false })}
              >
                {saveMutation.isPending && !publishOpen ? (
                  <Loader2 className="animate-spin" />
                ) : (
                  <Save />
                )}
                Save draft
              </Button>
            )}
            {canManage && (
              <Button size="sm" onClick={() => setPublishOpen(true)}>
                <Send />
                Publish
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={chatWithBot}
              disabled={agent.currentVersion === 0}
              title={agent.currentVersion === 0 ? "Publish first to chat" : "Open chat"}
            >
              <MessageSquare />
              Chat
            </Button>
            {canManage && (
              <Button variant="outline" size="sm" onClick={() => setShareOpen(true)}>
                <Share2 />
                Share
              </Button>
            )}
            {canManage && (
              <Button
                variant="ghost"
                size="icon"
                title="Version history"
                onClick={() => setHistoryOpen(true)}
              >
                <History />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              title={agent.status === "active" ? "Disable" : "Enable"}
              disabled={statusMutation.isPending || !canManage || agent.currentVersion === 0}
              onClick={() =>
                statusMutation.mutate(agent.status === "active" ? "disabled" : "active")
              }
            >
              <Power />
            </Button>
            {canOwn && (
              <Button
                variant="ghost"
                size="icon"
                title="Delete agent"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 />
              </Button>
            )}
          </div>
        </div>

        <div className="mx-auto flex max-w-7xl gap-1 overflow-x-auto px-4 sm:px-6">
          {visibleTabs.map((item) => (
            <button
              key={item}
              className={cn(
                "border-b-2 px-3 py-2 text-sm capitalize transition-colors",
                tab === item
                  ? "border-primary font-medium text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              )}
              onClick={() => changeTab(item)}
            >
              {item}
            </button>
          ))}
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-5 sm:px-6">
        {tab === "overview" && (
          <AgentOverview
            agent={agent}
            onTab={(next) => changeTab(next as Tab)}
            onOpenHistory={() => setHistoryOpen(true)}
          />
        )}
        {tab === "build" && canManage && (
          <div className="max-w-3xl">
            <AgentBuildForm form={form} catalog={catalog.data || emptyCatalog} />
          </div>
        )}
        {tab === "tools" && canManage && (
          <div className="max-w-3xl">
            <CapabilityPicker
              form={form}
              catalog={catalog.data || emptyCatalog}
              currentUserId={Number(user?.userId)}
            />
          </div>
        )}
        {tab === "test" && canManage && <AgentPlayground agentId={agentId} />}
        {tab === "runs" && canManage && <AgentRuns agentId={agentId} />}
      </main>

      <Dialog open={publishOpen} onOpenChange={setPublishOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Publish version</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Saves the current draft and makes it the version new chats use.
          </p>
          <Input
            placeholder="Publish note (optional)"
            value={publishNote}
            maxLength={1000}
            onChange={(event) => setPublishNote(event.target.value)}
          />
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setPublishOpen(false)}>
              Cancel
            </Button>
            <Button
              disabled={saveMutation.isPending}
              onClick={() => saveMutation.mutate({ publish: true, note: publishNote })}
            >
              {saveMutation.isPending ? <Loader2 className="animate-spin" /> : <Send />}
              Publish
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete agent</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Delete <span className="font-medium text-foreground">{agent.name}</span>?
            Versions, runs and shares are removed. The bot account stays as an
            inert user. This cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              {deleteMutation.isPending ? <Loader2 className="animate-spin" /> : <Trash2 />}
              Delete
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={shareOpen} onOpenChange={setShareOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>Share agent</DialogTitle>
          </DialogHeader>
          <AgentAccessPanel agentId={agentId} canOwn={canOwn} />
        </DialogContent>
      </Dialog>

      <Dialog open={historyOpen} onOpenChange={setHistoryOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>Version history</DialogTitle>
          </DialogHeader>
          <AgentVersions agent={agent} />
        </DialogContent>
      </Dialog>
    </div>
  );
}
