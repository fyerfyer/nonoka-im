import {
  Bot,
  Code2,
  MessageSquare,
  Settings2,
  ShieldCheck,
  type LucideIcon,
} from "lucide-react";

export interface WorkspaceNavItem {
  href: string;
  label: string;
  icon: LucideIcon;
  match: (pathname: string) => boolean;
  adminOnly?: boolean;
}

export const WORKSPACE_NAV: WorkspaceNavItem[] = [
  {
    href: "/chat",
    label: "Chat",
    icon: MessageSquare,
    match: (pathname) => pathname.startsWith("/chat"),
  },
  {
    href: "/agents",
    label: "Agents",
    icon: Bot,
    match: (pathname) => pathname.startsWith("/agents"),
  },
  {
    href: "/approvals",
    label: "Approvals",
    icon: ShieldCheck,
    match: (pathname) => pathname.startsWith("/approvals"),
  },
  {
    href: "/developer",
    label: "Developer",
    icon: Code2,
    match: (pathname) => pathname.startsWith("/developer"),
  },
  {
    href: "/admin",
    label: "Admin",
    icon: Settings2,
    match: (pathname) => pathname.startsWith("/admin"),
    adminOnly: true,
  },
];
