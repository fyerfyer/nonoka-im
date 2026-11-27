"use client";

import { usePathname } from "next/navigation";
import { ConversationList } from "@/components/chat/ConversationList";
import { ChatSidebarLayout } from "@/components/chat/ChatSidebarLayout";

export default function ChatLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();
  const hasTopic = pathname !== "/chat";

  return (
    <ChatSidebarLayout
      list={<ConversationList />}
      mobilePane={hasTopic ? "content" : "list"}
    >
      {children}
    </ChatSidebarLayout>
  );
}
