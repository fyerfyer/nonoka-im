export function getP2PTopic(userId1: number, userId2: number): string {
  const [a, b] = userId1 < userId2 ? [userId1, userId2] : [userId2, userId1];
  return `p2p_${a}_${b}`;
}

export function topicType(topic: string): "p2p" | "group" | "unknown" {
  if (topic.startsWith("p2p_")) return "p2p";
  if (topic.startsWith("grp_")) return "group";
  return "unknown";
}

export function extractPeerIdFromP2PTopic(
  topic: string,
  currentUserId: number
): number {
  const match = topic.match(/^p2p_(\d+)_(\d+)$/);
  if (!match) return 0;
  const id1 = parseInt(match[1], 10);
  const id2 = parseInt(match[2], 10);
  return id1 === currentUserId ? id2 : id1;
}
