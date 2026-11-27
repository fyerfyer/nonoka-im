"use client";

import { useState } from "react";
import { Bot, Loader2, MessageSquare, ShieldCheck } from "lucide-react";
import { useAuth } from "@/hooks/useAuth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

type Mode = "login" | "register";

export default function LoginPage() {
  const { login, register, isReady } = useAuth();
  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  const passwordsMatch = password === confirmPassword;
  const canSubmit =
    username.trim().length > 0 &&
    password.length > 0 &&
    (mode === "login" || (confirmPassword.length > 0 && passwordsMatch));

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!canSubmit || isLoading) return;
    setIsLoading(true);
    try {
      if (mode === "login") {
        await login(username.trim(), password);
      } else {
        await register(username.trim(), password);
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-background p-4">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-70 [background-image:radial-gradient(circle_at_20%_20%,var(--color-primary)_0,transparent_32%),radial-gradient(circle_at_80%_80%,var(--color-info)_0,transparent_28%)] [background-size:100%_100%] [filter:blur(80px)]"
      />
      <div className="relative grid w-full max-w-5xl overflow-hidden rounded-2xl border bg-card shadow-xl lg:grid-cols-[1.1fr_1fr]">
        <div className="hidden flex-col justify-between border-r bg-sidebar p-8 lg:flex">
          <div>
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary text-lg font-bold text-primary-foreground shadow-sm shadow-primary/20">
              N
            </div>
            <h1 className="mt-8 text-2xl font-semibold tracking-tight">
              Nonoka IM
            </h1>
            <p className="mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">
              An agent-native messaging workspace. Talk to people, bring agents
              into the conversation, and keep every action reviewable.
            </p>
          </div>
          <ul className="space-y-3 text-sm text-muted-foreground">
            <li className="flex items-center gap-2">
              <MessageSquare className="h-4 w-4 text-primary" />
              Direct messages and group conversations
            </li>
            <li className="flex items-center gap-2">
              <Bot className="h-4 w-4 text-primary" />
              Agents with tools, citations and streaming replies
            </li>
            <li className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4 text-primary" />
              Approval gates and an operator audit trail
            </li>
          </ul>
        </div>

        <div className="p-6 sm:p-8">
          <div className="mb-6 flex rounded-lg border p-1">
            {(["login", "register"] as const).map((item) => (
              <button
                key={item}
                type="button"
                data-testid={`auth-mode-${item}`}
                className={cn(
                  "flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                  mode === item
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
                onClick={() => {
                  setMode(item);
                  setConfirmPassword("");
                }}
              >
                {item === "login" ? "Sign in" : "Create account"}
              </button>
            ))}
          </div>

          <div className="mb-6 lg:hidden">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary text-base font-bold text-primary-foreground">
              N
            </div>
            <h1 className="mt-4 text-xl font-semibold">Nonoka IM</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {mode === "login"
                ? "Sign in to continue."
                : "Create an account to get started."}
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="Enter your username"
                autoComplete="username"
                disabled={isLoading || !isReady}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder={
                  mode === "login" ? "Enter your password" : "Create a password"
                }
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                disabled={isLoading || !isReady}
              />
            </div>
            {mode === "register" && (
              <div className="space-y-2">
                <Label htmlFor="confirm-password">Confirm password</Label>
                <Input
                  id="confirm-password"
                  type="password"
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  placeholder="Confirm your password"
                  autoComplete="new-password"
                  disabled={isLoading || !isReady}
                />
                {confirmPassword && !passwordsMatch && (
                  <p className="text-xs text-destructive">
                    Passwords do not match
                  </p>
                )}
              </div>
            )}
            <Button
              type="submit"
              className="w-full"
              disabled={!canSubmit || isLoading || !isReady}
            >
              {isLoading && <Loader2 className="animate-spin" />}
              {mode === "login" ? "Sign in" : "Create account"}
            </Button>
          </form>

          <p className="mt-6 text-center text-xs leading-relaxed text-muted-foreground">
            By continuing you agree to operate this deployment under your
            organization&apos;s policy. Ask an administrator for platform access
            if you need the operator console.
          </p>
        </div>
      </div>
    </div>
  );
}
