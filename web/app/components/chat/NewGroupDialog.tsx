"use client";

import { useState, useCallback, useEffect } from "react";
import { useRouter } from "next/navigation";
import { userApi, groupApi } from "@/lib/api";
import { useDebounce } from "@/hooks/useDebounce";
import { User } from "@/types";
import { useAuthStore } from "@/stores/authStore";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Search, Loader2, Check, Users } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

interface NewGroupDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function NewGroupDialog({ open, onOpenChange }: NewGroupDialogProps) {
  const currentUser = useAuthStore((s) => s.user);
  const [name, setName] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [results, setResults] = useState<User[]>([]);
  const [selected, setSelected] = useState<User[]>([]);
  const router = useRouter();
  const debouncedQuery = useDebounce(query, 400);

  useEffect(() => {
    if (!open) {
      setName("");
      setQuery("");
      setResults([]);
      setSelected([]);
    }
  }, [open]);

  const search = useCallback(
    async (q: string) => {
      if (!q.trim()) {
        setResults([]);
        return;
      }
      setLoading(true);
      try {
        const res = await userApi.searchUsers(q.trim());
        setResults(
          res.users.filter((u) => u.userId !== currentUser?.userId)
        );
      } catch (err) {
        toast.error(
          `Search failed: ${err instanceof Error ? err.message : "unknown"}`
        );
      } finally {
        setLoading(false);
      }
    },
    [currentUser?.userId]
  );

  useEffect(() => {
    search(debouncedQuery);
  }, [debouncedQuery, search]);

  const toggleUser = (user: User) => {
    setSelected((prev) => {
      const exists = prev.find((u) => u.userId === user.userId);
      if (exists) return prev.filter((u) => u.userId !== user.userId);
      return [...prev, user];
    });
  };

  const handleCreate = async () => {
    if (!name.trim() || selected.length === 0) return;
    setCreating(true);
    try {
      const group = await groupApi.createGroup(
        name.trim(),
        selected.map((u) => u.userId)
      );
      toast.success(`Group "${name.trim()}" created`);
      onOpenChange(false);
      router.push(`/chat/${encodeURIComponent(group.topic)}`);
    } catch (err) {
      toast.error(
        `Create group failed: ${err instanceof Error ? err.message : "unknown"}`
      );
    } finally {
      setCreating(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New Group</DialogTitle>
        </DialogHeader>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Group name"
          className="mb-2"
          autoFocus
        />
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Add members by username"
            className="pl-9"
          />
        </div>

        {selected.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {selected.map((u) => (
              <button
                key={u.userId}
                onClick={() => toggleUser(u)}
                className="flex items-center gap-1 rounded-full bg-primary/10 px-2 py-1 text-xs font-medium text-primary hover:bg-primary/20"
              >
                {u.username}
                <span className="text-[10px]">×</span>
              </button>
            ))}
          </div>
        )}

        <div className="max-h-[240px] overflow-y-auto">
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
              {results.map((u) => {
                const isSelected = selected.some(
                  (s) => s.userId === u.userId
                );
                return (
                  <Button
                    key={u.userId}
                    variant="ghost"
                    className={cn(
                      "h-auto justify-start gap-3 px-2 py-2",
                      isSelected && "bg-primary/10"
                    )}
                    onClick={() => toggleUser(u)}
                  >
                    <Avatar className="h-9 w-9">
                      <AvatarFallback className="bg-primary/10 text-primary text-xs">
                        {u.username.slice(0, 2).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <div className="flex flex-1 flex-col items-start">
                      <span className="text-sm font-medium">{u.username}</span>
                      <span className="text-xs text-muted-foreground">
                        ID: {u.userId}
                      </span>
                    </div>
                    {isSelected && <Check className="h-4 w-4 text-primary" />}
                  </Button>
                );
              })}
            </div>
          )}
        </div>

        <Button
          onClick={handleCreate}
          disabled={!name.trim() || selected.length === 0 || creating}
          className="w-full"
        >
          {creating ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Users className="mr-2 h-4 w-4" />
          )}
          Create Group
        </Button>
      </DialogContent>
    </Dialog>
  );
}
