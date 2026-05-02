// ─── openCrow _app ───
// Root app wrapper: global CSS, API init, auth guard, conditional layout.

import "@/app/globals.css";
import type { AppProps } from "next/app";
import { useRouter } from "next/router";
import { useEffect, useState } from "react";
import AuthenticatedLayout from "@/components/AuthenticatedLayout";
import { isAuthenticated, setAuthFailureHandler, initApiBase } from "@/lib/api";

const PUBLIC_PATHS = ["/", "/_error", "/404", "/500"];

/** Ensures API base URL and version are loaded before anything renders. */
function AppInit({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    initApiBase().finally(() => setReady(true));
  }, []);

  if (!ready) return null;
  return <>{children}</>;
}

/** Redirects to / when auth is lost. */
function AuthGuard({ children }: { children: React.ReactNode }) {
  // null = not yet determined (show nothing), avoids false-positive redirect on first render
  const [authed, setAuthed] = useState<boolean | null>(null);
  const router = useRouter();

  // On mount, check auth and register the global failure handler
  useEffect(() => {
    const ok = isAuthenticated();
    setAuthed(ok);
    setAuthFailureHandler(() => setAuthed(false));
  }, []);

  // When authed flips to false, redirect to login
  useEffect(() => {
    if (authed === false && router.pathname !== "/") {
      router.replace("/");
    }
  }, [authed, router]);

  if (authed === null) return null; // still determining
  if (!authed) return null; // redirect in progress

  return <>{children}</>;
}

export default function App({ Component, pageProps }: AppProps) {
  const router = useRouter();
  const isPublic = PUBLIC_PATHS.includes(router.pathname);

  return (
    <AppInit>
      {isPublic ? (
        <Component {...pageProps} />
      ) : (
        <AuthGuard>
          <AuthenticatedLayout>
            <Component {...pageProps} />
          </AuthenticatedLayout>
        </AuthGuard>
      )}
    </AppInit>
  );
}
