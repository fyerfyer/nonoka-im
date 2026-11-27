"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import { LogOut, Monitor, Moon, Settings2, Sun } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { useChatStore } from "@/stores/chatStore";
import { invalidateAgentNames } from "@/hooks/useAgentNames";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function UserMenu({
  canManagePlatform,
  side = "right",
  children,
}: {
  canManagePlatform?: boolean;
  side?: "right" | "top";
  children: React.ReactNode;
}) {
  const { user, logout } = useAuth();
  const { theme, setTheme } = useTheme();
  const router = useRouter();
  const [mounted, setMounted] = useState(false);

  // eslint-disable-next-line react-hooks/set-state-in-effect -- wait for the client theme to resolve
  useEffect(() => setMounted(true), []);

  const handleLogout = () => {
    useChatStore.getState().reset();
    invalidateAgentNames();
    logout();
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" side={side} sideOffset={8}>
        <DropdownMenuLabel className="max-w-48 truncate">
          {user?.username || "Signed in"}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {canManagePlatform && (
          <DropdownMenuItem onSelect={() => router.push("/admin")}>
            <Settings2 className="h-4 w-4" />
            Operator console
          </DropdownMenuItem>
        )}
        {mounted && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">
              Theme
            </DropdownMenuLabel>
            <DropdownMenuItem onSelect={() => setTheme("light")}>
              <Sun className="h-4 w-4" />
              Light
              {theme === "light" && (
                <span className="ml-auto h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setTheme("dark")}>
              <Moon className="h-4 w-4" />
              Dark
              {theme === "dark" && (
                <span className="ml-auto h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setTheme("system")}>
              <Monitor className="h-4 w-4" />
              System
              {theme === "system" && (
                <span className="ml-auto h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="text-destructive focus:text-destructive"
          onSelect={handleLogout}
        >
          <LogOut className="h-4 w-4" />
          Log out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
