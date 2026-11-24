"use client";

import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Bot, Check, Loader2, RotateCcw, Send, Square, X } from "lucide-react";
import { agentApi } from "@/lib/api/agents";
import { useRunStreamStore } from "@/stores/runStreamStore";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface PlaygroundMessage {
  role: "user" | "assistant";
  content: string;
}

export function AgentPlayground({ agentId }: { agentId: number }) {
  const [messages, setMessages] = useState<PlaygroundMessage[]>([]);
  const [input, setInput] = useState("");
  const [runId, setRunId] = useState<number | null>(null);
  const [starting, setStarting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const completedRunRef = useRef<number | null>(null);
  const live = useRunStreamStore((state) =>
    runId ? state.runs.get(runId) : undefined
  );
  const setRunOutput = useRunStreamStore((state) => state.setRunOutput);
  const clearTestRun = useRunStreamStore((state) => state.clearTestRun);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, live?.text, live?.tools.length, live?.status]);

  useEffect(() => {
    if (!runId || !live || !["completed", "failed", "cancelled"].includes(live.status)) {
      return;
    }
    if (completedRunRef.current === runId) return;
    completedRunRef.current = runId;
    let active = true;
    agentApi
      .runDetail(runId)
      .then((run) => {
        if (!active) return;
        setRunOutput(run);
        const output = run.outputText || live.text;
        if (output) {
          setMessages((current) => [...current, { role: "assistant", content: output }]);
        }
      })
      .catch(() => {
        if (live.text) setMessages((current) => [...current, { role: "assistant", content: live.text }]);
      });
    return () => {
      active = false;
    };
  }, [runId, live, setRunOutput]);

  const send = async () => {
    const text = input.trim();
    if (!text || starting || (live && !["completed", "failed", "cancelled"].includes(live.status))) {
      return;
    }
    setInput("");
    completedRunRef.current = null;
    setMessages((current) => [...current, { role: "user", content: text }]);
    setStarting(true);
    try {
      if (runId) clearTestRun(runId);
      const run = await agentApi.test(agentId, text);
      setRunId(run.id);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Test run failed");
    } finally {
      setStarting(false);
    }
  };

  const cancel = async () => {
    if (!runId) return;
    setCancelling(true);
    try {
      await agentApi.cancelRun(runId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setCancelling(false);
    }
  };

  const reset = () => {
    if (runId) clearTestRun(runId);
    setRunId(null);
    setMessages([]);
    setInput("");
    completedRunRef.current = null;
  };

  const terminal = !!live && ["completed", "failed", "cancelled"].includes(live.status);
  const active = !!live && !terminal;
  const approvalPending = live?.approvalId && live.approvalStatus === "pending";

  return (
    <div className="flex h-[calc(100vh-13rem)] min-h-[420px] flex-col rounded-xl border bg-card">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold">
            <Bot className="h-4 w-4 text-primary" />
            Draft playground
          </h3>
          <p className="text-xs text-muted-foreground">
            Uses the current draft snapshot. Nothing is published or written to chat.
          </p>
        </div>
        <Button variant="ghost" size="sm" onClick={reset}>
          <RotateCcw />
          Reset
        </Button>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        {messages.length === 0 && !live && (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-sm text-muted-foreground">
            <Bot className="h-9 w-9 text-primary/50" />
            <p>Send a message to test the draft before publishing.</p>
          </div>
        )}
        {messages.map((message, index) => (
          <div
            key={`${message.role}-${index}`}
            className={cn(
              "flex",
              message.role === "user" ? "justify-end" : "justify-start"
            )}
          >
            <div
              className={cn(
                "max-w-[85%] whitespace-pre-wrap rounded-2xl px-3.5 py-2 text-sm",
                message.role === "user"
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted"
              )}
            >
              {message.content}
            </div>
          </div>
        ))}
        {live && !terminal && (
          <div className="flex justify-start">
            <div className="max-w-[85%] rounded-2xl bg-muted px-3.5 py-2 text-sm">
              {live.text || (
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  Thinking…
                </span>
              )}
              {live.tools.length > 0 && (
                <div className="mt-2 space-y-1 border-t pt-2 text-xs text-muted-foreground">
                  {live.tools.map((tool, index) => (
                    <div key={index} className="flex items-center gap-1.5">
                      {tool.status === "running" ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : tool.status === "error" ? (
                        <X className="h-3 w-3 text-destructive" />
                      ) : (
                        <Check className="h-3 w-3 text-emerald-600" />
                      )}
                      <span>{tool.summary || tool.tool}</span>
                    </div>
                  ))}
                </div>
              )}
              {approvalPending && (
                <div className="mt-3 space-y-2 rounded-lg border border-amber-300/60 bg-amber-50/60 p-2.5 dark:border-amber-500/30 dark:bg-amber-500/10">
                  <p className="text-xs font-medium">
                    Approval required: {live.tools.at(-1)?.tool || "tool"}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {live.tools.at(-1)?.summary}
                  </p>
                  <div className="flex gap-2">
                    <Button
                      size="xs"
                      onClick={async () => {
                        try {
                          await agentApi.approveApproval(live.approvalId!);
                        } catch (err) {
                          toast.error(err instanceof Error ? err.message : "Approval failed");
                        }
                      }}
                    >
                      <Check />
                      Approve
                    </Button>
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={async () => {
                        try {
                          await agentApi.rejectApproval(live.approvalId!);
                        } catch (err) {
                          toast.error(err instanceof Error ? err.message : "Rejection failed");
                        }
                      }}
                    >
                      <X />
                      Reject
                    </Button>
                  </div>
                </div>
              )}
              {live.status === "failed" && (
                <p className="mt-2 text-xs text-destructive">
                  {live.errorCode || "Run failed"}
                </p>
              )}
              {live.status === "cancelled" && (
                <p className="mt-2 text-xs text-muted-foreground">Cancelled.</p>
              )}
              {(live.inputTokens || live.outputTokens) && (
                <p className="mt-2 text-[10px] text-muted-foreground">
                  {live.inputTokens ?? 0} in · {live.outputTokens ?? 0} out
                </p>
              )}
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      <div className="border-t p-3">
        <div className="flex items-end gap-2">
          <textarea
            className="max-h-32 min-h-10 flex-1 resize-none rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder="Ask the draft agent…"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                void send();
              }
            }}
          />
          {active ? (
            <Button variant="outline" size="icon" onClick={cancel} disabled={cancelling} title="Stop">
              {cancelling ? <Loader2 className="animate-spin" /> : <Square />}
            </Button>
          ) : (
            <Button size="icon" onClick={send} disabled={starting || !input.trim()} title="Send">
              {starting ? <Loader2 className="animate-spin" /> : <Send />}
            </Button>
          )}
        </div>
        <div className="mt-2 flex items-center gap-2 text-[10px] text-muted-foreground">
          <Badge variant="secondary">draft</Badge>
          <span>Enter sends · Shift+Enter adds a line</span>
        </div>
      </div>
    </div>
  );
}
