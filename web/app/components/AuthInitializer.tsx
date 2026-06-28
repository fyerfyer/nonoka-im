"use client";

import { useChatStore } from "@/stores/chatStore";
import { useAuthStore } from "@/stores/authStore";
import { useEffect, useRef } from "react";

export function AuthInitializer() {
  const initialized = useRef(false);

  if (!initialized.current && typeof window !== "undefined") {
    initialized.current = true;
    useAuthStore.getState().restoreFromStorage();
  }

  useEffect(() => {
    return () => {
      useChatStore.getState().reset();
    };
  }, []);

  return null;
}
