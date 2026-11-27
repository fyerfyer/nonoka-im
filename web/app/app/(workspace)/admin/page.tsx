"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ExternalLink,
  Loader2,
  Settings2,
  ShieldAlert,
} from "lucide-react";
import { usePlatformAdminState } from "@/hooks/useIsPlatformAdmin";
import { PageHeader } from "@/components/workspace/PageHeader";
import { EmptyState } from "@/components/workspace/EmptyState";
import { Button, buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AdminOverview } from "@/components/admin/AdminOverview";
import { AdminModels } from "@/components/admin/AdminModels";
import { AdminMCPPanel } from "@/components/admin/AdminMCPPanel";
import { AdminSkillsPanel } from "@/components/admin/AdminSkillsPanel";
import { AdminQuotas } from "@/components/admin/AdminQuotas";
import { AdminAudit } from "@/components/admin/AdminAudit";
import { AdminDLQ } from "@/components/admin/AdminDLQ";

const TABS = [
  { key: "overview", label: "Overview" },
  { key: "models", label: "Models" },
  { key: "mcp", label: "MCP" },
  { key: "skills", label: "Skills" },
  { key: "quotas", label: "Quotas" },
  { key: "audit", label: "Audit" },
  { key: "dlq", label: "DLQ" },
] as const;

type Tab = (typeof TABS)[number]["key"];

const BIFROST_DASHBOARD_URL =
  process.env.NEXT_PUBLIC_BIFROST_DASHBOARD_URL || "http://localhost:8088";

function isTab(value: string | null): value is Tab {
  return !!value && TABS.some((tab) => tab.key === value);
}

function AdminPageInner() {
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("overview");
  const { isAdmin, isReady, error } = usePlatformAdminState();

  useEffect(() => {
    const requested = new URLSearchParams(window.location.search).get("tab");
    if (isTab(requested)) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- sync the initial tab from a deep link
      setTab(requested);
    }
  }, []);

  if (!isReady) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!isAdmin) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader
          title="Operator console"
          description="Deployment-level administration"
          icon={Settings2}
        />
        <div className="flex flex-1 items-center justify-center p-6">
          <EmptyState
            icon={ShieldAlert}
            title="Platform admin access required"
            description={
              error
                ? `Unable to verify admin access: ${error instanceof Error ? error.message : "request failed"}`
                : "This console is limited to platform administrators. Ask an operator to grant your account access."
            }
            action={
              <Button variant="outline" onClick={() => router.push("/agents")}>
                Back to agents
              </Button>
            }
          />
        </div>
      </div>
    );
  }

  const changeTab = (next: Tab) => {
    setTab(next);
    window.history.replaceState(
      null,
      "",
      next === "overview" ? "/admin" : `/admin?tab=${next}`
    );
  };

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <PageHeader
        title="Operator console"
        description="Gateway state, model catalog and platform controls"
        icon={Settings2}
        actions={
          <a
            href={BIFROST_DASHBOARD_URL}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            <ExternalLink />
            Open Bifrost
          </a>
        }
      />

      <div className="shrink-0 overflow-x-auto border-b bg-background px-2 sm:px-4">
        <div className="flex min-w-max gap-1 py-1.5">
          {TABS.map((item) => (
            <button
              key={item.key}
              className={cn(
                "rounded-lg px-3 py-1.5 text-xs font-medium transition-colors",
                tab === item.key
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
              onClick={() => changeTab(item.key)}
              aria-current={tab === item.key ? "page" : undefined}
            >
              {item.label}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-7xl px-4 py-5 sm:px-6">
          {tab === "overview" && <AdminOverview />}
          {tab === "models" && <AdminModels />}
          {tab === "mcp" && <AdminMCPPanel />}
          {tab === "skills" && <AdminSkillsPanel />}
          {tab === "quotas" && <AdminQuotas />}
          {tab === "audit" && <AdminAudit />}
          {tab === "dlq" && <AdminDLQ />}
        </div>
      </div>
    </div>
  );
}

export default function AdminPage() {
  return <AdminPageInner />;
}
