"use client";

import { useChatStore } from "@/stores/chatStore";
import { useAuthStore } from "@/stores/authStore";
import { useEffect } from "react";

export function AuthInitializer() {
  useEffect(() => {
    useAuthStore.getState().restoreFromStorage();
  }, []);

  useEffect(() => {
    return () => {
      useChatStore.getState().reset();
    };
  }, []);

  return null;
}
