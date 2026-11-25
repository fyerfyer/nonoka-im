"use client";

import type { RunCitation } from "@/lib/api/agents";
import { cn } from "@/lib/utils";

interface SourceChipsProps {
  citations: RunCitation[];
  onJump?: (citation: RunCitation) => void;
  className?: string;
}

/**
 * Server-controlled sources consulted by an agent answer. Citations are
 * references, not model-generated text: the backend only stores message IDs
 * returned by the authorized search tool.
 */
export function SourceChips({ citations, onJump, className }: SourceChipsProps) {
  if (citations.length === 0) return null;
  return (
    <div className={cn("mt-2 flex flex-wrap items-center gap-1", className)}>
      <span className="text-[10px] font-medium text-muted-foreground">Sources</span>
      {citations.map((citation, index) => {
        const label = citation.snippet?.trim() || `message ${citation.msgId}`;
        return (
          <button
            key={`${citation.topic}:${citation.msgId}`}
            type="button"
            title={label}
            onClick={() => onJump?.(citation)}
            className="max-w-56 truncate rounded-full border border-border/70 bg-background/60 px-2 py-0.5 text-[10px] text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
          >
            [{index + 1}] {label}
          </button>
        );
      })}
    </div>
  );
}
