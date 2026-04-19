"use client";

import { useState, useEffect } from "react";
import Image from "next/image";
import {
  endpoints,
  setTokens,
  isAuthenticated,
  setAuthFailureHandler,
  initApiBase,
} from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";

import { Spinner } from "@/components/ui/Spinner";
import { HeartbeatDot } from "@/components/ui/HeartbeatDot";
import AuthenticatedApp from "@/components/AuthenticatedApp";

interface HealthState {
  name: string;
}

export default function HomePage() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [health, setHealth] = useState<HealthState | null>(null);
  const [healthLoading, setHealthLoading] = useState(true);

  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [device, setDevice] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // Init API base URL from server config, then check auth
  useEffect(() => {
    initApiBase().then(() => {
      setAuthed(isAuthenticated());
      setDevice(navigator.userAgent.slice(0, 64) || "Web Browser");
      setAuthFailureHandler(() => setAuthed(false));
    });
  }, []);

  // Fetch health
  useEffect(() => {
    if (authed === true) return;
    endpoints
      .health()
      .then(setHealth)
      .catch(() => setHealth(null))
      .finally(() => setHealthLoading(false));
  }, [authed]);

  if (authed === null) return null;
  if (authed) return <AuthenticatedApp onLogout={() => setAuthed(false)} />;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await endpoints.login(username, password, device || "Web Browser");
      setTokens(res.tokens.accessToken, res.tokens.refreshToken);
      setAuthed(true);
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError("Authentication failed");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="relative min-h-dvh flex items-center justify-center bg-surface-lowest overflow-hidden">
      {/* Dot mesh */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage:
            "radial-gradient(circle, color-mix(in srgb, var(--color-violet) 45%, transparent) 1.5px, transparent 1.5px)",
          backgroundSize: "28px 28px",
          maskImage: "radial-gradient(ellipse 80% 80% at 50% 50%, black 30%, transparent 80%)",
          WebkitMaskImage:
            "radial-gradient(ellipse 80% 80% at 50% 50%, black 30%, transparent 80%)",
        }}
      />

      {/* Radial bg glow */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "radial-gradient(ellipse 60% 50% at 50% 45%, color-mix(in srgb, var(--color-surface-mid) 80%, transparent) 0%, transparent 100%)",
        }}
      />

      <div className="relative z-10 w-full max-w-sm px-6">
        {/* Logo */}
        <div className="animate-fade-in stagger-1 flex flex-col items-center justify-center mb-2">
          <Image
            src="/crow.svg"
            alt="openCrow Logo"
            width={72}
            height={72}
            className="mb-4 opacity-90 crow-icon"
            priority
          />
          <h1 className="font-display text-3xl font-bold">
            <span className="text-on-surface-variant">open</span>
            <span className="text-violet-light">Crow</span>
          </h1>
        </div>

        {/* Tagline */}
        <div className="animate-fade-in stagger-2 flex items-center justify-center gap-2 mb-8">
          <HeartbeatDot />
          <span className="font-mono text-xs tracking-[0.3em] text-on-surface-variant uppercase">
            The Monolithic Intelligence
          </span>
        </div>

        {/* Health status */}
        <div className="animate-fade-in stagger-3 flex items-center justify-center gap-3 mb-8">
          {healthLoading ? (
            <Spinner size="sm" />
          ) : health ? (
            <Badge variant="success" dot>
              {health.name} reachable
            </Badge>
          ) : (
            <Badge variant="error" dot>
              unreachable
            </Badge>
          )}
        </div>

        {/* Card */}
        <div className="animate-slide-up stagger-4 bg-surface-low rounded border border-outline-ghost shadow-float p-6">
          {/* Form */}
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <Input
              label="Username"
              type="text"
              placeholder="admin"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              autoComplete="username"
            />
            <Input
              label="Password"
              type="password"
              placeholder="&bull;&bull;&bull;&bull;&bull;&bull;&bull;&bull;"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoComplete="current-password"
            />
            <Input
              label="Device Label"
              type="text"
              value={device}
              onChange={(e) => setDevice(e.target.value)}
              placeholder="Web Browser"
            />

            {error && <p className="text-error text-xs font-mono break-words">{error}</p>}

            <Button
              type="submit"
              variant="primary"
              size="lg"
              loading={submitting}
              className="w-full mt-2"
            >
              Authenticate
            </Button>
          </form>
        </div>

        {/* Footer */}
        <p className="animate-fade-in stagger-5 text-center text-xs text-on-surface-variant/50 mt-6 font-mono">
          v0.1 &mdash; self-hosted intelligence
        </p>
      </div>
    </div>
  );
}
