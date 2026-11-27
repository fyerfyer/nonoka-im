"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { authApi } from "@/lib/api";
import { User } from "@/types";
import { toast } from "sonner";

const PUBLIC_PATHS = ["/login"];

export function useAuth() {
  const router = useRouter();
  const pathname = usePathname();
  const user = useAuthStore((state) => state.user);
  const token = useAuthStore((state) => state.token);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const loginToStore = useAuthStore((state) => state.login);
  const logoutFromStore = useAuthStore((state) => state.logout);
  const restoreFromStorage = useAuthStore((state) => state.restoreFromStorage);
  const [isReady, setIsReady] = useState(false);

  useEffect(() => {
    restoreFromStorage();
    // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate the auth store from localStorage once
    setIsReady(true);
  }, [restoreFromStorage]);

  const login = useCallback(
    async (username: string, password: string) => {
      try {
        const reply = await authApi.login({ username, password });
        const nextUser: User = { userId: Number(reply.userId), username };
        loginToStore(reply.token, reply.refreshToken, nextUser);
        toast.success("Login successful");
        router.replace("/chat");
        return true;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Login failed";
        toast.error(message);
        return false;
      }
    },
    [loginToStore, router]
  );

  const register = useCallback(
    async (username: string, password: string) => {
      try {
        await authApi.register({ username, password });
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
    logoutFromStore();
    router.replace("/login");
  }, [logoutFromStore, router]);

  // Redirect unauthenticated users away from protected routes and keep
  // authenticated users out of the login screen.
  useEffect(() => {
    if (!isReady) return;
    const isPublic = PUBLIC_PATHS.includes(pathname);
    if (!isAuthenticated && !isPublic) {
      router.replace("/login");
    }
    if (isAuthenticated && isPublic) {
      router.replace("/chat");
    }
  }, [isReady, isAuthenticated, pathname, router]);

  return {
    user,
    token,
    isAuthenticated,
    isReady,
    login,
    register,
    logout,
  };
}
