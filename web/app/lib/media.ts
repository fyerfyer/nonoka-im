const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://127.0.0.1:8000";

export interface MediaMeta {
  url: string;
  name: string;
  size: number;
  mime: string;
}

// parseMediaContent decodes the JSON metadata stored in IMAGE/FILE message
// content. Returns null for text or malformed payloads.
export function parseMediaContent(content: string): MediaMeta | null {
  try {
    const obj = JSON.parse(content) as Partial<MediaMeta>;
    if (typeof obj.url !== "string" || obj.url === "") return null;
    return {
      url: obj.url,
      name: typeof obj.name === "string" ? obj.name : "file",
      size: typeof obj.size === "number" ? obj.size : 0,
      mime: typeof obj.mime === "string" ? obj.mime : "",
    };
  } catch {
    return null;
  }
}

// absoluteFileUrl resolves stored "/v1/files/<id>" paths against the API
// origin so <img> and <a> work from the web app served on another port.
export function absoluteFileUrl(url: string): string {
  if (/^https?:\/\//.test(url)) return url;
  return `${API_BASE_URL.replace(/\/$/, "")}${url.startsWith("/") ? url : `/${url}`}`;
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
