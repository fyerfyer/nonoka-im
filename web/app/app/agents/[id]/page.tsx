import { AgentDetailClient } from "@/components/agents/AgentDetailClient";

export default async function AgentDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ tab?: string }>;
}) {
  const { id } = await params;
  const { tab } = await searchParams;
  return <AgentDetailClient agentId={Number(id)} initialTab={tab} />;
}
