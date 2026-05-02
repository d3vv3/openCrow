// ─── openCrow Chat Page ───
// Thin shell: sets activeChatId to null in the store so the persistent
// ChatShell (mounted in AuthenticatedLayout) shows the new-conversation view.

"use client";

import { useEffect } from "react";
import { useAppStore } from "@/lib/store";

export default function ChatPage() {
  const setActiveChatId = useAppStore((s) => s.setActiveChatId);

  useEffect(() => {
    setActiveChatId(null);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return null;
}
