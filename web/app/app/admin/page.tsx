"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Loader2, Settings2, ShieldAlert } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { AgentPlatformPanel } from "@/components/admin/AgentPlatformPanel";
import { AdminQuotas } from "@/components/admin/AdminQuotas";
import { AdminAudit } from "@/components/admin/AdminAudit";
import { emptyCatalog } from "@/components/agents/agent-utils";
import { agentApi } from "@/lib/api/agents";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const TABS = [
  { key: "platform", label: "Platform" },
  { key: "quotas", label: "Quotas & usage" },
  { key: "audit", label: "Audit log" },
] as const;

type Tab = (typeof TABS)[number]["key"];

export default function AdminPage() {
  const router = useRouter();
  const { isReady, isAuthenticated } = useAuth();
  const [tab, setTab] = useState<Tab>("platform");
  const [authorized, setAuthorized] = useState<boolean | null>(null);

  useEffect(() => {
    if (!isReady || !isAuthenticated) return;
    let cancelled = false;
    agentApi
      .listQuotas("agent", 1, 0)
      .then(() => !cancelled && setAuthorized(true))
      .catch(() => !cancelled && setAuthorized(false));
    return () => {
      cancelled = true;
    };
  }, [isReady, isAuthenticated]);

  if (!isReady || !isAuthenticated || authorized === null) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin" />
      </div>
    );
  }

  if (!authorized) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <div className="max-w-sm text-center">
          <ShieldAlert className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
          <h1 className="text-base font-semibold">Platform admin access required</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            This console is limited to platform administrators.
          </p>
          <Button className="mt-4" variant="outline" onClick={() => router.push("/agents")}>
            Back to agents
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-10 flex h-14 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur sm:px-6">
        <Button variant="ghost" size="icon" onClick={() => router.push("/agents")}>
          <ArrowLeft />
        </Button>
        <Settings2 className="h-5 w-5 text-primary" />
        <h1 className="text-base font-semibold">Operator console</h1>
        <nav className="ml-4 hidden gap-1 sm:flex">
          {TABS.map((item) => (
            <button
              key={item.key}
              className={cn(
                "rounded-md px-3 py-1.5 text-xs",
                tab === item.key ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground"
              )}
              onClick={() => setTab(item.key)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6">
        <div className="mb-4 flex gap-1 sm:hidden">
          {TABS.map((item) => (
            <button
              key={item.key}
              className={cn(
                "rounded-md px-2.5 py-1 text-xs",
                tab === item.key ? "bg-primary text-primary-foreground" : "text-muted-foreground"
              )}
              onClick={() => setTab(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>
        {tab === "platform" && <AgentPlatformPanel catalog={emptyCatalog} />}
        {tab === "quotas" && <AdminQuotas />}
        {tab === "audit" && <AdminAudit />}
      </main>
    </div>
  );
}
