import { redirect } from "next/navigation";

// Legacy route: the Operator Console is now served from /admin.
export default function AdminAgentsRedirect() {
  redirect("/admin");
}
