"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter } from "next/navigation";
import { userApi } from "@/lib/api";
import { useDebounce } from "@/hooks/useDebounce";
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
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Search, Loader2, User as UserIcon } from "lucide-react";
import { toast } from "sonner";

interface NewChatDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function NewChatDialog({ open, onOpenChange }: NewChatDialogProps) {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<User[]>([]);
  const router = useRouter();
  const debouncedQuery = useDebounce(query, 400);

  const search = useCallback(
    async (q: string) => {
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
    },
    []
  );

  useEffect(() => {
    search(debouncedQuery);
  }, [debouncedQuery, search]);

  const handleSelect = (targetUser: User) => {
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New Chat</DialogTitle>
        </DialogHeader>
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
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
                  onClick={() => handleSelect(u)}
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
      </DialogContent>
    </Dialog>
  );
}
