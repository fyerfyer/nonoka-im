"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Send, Smile, Paperclip, Image as ImageIcon, File as FileIcon } from "lucide-react";
import { cn } from "@/lib/utils";

const EMOJIS = [
  "😀", "😄", "😂", "🤣", "😊", "😍", "😘", "🤔",
  "😅", "😭", "😢", "😡", "👍", "👎", "👏", "🙏",
  "❤️", "😜", "🔥", "🎉", "✨", "💯", "🤝", "👀",
];

interface MessageInputProps {
  onSend: (text: string) => void;
  onSendFile?: (file: File) => void;
  disabled?: boolean;
}

export function MessageInput({ onSend, onSendFile, disabled }: MessageInputProps) {
  const [text, setText] = useState("");
  const [emojiOpen, setEmojiOpen] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleSubmit = () => {
    if (!text.trim() || disabled) return;
    onSend(text);
    setText("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
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
