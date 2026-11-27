"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { usePendingApprovals } from "@/hooks/usePendingApprovals";
import { useIsPlatformAdmin } from "@/hooks/useIsPlatformAdmin";
import { useAuthStore } from "@/stores/authStore";
import { WORKSPACE_NAV } from "./nav";
import { UserMenu } from "./UserMenu";

export function MobileTabBar() {
  const pathname = usePathname();
  const user = useAuthStore((state) => state.user);
  const canManagePlatform = useIsPlatformAdmin();
  const pendingApprovals = usePendingApprovals();
  const initials = (user?.username || "?").slice(0, 2).toUpperCase();

  const items = WORKSPACE_NAV.filter(
    (item) => !item.adminOnly && !item.href.startsWith("/admin")
  );

  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-40 flex h-14 items-stretch border-t border-border bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
      aria-label="Workspace"
      data-testid="mobile-tab-bar"
    >
      {items.map((item) => {
        const active = item.match(pathname);
        const badge = item.href === "/approvals" && pendingApprovals.count > 0;
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-label={item.label}
            aria-current={active ? "page" : undefined}
            className={cn(
              "relative flex flex-1 flex-col items-center justify-center gap-0.5 text-[10px] font-medium transition-colors",
              active ? "text-primary" : "text-muted-foreground"
            )}
          >
            <item.icon className="h-5 w-5" />
            <span>{item.label}</span>
            {badge && (
              <span className="absolute right-[calc(50%-18px)] top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[9px] font-semibold text-primary-foreground">
                {pendingApprovals.count > 9 ? "9+" : pendingApprovals.count}
              </span>
            )}
          </Link>
        );
      })}
      <UserMenu canManagePlatform={canManagePlatform} side="top">
        <button
          type="button"
          className="flex w-14 flex-col items-center justify-center text-[10px] font-medium text-muted-foreground"
          aria-label="Account menu"
        >
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/10 text-[10px] font-bold text-primary">
            {initials}
          </span>
        </button>
      </UserMenu>
    </nav>
  );
}
