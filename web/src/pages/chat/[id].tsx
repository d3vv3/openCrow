// ─── openCrow Chat/[id] Page ───
// Chat page for a specific conversation. Reads id from the URL.

"use client";

import { useRouter } from "next/router";
import ChatShell from "@/components/ChatShell";
import { useAppStore } from "@/lib/store";

export default function ChatWithConversationPage() {
  const router = useRouter();
  const { id } = router.query as { id?: string };

  const conversations = useAppStore((s) => s.conversations);
  const setConversations = useAppStore((s) => s.setConversations);

  const conversationId = id ?? null;
  const activeConversation = conversations.find((c) => c.id === conversationId);
  const isReadOnly = !!activeConversation?.channel;

  return (
    <div className="flex-1 flex flex-col min-w-0 overflow-hidden animate-in fade-in duration-200">
      <ChatShell
        activeConversationId={conversationId}
        onActiveConversationChange={(newId) => {
          if (newId) {
            router.push(`/chat/${newId}`, undefined, { shallow: false });
          } else {
            router.push("/chat");
          }
        }}
        onConversationsUpdate={(list) => setConversations(list)}
        readOnly={isReadOnly}
      />
    </div>
  );
}
