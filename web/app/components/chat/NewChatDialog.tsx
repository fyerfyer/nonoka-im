"use client";

import { useState, useCallback, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { userApi } from "@/lib/api";
import { agentApi, type ManagedAgent } from "@/lib/api/agents";
import { botsApi, type Bot as ExternalBot } from "@/lib/api/bots";
import { User } from "@/types";
import { getP2PTopic } from "@/lib/topic";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Search, Loader2, User as UserIcon, Bot } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

interface NewChatDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type Tab = "users" | "agents" | "bots";

export function NewChatDialog({ open, onOpenChange }: NewChatDialogProps) {
  const [tab, setTab] = useState<Tab>("users");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<User[]>([]);
  const [agents, setAgents] = useState<ManagedAgent[] | null>(null);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [bots, setBots] = useState<ExternalBot[] | null>(null);
  const [botsLoading, setBotsLoading] = useState(false);
  const router = useRouter();
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Debounce in the change handler (not an effect) so state updates only
  // happen in event/async contexts.
  useEffect(
    () => () => {
      if (searchTimer.current) clearTimeout(searchTimer.current);
    },
    []
  );

  const runSearch = useCallback(async (q: string) => {
    if (!q.trim()) {
      setResults([]);
      return;
    }
    setLoading(true);
    try {
      const res = await userApi.searchUsers(q.trim());
      setResults(res.users);
    } catch (err) {
      toast.error(
        `Search failed: ${err instanceof Error ? err.message : "unknown"}`
      );
    } finally {
      setLoading(false);
    }
  }, []);

  const handleQueryChange = (value: string) => {
    setQuery(value);
    if (searchTimer.current) clearTimeout(searchTimer.current);
    searchTimer.current = setTimeout(() => runSearch(value), 400);
  };

  const loadAgents = useCallback(async () => {
    setAgentsLoading(true);
    try {
      const res = await agentApi.list();
      setAgents(
        res.items.filter((a) => a.status === "active" && a.currentVersion > 0)
      );
    } catch (err) {
      toast.error(
        `Failed to load agents: ${err instanceof Error ? err.message : "unknown"}`
      );
      setAgents([]);
    } finally {
      setAgentsLoading(false);
    }
  }, []);

  const loadBots = useCallback(async () => {
    setBotsLoading(true);
    try {
      const res = await botsApi.directory();
      setBots(res.items.filter((bot) => bot.status === "active"));
    } catch (err) {
      toast.error(
        `Failed to load bots: ${err instanceof Error ? err.message : "unknown"}`
      );
      setBots([]);
    } finally {
      setBotsLoading(false);
    }
  }, []);

  const switchTab = (next: Tab) => {
    setTab(next);
    if (next === "agents") void loadAgents();
    if (next === "bots") void loadBots();
  };

  const handleSelectUser = (targetUser: User) => {
    const currentUserId = useAuthStore.getState().user?.userId || 0;
    const topic = getP2PTopic(currentUserId, targetUser.userId);
    useChatStore.getState().upsertConversation({
      topic,
      type: "p2p",
      peerId: targetUser.userId,
      peerUsername: targetUser.username,
    });
    onOpenChange(false);
    router.push(`/chat/${encodeURIComponent(topic)}`);
  };

  const handleSelectBot = (bot: ExternalBot) => {
    const currentUserId = useAuthStore.getState().user?.userId || 0;
    const topic = getP2PTopic(currentUserId, Number(bot.botUserId));
    useChatStore.getState().upsertConversation({
      topic,
      type: "p2p",
      peerId: Number(bot.botUserId),
      peerUsername: bot.name,
      peerIsBot: true,
    });
    onOpenChange(false);
    router.push(`/chat/${encodeURIComponent(topic)}`);
  };

  const handleSelectAgent = (agent: ManagedAgent) => {
    const currentUserId = useAuthStore.getState().user?.userId || 0;
    const topic = getP2PTopic(currentUserId, agent.botUserId);
    useChatStore.getState().upsertConversation({
      topic,
      type: "p2p",
      peerId: agent.botUserId,
      peerUsername: agent.name,
      peerIsBot: true,
    });
    onOpenChange(false);
    router.push(`/chat/${encodeURIComponent(topic)}`);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New Chat</DialogTitle>
        </DialogHeader>
        <div className="flex rounded-lg bg-muted p-1">
          {(
            [
              { id: "users", label: "Users", icon: UserIcon },
              { id: "agents", label: "Agents", icon: Bot },
              { id: "bots", label: "Bots", icon: Bot },
            ] as const
          ).map((item) => (
            <button
              key={item.id}
              onClick={() => switchTab(item.id)}
              className={cn(
                "flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                tab === item.id
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <item.icon className="h-3.5 w-3.5" />
              {item.label}
            </button>
          ))}
        </div>
        {tab === "users" ? (
          <>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={query}
                onChange={(e) => handleQueryChange(e.target.value)}
                placeholder="Search by username"
                className="pl-9"
                autoFocus
              />
            </div>
            <div className="max-h-[300px] overflow-y-auto">
              {loading ? (
                <div className="flex justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              ) : results.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {query.trim() ? "No users found" : "Type to search users"}
                </p>
              ) : (
                <div className="flex flex-col gap-1">
                  {results.map((u) => (
                    <Button
                      key={u.userId}
                      variant="ghost"
                      className="h-auto justify-start gap-3 px-2 py-2"
                      onClick={() => handleSelectUser(u)}
                    >
                      <Avatar className="h-9 w-9">
                        <AvatarFallback className="bg-primary/10 text-primary text-xs">
                          {u.username.slice(0, 2).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <div className="flex flex-col items-start">
                        <span className="text-sm font-medium">{u.username}</span>
                        <span className="text-xs text-muted-foreground">
                          ID: {u.userId}
                        </span>
                      </div>
                    </Button>
                  ))}
                </div>
              )}
            </div>
          </>
        ) : tab === "agents" ? (
          <div className="max-h-[300px] overflow-y-auto">
            {agentsLoading ? (
              <div className="flex justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : !agents || agents.length === 0 ? (
              <div className="flex flex-col items-center gap-3 py-8">
                <p className="text-sm text-muted-foreground">
                  No agents available yet. Create one on the Agents page.
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    onOpenChange(false);
                    router.push("/agents");
                  }}
                >
                  Go to Agents
                </Button>
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                {agents.map((agent) => (
                  <Button
                    key={agent.id}
                    variant="ghost"
                    className="h-auto justify-start gap-3 px-2 py-2"
                    onClick={() => handleSelectAgent(agent)}
                  >
                    <Avatar className="h-9 w-9">
                      <AvatarFallback className="bg-primary/10 text-primary">
                        <Bot className="h-4 w-4" />
                      </AvatarFallback>
                    </Avatar>
                    <div className="flex min-w-0 flex-col items-start gap-0.5">
                      <span className="flex items-center gap-1.5 text-sm font-medium">
                        <span className="truncate">{agent.name}</span>
                        <Badge variant="secondary" className="px-1.5 text-[10px]">
                          AI
                        </Badge>
                      </span>
                      <span className="max-w-full truncate text-xs text-muted-foreground">
                        {agent.description || "No description"}
                      </span>
                    </div>
                  </Button>
                ))}
              </div>
            )}
          </div>
        ) : (
          <div className="max-h-[300px] overflow-y-auto">
            {botsLoading ? (
              <div className="flex justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : !bots || bots.length === 0 ? (
              <div className="flex flex-col items-center gap-3 py-8">
                <p className="text-sm text-muted-foreground">
                  No external bots yet. Create one in the Developer console.
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    onOpenChange(false);
                    router.push("/developer");
                  }}
                >
                  Go to Developer console
                </Button>
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                {bots.map((bot) => (
                  <Button
                    key={bot.id}
                    variant="ghost"
                    className="h-auto justify-start gap-3 px-2 py-2"
                    onClick={() => handleSelectBot(bot)}
                  >
                    <Avatar className="h-9 w-9">
                      <AvatarFallback className="bg-primary/10 text-primary">
                        <Bot className="h-4 w-4" />
                      </AvatarFallback>
                    </Avatar>
                    <div className="flex min-w-0 flex-col items-start gap-0.5">
                      <span className="flex items-center gap-1.5 text-sm font-medium">
                        <span className="truncate">{bot.name}</span>
                        <Badge variant="secondary" className="px-1.5 text-[10px]">
                          BOT
                        </Badge>
                      </span>
                      <span className="max-w-full truncate text-xs text-muted-foreground">
                        @{bot.username}
                      </span>
                    </div>
                  </Button>
                ))}
              </div>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
