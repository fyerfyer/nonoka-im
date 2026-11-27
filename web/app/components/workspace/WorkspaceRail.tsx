"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { usePendingApprovals } from "@/hooks/usePendingApprovals";
import { useIsPlatformAdmin } from "@/hooks/useIsPlatformAdmin";
import { useAuthStore } from "@/stores/authStore";
import { WORKSPACE_NAV } from "./nav";
import { ThemeToggle } from "./ThemeToggle";
import { UserMenu } from "./UserMenu";

export function WorkspaceRail() {
  const pathname = usePathname();
  const user = useAuthStore((state) => state.user);
  const canManagePlatform = useIsPlatformAdmin();
  const pendingApprovals = usePendingApprovals();
  const initials = (user?.username || "?").slice(0, 2).toUpperCase();

  const items = WORKSPACE_NAV.filter(
    (item) => !item.adminOnly || canManagePlatform
  );

  return (
    <nav
      className="hidden h-full w-16 shrink-0 flex-col items-center border-r border-sidebar-border bg-sidebar py-3 md:flex"
      aria-label="Workspace"
      data-testid="workspace-rail"
    >
      <Link
        href="/chat"
        className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary text-base font-bold text-primary-foreground shadow-sm shadow-primary/20"
        aria-label="Nonoka home"
      >
        N
      </Link>

      <div className="mt-5 flex flex-1 flex-col items-center gap-1.5">
        {items.map((item) => {
          const active = item.match(pathname);
          const badge = item.href === "/approvals" && pendingApprovals.count > 0;
          return (
            <Link
              key={item.href}
              href={item.href}
              title={item.label}
              aria-label={item.label}
              aria-current={active ? "page" : undefined}
              className={cn(
                "relative flex h-10 w-10 items-center justify-center rounded-xl transition-colors",
                active
                  ? "bg-sidebar-accent text-sidebar-accent-foreground"
                  : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground"
              )}
            >
              <item.icon className="h-[18px] w-[18px]" />
              {active && (
                <span className="absolute -left-2.5 h-5 w-1 rounded-r-full bg-primary" />
              )}
              {badge && (
                <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[9px] font-semibold text-primary-foreground">
                  {pendingApprovals.count > 9 ? "9+" : pendingApprovals.count}
                </span>
              )}
            </Link>
          );
        })}
      </div>

      <div className="flex flex-col items-center gap-1 pb-1">
        <ThemeToggle />
        <UserMenu canManagePlatform={canManagePlatform}>
          <button
            type="button"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-primary/10 text-xs font-bold text-primary transition-colors hover:bg-primary/20"
            title="Account"
            aria-label="Account menu"
          >
            {initials}
          </button>
        </UserMenu>
      </div>
    </nav>
  );
}
