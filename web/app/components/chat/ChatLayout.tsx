"use client";

import { useAuth } from "@/hooks/useAuth";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { LogOut } from "lucide-react";

interface ChatLayoutProps {
  children: React.ReactNode;
  sidebar?: React.ReactNode;
}

export function ChatLayout({ children, sidebar }: ChatLayoutProps) {
  const { user, isReady, logout } = useAuth();

  if (!isReady) {
    return (
      <div className="flex h-screen items-center justify-center">
        <Skeleton className="h-8 w-48" />
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center justify-between border-b px-4 py-3">
        <div className="flex items-center gap-2">
          <h1 className="text-lg font-semibold">Nonoka IM</h1>
        </div>
        <div className="flex items-center gap-4">
          {user && (
            <span className="text-muted-foreground text-sm">
              {user.username}
            </span>
          )}
          <Button variant="ghost" size="icon" onClick={logout} title="Logout">
            <LogOut className="h-5 w-5" />
          </Button>
        </div>
      </header>
      <div className="flex min-h-0 flex-1">
        {sidebar && (
          <aside className="hidden w-80 border-r md:block">{sidebar}</aside>
        )}
        <main className="min-h-0 flex-1 overflow-hidden">{children}</main>
      </div>
    </div>
  );
}
