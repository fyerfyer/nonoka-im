"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { authApi } from "@/lib/api";
import { User } from "@/types";
import { toast } from "sonner";

export function useAuth() {
  const router = useRouter();
  const pathname = usePathname();
  const store = useAuthStore();
  const [isReady, setIsReady] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      store.restoreFromStorage();
      setIsReady(true);
    }, 0);
    return () => clearTimeout(timer);
  }, [store]);

  const login = useCallback(
    async (username: string, password: string) => {
      try {
        const reply = await authApi.login({ username, password });
        const user: User = { userId: reply.userId, username };
        store.login(reply.token, reply.refreshToken, user);
        toast.success("Login successful");
        router.replace("/chat");
        return true;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Login failed";
        toast.error(message);
        return false;
      }
    },
    [router, store]
  );

  const register = useCallback(
    async (username: string, password: string) => {
      try {
        await authApi.register({ username, password });
        toast.success("Registration successful");
        return await login(username, password);
      } catch (err) {
        const message =
          err instanceof Error ? err.message : "Registration failed";
        toast.error(message);
        return false;
      }
    },
    [login]
  );

  const logout = useCallback(() => {
    // Best-effort server-side revocation of the refresh token.
    authApi.logout().catch(() => {});
    store.logout();
    router.replace("/login");
  }, [router, store]);

  // Redirect unauthenticated users away from protected routes.
  useEffect(() => {
    if (!isReady) return;
    const publicPaths = ["/login", "/register"];
    const isPublic = publicPaths.includes(pathname);
    if (!store.isAuthenticated && !isPublic) {
      router.replace("/login");
    }
    if (store.isAuthenticated && isPublic) {
      router.replace("/chat");
    }
  }, [isReady, store.isAuthenticated, pathname, router]);

  return {
    user: store.user,
    token: store.token,
    isAuthenticated: store.isAuthenticated,
    isReady,
    login,
    register,
    logout,
  };
}
