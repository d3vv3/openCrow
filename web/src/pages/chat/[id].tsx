// ─── openCrow Chat/[id] Page ───
// Thin shell: sets activeChatId in the store so the persistent ChatShell
// (mounted in AuthenticatedLayout) loads the correct conversation.

"use client";

import { useEffect } from "react";
import { useRouter } from "next/router";
import { useAppStore } from "@/lib/store";

export default function ChatWithConversationPage() {
  const router = useRouter();
  const { id } = router.query as { id?: string };
  const setActiveChatId = useAppStore((s) => s.setActiveChatId);

  useEffect(() => {
    if (id) setActiveChatId(id);
  }, [id]); // eslint-disable-line react-hooks/exhaustive-deps

  return null;
}
