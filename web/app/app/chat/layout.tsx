"use client";

import { useRealtime } from "@/hooks/useRealtime";

export default function ChatLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  useRealtime();
  return <>{children}</>;
}
