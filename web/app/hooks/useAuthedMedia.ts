"use client";

import { useCallback, useEffect, useState } from "react";
import { API_BASE_URL, fileApi } from "@/lib/api";

function absolute(url: string): string {
  if (/^https?:\/\//.test(url)) return url;
  return `${API_BASE_URL.replace(/\/$/, "")}${url.startsWith("/") ? url : `/${url}`}`;
}

interface MediaState {
  key: string;
  src: string | null;
  loading: boolean;
  error: string | null;
}

// useAuthedMedia resolves a downloadable attachment URL for media rendering.
// It prefers a short-TTL signed URL (no Authorization header needed) and falls
// back to a bearer-authenticated blob URL. Object URLs are revoked on cleanup.
export function useAuthedMedia(fileId: string | null, url: string | null) {
  const [nonce, setNonce] = useState(0);
  const key = `${fileId ?? ""}|${url ?? ""}|${nonce}`;
  const [state, setState] = useState<MediaState>({
    key: "",
    src: null,
    loading: true,
    error: null,
  });

  const retry = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    let objectUrl: string | null = null;

    const run = async () => {
      try {
        if (fileId) {
          try {
            const signed = await fileApi.sign(fileId);
            if (!cancelled) {
              setState({ key, src: absolute(signed.url), loading: false, error: null });
            }
            return;
          } catch {
            // Fall through to an authenticated blob fetch.
          }
        }
        if (!url) throw new Error("attachment URL is missing");
        objectUrl = await fileApi.downloadBlob(url);
        if (cancelled) {
          URL.revokeObjectURL(objectUrl);
          objectUrl = null;
          return;
        }
        setState({ key, src: objectUrl, loading: false, error: null });
      } catch (err) {
        if (!cancelled) {
          setState({
            key,
            src: null,
            loading: false,
            error: err instanceof Error ? err.message : "failed to load attachment",
          });
        }
      }
    };

    void run();
    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [fileId, url, key]);

  const current = state.key === key ? state : { key, src: null, loading: true, error: null };
  return { ...current, retry };
}
