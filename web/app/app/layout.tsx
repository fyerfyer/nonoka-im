import type { Metadata } from "next";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { Providers } from "./providers";
import "./globals.css";

export const metadata: Metadata = {
  title: "Nonoka IM",
  description: "Agent-native IM infrastructure reference workspace",
};

const sidebarWidthScript = `try {
  var stored = Number(localStorage.getItem("nonoka.web.chatSidebarWidth"));
  if (stored >= 280 && stored <= 420) {
    document.documentElement.style.setProperty("--chat-sidebar-width", stored + "px");
  }
} catch (error) {}`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="h-full font-sans antialiased">
        <script dangerouslySetInnerHTML={{ __html: sidebarWidthScript }} />
        <Providers>
          <TooltipProvider>{children}</TooltipProvider>
          <Toaster position="top-center" />
        </Providers>
      </body>
    </html>
  );
}
