"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import {
  CHAT_SIDEBAR_MAX,
  CHAT_SIDEBAR_MIN,
  useResizableSidebar,
} from "@/hooks/useResizableSidebar";

export function ChatSidebarLayout({
  list,
  children,
  mobilePane,
}: {
  list: ReactNode;
  children: ReactNode;
  mobilePane: "list" | "content";
}) {
  const sidebar = useResizableSidebar();

  return (
    <div className="flex h-full min-h-0 w-full overflow-hidden bg-chat-canvas">
      <aside
        id="conversation-list"
        data-testid="conversation-list"
        className={cn(
          "h-full min-h-0 w-full shrink-0 flex-col overflow-hidden border-r border-border bg-chat-list md:flex md:w-[var(--chat-sidebar-width,320px)]",
          mobilePane === "content" ? "hidden md:flex" : "flex"
        )}
      >
        {list}
      </aside>

      <div
        role="separator"
        tabIndex={0}
        aria-label="Resize conversation list"
        aria-orientation="vertical"
        aria-controls="conversation-list"
        aria-valuemin={CHAT_SIDEBAR_MIN}
        aria-valuemax={CHAT_SIDEBAR_MAX}
        aria-valuenow={sidebar.width}
        aria-valuetext={`${sidebar.width} pixels`}
        title="Drag to resize. Double-click to reset."
        data-testid="chat-sidebar-separator"
        onPointerDown={sidebar.handlePointerDown}
        onPointerMove={sidebar.handlePointerMove}
        onPointerUp={sidebar.handlePointerUp}
        onPointerCancel={sidebar.handlePointerCancel}
        onKeyDown={sidebar.handleKeyDown}
        onDoubleClick={sidebar.handleDoubleClick}
        className={cn(
          "group relative hidden w-2 shrink-0 cursor-col-resize touch-none select-none outline-none md:block",
          "focus-visible:bg-primary/10",
          sidebar.dragging && "bg-primary/10"
        )}
      >
        <span
          className={cn(
            "absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-border transition-colors",
            "group-hover:bg-primary/40 group-focus-visible:bg-primary",
            sidebar.dragging && "bg-primary"
          )}
        />
        <span className="absolute left-1/2 top-1/2 flex h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-border opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
      </div>

      <section
        className={cn(
          "min-h-0 min-w-0 flex-1 flex-col overflow-hidden",
          mobilePane === "content" ? "flex" : "hidden md:flex"
        )}
      >
        {children}
      </section>
    </div>
  );
}
