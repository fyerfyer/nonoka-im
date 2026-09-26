"use client";

import { useCallback, useEffect, useState } from "react";
import { groupApi, userApi } from "@/lib/api";
import { Group, GroupMember, User } from "@/types";
import { useAuthStore } from "@/stores/authStore";
import { useChatStore } from "@/stores/chatStore";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Loader2, UserMinus, UserPlus } from "lucide-react";
import { toast } from "sonner";

interface GroupMembersDialogProps {
  group: Group;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function GroupMembersDialog({
  group,
  open,
  onOpenChange,
}: GroupMembersDialogProps) {
  const currentUser = useAuthStore((s) => s.user);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState("");
  const [candidates, setCandidates] = useState<User[]>([]);
  const [searching, setSearching] = useState(false);

  const loadMembers = useCallback(async () => {
    setLoading(true);
    try {
      const reply = await groupApi.listMembers(group.groupId);
      setMembers(reply.members);
      useChatStore.getState().setMemberNames(group.topic, reply.members);
    } catch (err) {
      toast.error(
        `Failed to load members: ${err instanceof Error ? err.message : "unknown"}`
      );
    } finally {
      setLoading(false);
    }
  }, [group.groupId, group.topic]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setCandidates([]);
      void loadMembers();
    }
  }, [open, loadMembers]);

  // Debounced candidate search for adding members.
  useEffect(() => {
    if (!query.trim()) {
      setCandidates([]);
      return;
    }
    setSearching(true);
    const timer = setTimeout(async () => {
      try {
        const reply = await userApi.searchUsers(query.trim(), 10);
        setCandidates(
          reply.users.filter((u) => !members.some((m) => m.userId === u.userId))
        );
      } catch {
        setCandidates([]);
      } finally {
        setSearching(false);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [query, members]);

  const addMember = async (userId: number) => {
    try {
      await groupApi.addMember(group.groupId, userId);
      setQuery("");
      setCandidates([]);
      await loadMembers();
      toast.success("Member added");
    } catch (err) {
      toast.error(
        `Failed to add member: ${err instanceof Error ? err.message : "unknown"}`
      );
    }
  };

  const removeMember = async (member: GroupMember) => {
    try {
      await groupApi.removeMember(group.groupId, member.userId);
      await loadMembers();
      toast.success(
        member.userId === currentUser?.userId ? "Left group" : "Member removed"
      );
    } catch (err) {
      toast.error(
        `Failed to remove member: ${err instanceof Error ? err.message : "unknown"}`
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{group.name} · Members</DialogTitle>
        </DialogHeader>

        <div className="space-y-2">
          <Input
            placeholder="Search users to add..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {searching && (
            <div className="flex justify-center py-1">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}
          {candidates.length > 0 && (
            <div className="rounded-md border">
              {candidates.map((u) => (
                <button
                  key={u.userId}
                  className="flex w-full items-center justify-between px-3 py-2 text-sm hover:bg-accent"
                  onClick={() => addMember(u.userId)}
                >
                  <span>{u.username}</span>
                  <UserPlus className="h-4 w-4 text-muted-foreground" />
                </button>
              ))}
            </div>
          )}
        </div>

        <ScrollArea className="max-h-64">
          {loading ? (
            <div className="flex justify-center py-6">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <ul className="divide-y">
              {members.map((m) => (
                <li key={m.userId} className="flex items-center gap-3 py-2">
                  <Avatar className="h-8 w-8">
                    <AvatarFallback className="bg-primary/10 text-xs text-primary">
                      {m.username.slice(0, 2).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                  <span className="flex-1 text-sm">
                    {m.username}
                    {m.userId === group.ownerId && (
                      <span className="ml-2 text-xs text-muted-foreground">
                        owner
                      </span>
                    )}
                    {m.userId === currentUser?.userId && (
                      <span className="ml-2 text-xs text-muted-foreground">
                        you
                      </span>
                    )}
                  </span>
                  {m.userId !== group.ownerId && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-muted-foreground hover:text-destructive"
                      title={
                        m.userId === currentUser?.userId
                          ? "Leave group"
                          : "Remove member"
                      }
                      onClick={() => removeMember(m)}
                    >
                      <UserMinus className="h-4 w-4" />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
