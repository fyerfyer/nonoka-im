"use client";

import { useState, useRef, useEffect, useMemo } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Send, Smile, Paperclip, Image as ImageIcon, File as FileIcon } from "lucide-react";
import { cn } from "@/lib/utils";
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

interface MessageInputProps {
  onSend: (text: string) => void;
  onSendFile?: (file: File) => void;
  members?: MentionMember[];
  disabled?: boolean;
}

export function MessageInput({ onSend, onSendFile, members, disabled }: MessageInputProps) {
  const [text, setText] = useState("");
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [mentionIndex, setMentionIndex] = useState(0);
  const [mentionDismissed, setMentionDismissed] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Active "@query" completion state (group mentions only).
  const mentionState = useMemo(() => {
    if (!members || members.length === 0 || mentionDismissed) return null;
    const el = textareaRef.current;
    const caret = el?.selectionStart ?? text.length;
    const active = activeMentionQuery(text, caret);
    if (!active) return null;
    const candidates = filterMembers(members, active.query);
    if (candidates.length === 0) return null;
    return { ...active, candidates };
  }, [text, members, mentionDismissed]);

  const applyMention = (username: string) => {
    if (!mentionState) return;
    const el = textareaRef.current;
    const caret = el?.selectionStart ?? text.length;
    const next =
      text.slice(0, mentionState.start) +
      `@${username} ` +
      text.slice(caret);
    setMentionDismissed(false);
    setText(next);
    setMentionIndex(0);
    requestAnimationFrame(() => {
      el?.focus();
      const pos = mentionState.start + username.length + 2;
      el?.setSelectionRange(pos, pos);
    });
  };

  const handleSubmit = () => {
    if (!text.trim() || disabled) return;
    onSend(text);
    setText("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
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
      handleSubmit();
    }
  };

  const insertEmoji = (emoji: string) => {
    const el = textareaRef.current;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? text.length;
    const next = text.slice(0, start) + emoji + text.slice(end);
    setText(next);
    requestAnimationFrame(() => {
      el?.focus();
      const pos = start + emoji.length;
      el?.setSelectionRange(pos, pos);
    });
  };

  const pickFile = (input: HTMLInputElement | null) => {
    if (!input) return;
    input.value = "";
    input.click();
  };

  const handleFileChange = (
    e: React.ChangeEvent<HTMLInputElement>
  ) => {
    const file = e.target.files?.[0];
    if (file && onSendFile) {
      onSendFile(file);
    }
  };

  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
  }, [text]);

  return (
    <div className="border-t bg-background p-3">
      {/* Hidden pickers for the attachment menu. */}
      <input
        ref={imageInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFileChange}
      />
      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        onChange={handleFileChange}
      />
      <div className="relative flex items-center gap-2 rounded-2xl border bg-background px-3 py-1.5 shadow-sm">
        <button
          type="button"
          className={cn(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-colors",
            emojiOpen
              ? "text-foreground bg-muted"
              : "text-muted-foreground hover:text-foreground"
          )}
          disabled={disabled}
          title="Emoji"
          onClick={() => setEmojiOpen((v) => !v)}
        >
          <Smile className="h-5 w-5" />
        </button>
        {emojiOpen && (
          <>
            <div
              className="fixed inset-0 z-40"
              onClick={() => setEmojiOpen(false)}
            />
            <div className="absolute bottom-full left-0 z-50 mb-2 w-64 rounded-xl border bg-popover p-2 shadow-lg">
              <div className="grid grid-cols-8 gap-0.5">
                {EMOJIS.map((emoji) => (
                  <button
                    key={emoji}
                    type="button"
                    className="flex h-7 w-7 items-center justify-center rounded text-lg hover:bg-muted"
                    onClick={() => insertEmoji(emoji)}
                  >
                    {emoji}
                  </button>
                ))}
              </div>
            </div>
          </>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
              disabled={disabled}
              title="Attach"
            >
              <Paperclip className="h-5 w-5" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="top">
            <DropdownMenuItem onSelect={() => pickFile(imageInputRef.current)}>
              <ImageIcon className="h-4 w-4" />
              Image
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => pickFile(fileInputRef.current)}>
              <FileIcon className="h-4 w-4" />
              File
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {mentionState && (
          <div className="absolute bottom-full left-10 z-50 mb-2 w-52 rounded-xl border bg-popover p-1 shadow-lg">
            <div className="px-2 py-1 text-[10px] text-muted-foreground">
              Mention a member
            </div>
            {mentionState.candidates.map((m, i) => (
              <button
                key={m.userId}
                type="button"
                className={cn(
                  "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm",
                  i === Math.min(mentionIndex, mentionState.candidates.length - 1)
                    ? "bg-muted"
                    : "hover:bg-muted"
                )}
                onMouseDown={(e) => {
                  e.preventDefault(); // keep textarea focus
                  applyMention(m.username);
                }}
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[10px] font-semibold text-primary">
                  {m.username.slice(0, 2).toUpperCase()}
                </span>
                <span className="truncate">{m.username}</span>
              </button>
            ))}
          </div>
        )}
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => {
            setMentionDismissed(false);
            setText(e.target.value);
          }}
          onKeyDown={handleKeyDown}
          placeholder="Type a message..."
          disabled={disabled}
          rows={1}
          className="max-h-[120px] min-h-[36px] w-full resize-none bg-transparent py-1.5 text-sm outline-none placeholder:text-muted-foreground/70"
        />
        <Button
          size="icon"
          className={cn(
            "h-9 w-9 shrink-0 rounded-full transition-opacity",
            !text.trim() && "opacity-50"
          )}
          onClick={handleSubmit}
          disabled={!text.trim() || disabled}
        >
          <Send className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
