// Mention helpers for group @mentions. Plain-text scheme: "@username" tokens
// bounded by whitespace/start/end count as mentions when the username is a
// known group member.

export interface MentionMember {
  userId: number;
  username: string;
}

// activeMentionQuery inspects the text before the caret and returns the
// partial "@query" currently being typed (must start at a word boundary),
// or null when no @-completion is active.
export function activeMentionQuery(
  text: string,
  caret: number
): { query: string; start: number } | null {
  const before = text.slice(0, caret);
  const match = /(?:^|\s)@([^\s@]*)$/.exec(before);
  if (!match) return null;
  return { query: match[1], start: caret - match[1].length - 1 };
}

// filterMembers returns members whose username starts with the query.
export function filterMembers(
  members: MentionMember[],
  query: string
): MentionMember[] {
  const q = query.toLowerCase();
  return members
    .filter((m) => m.username.toLowerCase().startsWith(q))
    .slice(0, 6);
}

// extractMentionIds finds valid "@username" tokens in the final text and
// returns the matching member ids (deduplicated).
export function extractMentionIds(
  text: string,
  members: MentionMember[]
): number[] {
  if (members.length === 0) return [];
  const byName = new Map(members.map((m) => [m.username.toLowerCase(), m.userId]));
  const ids = new Set<number>();
  const re = /(?:^|\s)@([^\s@]+)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    const id = byName.get(m[1].toLowerCase());
    if (id !== undefined) ids.add(id);
  }
  return Array.from(ids);
}

export interface TextSegment {
  text: string;
  mentionedUserId?: number;
}

// segmentMentions splits text into plain and mention segments for rendering.
// A segment is a mention only when the username is a known group member.
export function segmentMentions(
  text: string,
  members: MentionMember[]
): TextSegment[] {
  if (members.length === 0) return [{ text }];
  const byName = new Map(
    members.map((m) => [m.username.toLowerCase(), m.userId])
  );
  const segments: TextSegment[] = [];
  const re = /@([^\s@]+)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    // Only treat as a mention at a word boundary.
    if (m.index > 0 && !/\s/.test(text[m.index - 1])) continue;
    const id = byName.get(m[1].toLowerCase());
    if (id === undefined) continue;
    if (m.index > last) segments.push({ text: text.slice(last, m.index) });
    segments.push({ text: m[0], mentionedUserId: id });
    last = m.index + m[0].length;
  }
  if (last < text.length) segments.push({ text: text.slice(last) });
  return segments.length > 0 ? segments : [{ text }];
}
