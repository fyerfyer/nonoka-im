"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { ConversationList } from "@/components/chat/ConversationList";
import { MessageSquareText } from "lucide-react";

function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("token");
}

export default function ChatPage() {
  const [mounted, setMounted] = useState(false);
  const token = useAuthStore((s) => s.token);
  const router = useRouter();

  useEffect(() => {
    setMounted(true);
    if (!getToken()) {
      router.replace("/login");
    }
  }, [router, token]);

  if (!mounted || !getToken()) {
    return (
      <div className="flex h-screen w-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  return (
    <div className="flex h-screen w-full overflow-hidden bg-background">
      <ConversationList />
      <main className="hidden flex-1 flex-col items-center justify-center bg-muted/30 md:flex">
        <div className="flex flex-col items-center gap-4 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
            <MessageSquareText className="h-8 w-8 text-primary" />
          </div>
          <div>
            <h1 className="text-xl font-semibold">Nonoka IM</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Select a conversation to start messaging
            </p>
          </div>
        </div>
      </main>
    </div>
  );
}
