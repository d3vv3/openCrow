// ─── openCrow Configuration/[tab] Page ───
// Redirects legacy /configuration/:tab URLs to /configuration?tab=:tab

"use client";

import { useEffect } from "react";
import { useRouter } from "next/router";

export default function ConfigurationTabPage() {
  const router = useRouter();
  const { tab } = router.query as { tab?: string };

  useEffect(() => {
    if (tab) {
      router.replace({ pathname: "/configuration", query: { tab } });
    }
  }, [tab, router]);

  // Brief flash before redirect; show nothing
  return null;
}
