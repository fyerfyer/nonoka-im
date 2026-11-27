import type { ComponentType, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function PageHeader({
  title,
  description,
  icon: Icon,
  actions,
  className,
}: {
  title: string;
  description?: string;
  icon?: ComponentType<{ className?: string }>;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "flex flex-col gap-3 border-b bg-background/95 px-4 py-4 backdrop-blur sm:flex-row sm:items-center sm:justify-between sm:px-6",
        className
      )}
    >
      <div className="flex min-w-0 items-center gap-3">
        {Icon && (
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary/10">
            <Icon className="h-4 w-4 text-primary" />
          </div>
        )}
        <div className="min-w-0">
          <h1 className="truncate text-base font-semibold">{title}</h1>
          {description && (
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {description}
            </p>
          )}
        </div>
      </div>
      {actions && (
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      )}
    </header>
  );
}
