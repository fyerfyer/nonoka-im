"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";

export const CHAT_SIDEBAR_MIN = 280;
export const CHAT_SIDEBAR_MAX = 420;
export const CHAT_SIDEBAR_DEFAULT = 320;
export const CHAT_SIDEBAR_STORAGE_KEY = "nonoka.web.chatSidebarWidth";
const KEYBOARD_STEP = 16;
const KEYBOARD_LARGE_STEP = 48;

function clampWidth(value: number): number {
  if (!Number.isFinite(value)) return CHAT_SIDEBAR_DEFAULT;
  return Math.min(CHAT_SIDEBAR_MAX, Math.max(CHAT_SIDEBAR_MIN, Math.round(value)));
}

function readStoredWidth(): number {
  if (typeof window === "undefined") return CHAT_SIDEBAR_DEFAULT;
  try {
    const stored = Number(localStorage.getItem(CHAT_SIDEBAR_STORAGE_KEY));
    return stored >= CHAT_SIDEBAR_MIN && stored <= CHAT_SIDEBAR_MAX
      ? Math.round(stored)
      : CHAT_SIDEBAR_DEFAULT;
  } catch {
    return CHAT_SIDEBAR_DEFAULT;
  }
}

function writeStoredWidth(width: number) {
  try {
    localStorage.setItem(CHAT_SIDEBAR_STORAGE_KEY, String(width));
  } catch {
    // Storage can be unavailable in private modes; resizing still works.
  }
}

export function useResizableSidebar() {
  const [width, setWidthState] = useState(CHAT_SIDEBAR_DEFAULT);
  const [dragging, setDragging] = useState(false);
  const widthRef = useRef(CHAT_SIDEBAR_DEFAULT);
  const dragRef = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(
    null
  );

  const applyDomWidth = useCallback((next: number) => {
    widthRef.current = next;
    document.documentElement.style.setProperty(
      "--chat-sidebar-width",
      `${next}px`
    );
  }, []);

  const commitWidth = useCallback(
    (next: number, persist = true) => {
      const clamped = clampWidth(next);
      applyDomWidth(clamped);
      setWidthState(clamped);
      if (persist) writeStoredWidth(clamped);
    },
    [applyDomWidth]
  );

  useEffect(() => {
    const stored = readStoredWidth();
    applyDomWidth(stored);
    // eslint-disable-next-line react-hooks/set-state-in-effect -- restore persisted width after hydration
    setWidthState(stored);
  }, [applyDomWidth]);

  const handlePointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (event.button !== 0) return;
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      dragRef.current = {
        pointerId: event.pointerId,
        startX: event.clientX,
        startWidth: widthRef.current,
      };
      setDragging(true);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    []
  );

  const handlePointerMove = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) return;
      event.preventDefault();
      applyDomWidth(
        clampWidth(drag.startWidth + (event.clientX - drag.startX))
      );
    },
    [applyDomWidth]
  );

  const endDrag = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) return;
      dragRef.current = null;
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
      setDragging(false);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      commitWidth(widthRef.current);
    },
    [commitWidth]
  );

  const handleKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      let next = widthRef.current;
      switch (event.key) {
        case "ArrowLeft":
          next -= event.shiftKey ? KEYBOARD_LARGE_STEP : KEYBOARD_STEP;
          break;
        case "ArrowRight":
          next += event.shiftKey ? KEYBOARD_LARGE_STEP : KEYBOARD_STEP;
          break;
        case "Home":
          next = CHAT_SIDEBAR_MIN;
          break;
        case "End":
          next = CHAT_SIDEBAR_MAX;
          break;
        case "Enter":
        case " ":
          next = CHAT_SIDEBAR_DEFAULT;
          break;
        default:
          return;
      }
      event.preventDefault();
      commitWidth(next);
    },
    [commitWidth]
  );

  const reset = useCallback(() => {
    commitWidth(CHAT_SIDEBAR_DEFAULT);
  }, [commitWidth]);

  return {
    width,
    dragging,
    handlePointerDown,
    handlePointerMove,
    handlePointerUp: endDrag,
    handlePointerCancel: endDrag,
    handleKeyDown,
    handleDoubleClick: reset,
  };
}
