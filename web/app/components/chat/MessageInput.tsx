"use client";

import { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Send,
  Smile,
  Paperclip,
  Image as ImageIcon,
  File as FileIcon,
  X,
  RotateCcw,
  Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useAgentNames } from "@/hooks/useAgentNames";
import {
  activeMentionQuery,
  filterMembers,
  MentionMember,
} from "@/lib/mentions";

const EMOJIS = [
  "😀", "😄", "😂", "🤣", "😊", "😍", "😘", "🤔",
  "😅", "😭", "😢", "😡", "👍", "👎", "👏", "🙏",
  "❤️", "😜", "🔥", "🎉", "✨", "💯", "🤝", "👀",
];

const MAX_FILE_BYTES = 10 << 20;

interface Attachment {
  id: string;
  file: File;
  preview?: string;
  status: "pending" | "uploading" | "sent" | "failed";
  progress: number;
  error?: string;
}

interface MessageInputProps {
  onSend: (text: string) => void;
  onSendFile?: (file: File, onProgress?: (percent: number) => void) => Promise<boolean>;
  members?: MentionMember[];
  disabled?: boolean;
}

export function MessageInput({ onSend, onSendFile, members, disabled }: MessageInputProps) {
  const [text, setText] = useState("");
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [mentionDismissed, setMentionDismissed] = useState(false);
  const [caret, setCaret] = useState(0);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [dragging, setDragging] = useState(false);
  const [sending, setSending] = useState(false);
  const sendingRef = useRef(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const agentNames = useAgentNames();

  const mentionState = useMemo(() => {
    if (!members || members.length === 0 || mentionDismissed) return null;
    const active = activeMentionQuery(text, caret || text.length);
    if (!active) return null;
    const candidates = filterMembers(members, active.query);
    if (candidates.length === 0) return null;
    return { ...active, candidates };
  }, [text, caret, members, mentionDismissed]);

  const addFiles = useCallback((files: FileList | File[]) => {
    const next: Attachment[] = [];
    Array.from(files).forEach((file) => {
      if (file.size > MAX_FILE_BYTES) {
        next.push({
          id: `${Date.now()}-${file.name}-${Math.random()}`,
          file,
          status: "failed",
          progress: 0,
          error: "File exceeds the 10MB limit",
        });
        return;
      }
      next.push({
        id: `${Date.now()}-${file.name}-${Math.random()}`,
        file,
        preview: file.type.startsWith("image/") ? URL.createObjectURL(file) : undefined,
        status: "pending",
        progress: 0,
      });
    });
    if (next.length > 0) setAttachments((current) => [...current, ...next]);
  }, []);

  const removeAttachment = (id: string) => {
    setAttachments((current) => {
      const target = current.find((a) => a.id === id);
      if (target?.preview) URL.revokeObjectURL(target.preview);
      return current.filter((a) => a.id !== id);
    });
  };

  const retryAttachment = (id: string) => {
    setAttachments((current) =>
      current.map((a) =>
        a.id === id ? { ...a, status: "pending", progress: 0, error: undefined } : a
      )
    );
  };

  const uploadPending = useCallback(async () => {
    if (!onSendFile) return false;
    let allOk = true;
    for (const attachment of attachments) {
      if (attachment.status !== "pending" && attachment.status !== "failed") continue;
      if (attachment.error === "File exceeds the 10MB limit") {
        allOk = false;
        continue;
      }
      setAttachments((current) =>
        current.map((a) => (a.id === attachment.id ? { ...a, status: "uploading", progress: 0 } : a))
      );
      const ok = await onSendFile(attachment.file, (percent) => {
        setAttachments((current) =>
          current.map((a) => (a.id === attachment.id ? { ...a, progress: percent } : a))
        );
      });
      setAttachments((current) =>
        current.map((a) =>
          a.id === attachment.id
            ? { ...a, status: ok ? "sent" : "failed", progress: ok ? 100 : a.progress, error: ok ? undefined : "Upload failed" }
            : a
        )
      );
      if (!ok) allOk = false;
    }
    return allOk;
  }, [attachments, onSendFile]);

  const handleSubmit = useCallback(async () => {
    if (disabled || sendingRef.current) return;
    const hasText = text.trim().length > 0;
    const pending = attachments.filter((a) => a.status === "pending" || a.status === "failed" || a.status === "uploading");
    if (!hasText && pending.length === 0) return;
    sendingRef.current = true;
    setSending(true);
    try {
      if (hasText) {
        onSend(text.trim());
        setText("");
        setCaret(0);
        if (textareaRef.current) textareaRef.current.style.height = "auto";
      }
      if (pending.length > 0) {
        await uploadPending();
        clearSentAttachments();
      }
    } finally {
      sendingRef.current = false;
      setSending(false);
    }
  }, [disabled, text, attachments, onSend, uploadPending]);

  const applyMention = (username: string) => {
    if (!mentionState) return;
    const el = textareaRef.current;
    const caretPos = el?.selectionStart ?? text.length;
    const next =
      text.slice(0, mentionState.start) +
      `@${username} ` +
      text.slice(caretPos);
    const pos = mentionState.start + username.length + 2;
    setMentionDismissed(false);
    setText(next);
    setCaret(pos);
    setMentionIndex(0);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(pos, pos);
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (mentionState) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setMentionIndex((i) => (i + 1) % mentionState.candidates.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setMentionIndex(
          (i) =>
            (i - 1 + mentionState.candidates.length) %
            mentionState.candidates.length
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        applyMention(mentionState.candidates[mentionIndex].username);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionDismissed(true);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void handleSubmit();
    }
  };

  const insertEmoji = (emoji: string) => {
    const el = textareaRef.current;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? text.length;
    const next = text.slice(0, start) + emoji + text.slice(end);
    const pos = start + emoji.length;
    setText(next);
    setCaret(pos);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(pos, pos);
    });
  };

  const pickFile = (input: HTMLInputElement | null) => {
    if (!input) return;
    input.value = "";
    input.click();
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      addFiles(e.target.files);
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
    const files = e.clipboardData?.files;
    if (files && files.length > 0) {
      e.preventDefault();
      addFiles(files);
    }
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      addFiles(e.dataTransfer.files);
    }
  };

  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
  }, [text]);

  const attachmentsRef = useRef<Attachment[]>([]);
  useEffect(() => {
    attachmentsRef.current = attachments;
  }, [attachments]);

  useEffect(() => {
    return () => {
      attachmentsRef.current.forEach((a) => {
        if (a.preview) URL.revokeObjectURL(a.preview);
      });
    };
  }, []);

  const clearSentAttachments = () => {
    setAttachments((current) => {
      const remaining = current.filter((a) => a.status !== "sent");
      current.forEach((a) => {
        if (a.status === "sent" && a.preview) URL.revokeObjectURL(a.preview);
      });
      return remaining;
    });
  };

  const hasPending = attachments.some(
    (a) => a.status === "pending" || a.status === "uploading" || a.status === "failed"
  );

  return (
    <div
      className="relative border-t bg-background p-3"
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={handleDrop}
    >
      {dragging && (
        <div className="pointer-events-none absolute inset-2 z-20 flex items-center justify-center rounded-xl border-2 border-dashed border-primary bg-primary/5 text-sm font-medium text-primary">
          Drop files to attach
        </div>
      )}
      <input
        ref={imageInputRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        onChange={handleFileChange}
      />
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="hidden"
        onChange={handleFileChange}
      />
      {attachments.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2">
          {attachments.map((attachment) => (
            <div key={attachment.id} className="relative flex w-44 items-center gap-2 rounded-xl border bg-card p-2 text-xs">
              {attachment.preview ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={attachment.preview} alt={attachment.file.name} className="h-9 w-9 rounded-lg object-cover" />
              ) : (
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-muted">
                  <FileIcon className="h-4 w-4" />
                </span>
              )}
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{attachment.file.name}</span>
                <span className="block text-[10px] text-muted-foreground">
                  {attachment.status === "uploading"
                    ? `${attachment.progress}%`
                    : attachment.status === "sent"
                      ? "Sent"
                      : attachment.status === "failed"
                        ? attachment.error || "Failed"
                        : "Ready"}
                </span>
                {attachment.status === "uploading" && (
                  <span className="mt-1 block h-1 overflow-hidden rounded-full bg-muted">
                    <span className="block h-full bg-primary transition-all" style={{ width: `${attachment.progress}%` }} />
                  </span>
                )}
              </span>
              {attachment.status === "failed" ? (
                <button type="button" className="text-muted-foreground hover:text-foreground" onClick={() => retryAttachment(attachment.id)} title="Retry">
                  <RotateCcw className="h-3.5 w-3.5" />
                </button>
              ) : (
                <button type="button" className="text-muted-foreground hover:text-foreground" onClick={() => removeAttachment(attachment.id)} title="Remove">
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      <div className="relative flex items-center gap-2 rounded-2xl border bg-background px-3 py-1.5 shadow-sm">
        <button
          type="button"
          className={cn(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-colors",
            emojiOpen ? "text-foreground bg-muted" : "text-muted-foreground hover:text-foreground"
          )}
          disabled={disabled}
          title="Emoji"
          onClick={() => setEmojiOpen((v) => !v)}
        >
          <Smile className="h-5 w-5" />
        </button>
        {emojiOpen && (
          <>
            <div className="fixed inset-0 z-40" onClick={() => setEmojiOpen(false)} />
            <div className="absolute bottom-full left-0 z-50 mb-2 w-64 rounded-xl border bg-popover p-2 shadow-lg">
              <div className="grid grid-cols-8 gap-0.5">
                {EMOJIS.map((emoji) => (
                  <button key={emoji} type="button" className="flex h-7 w-7 items-center justify-center rounded text-lg hover:bg-muted" onClick={() => insertEmoji(emoji)}>
                    {emoji}
                  </button>
                ))}
              </div>
            </div>
          </>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground" disabled={disabled} title="Attach">
              <Paperclip className="h-5 w-5" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="top">
            <DropdownMenuItem onSelect={() => pickFile(imageInputRef.current)}>
              <ImageIcon className="h-4 w-4" /> Image
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => pickFile(fileInputRef.current)}>
              <FileIcon className="h-4 w-4" /> File
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {mentionState && (
          <div className="absolute bottom-full left-10 z-50 mb-2 w-52 rounded-xl border bg-popover p-1 shadow-lg">
            <div className="px-2 py-1 text-[10px] text-muted-foreground">Mention a member</div>
            {mentionState.candidates.map((m, i) => {
              const agentName = agentNames.get(m.userId)?.name;
              return (
                <button
                  key={m.userId}
                  type="button"
                  className={cn(
                    "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm",
                    i === Math.min(mentionIndex, mentionState.candidates.length - 1) ? "bg-muted" : "hover:bg-muted"
                  )}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    applyMention(m.username);
                  }}
                >
                  <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[10px] font-semibold text-primary">
                    {(agentName ?? m.username).slice(0, 2).toUpperCase()}
                  </span>
                  <span className="truncate">{agentName ?? m.username}</span>
                  {agentName && (
                    <span className="ml-auto rounded bg-primary/10 px-1 text-[9px] font-medium text-primary">AI</span>
                  )}
                </button>
              );
            })}
          </div>
        )}
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => {
            setMentionDismissed(false);
            setCaret(e.target.selectionStart);
            setText(e.target.value);
          }}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder="Type a message..."
          disabled={disabled}
          rows={1}
          className="max-h-[120px] min-h-[36px] w-full resize-none bg-transparent py-1.5 text-sm outline-none placeholder:text-muted-foreground/70"
        />
        <Button
          size="icon"
          aria-label="Send message"
          title="Send message"
          className={cn("h-9 w-9 shrink-0 rounded-full transition-opacity", !text.trim() && !hasPending && "opacity-50")}
          onClick={() => void handleSubmit()}
          disabled={disabled || sending || (!text.trim() && !hasPending)}
        >
          {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
