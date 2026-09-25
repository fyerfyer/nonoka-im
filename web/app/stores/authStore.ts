import { create } from "zustand";
import { AuthState, User } from "@/types";

interface AuthStore extends AuthState {
  login: (token: string, refreshToken: string, user: User) => void;
  logout: () => void;
  restoreFromStorage: () => void;
}

const initialState: AuthState = {
  user: null,
  token: null,
  isAuthenticated: false,
};

export const useAuthStore = create<AuthStore>((set) => ({
  ...initialState,
  login: (token, refreshToken, user) => {
    if (typeof window !== "undefined") {
      localStorage.setItem("token", token);
      localStorage.setItem("refreshToken", refreshToken);
      localStorage.setItem("user", JSON.stringify(user));
    }
    set({ token, user, isAuthenticated: true });
  },
  logout: () => {
    if (typeof window !== "undefined") {
      localStorage.removeItem("token");
      localStorage.removeItem("refreshToken");
      localStorage.removeItem("user");
    }
    set(initialState);
  },
  restoreFromStorage: () => {
    if (typeof window === "undefined") return;
    const token = localStorage.getItem("token");
    const raw = localStorage.getItem("user");
    let user: User | null = null;
    if (raw) {
      try {
        user = JSON.parse(raw) as User;
      } catch {
        user = null;
      }
    }
    if (token && user) {
      set({ token, user, isAuthenticated: true });
    } else {
      set(initialState);
    }
  },
}));
