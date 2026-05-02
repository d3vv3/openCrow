// ─── openCrow Chat Page ───
// Renders ChatShell with conversation state bridged to Zustand store + URL routing.

"use client";

import { useRouter } from "next/router";
import ChatShell from "@/components/ChatShell";
import { useAppStore } from "@/lib/store";

export default function ChatPage() {
  const router = useRouter();
  const setConversations = useAppStore((s) => s.setConversations);

  return (
    <div className="flex-1 flex flex-col min-w-0 overflow-hidden animate-in fade-in duration-200">
      <ChatShell
        activeConversationId={null}
        onActiveConversationChange={(id) => {
          if (id) {
            router.push(`/chat/${id}`, undefined, { shallow: false });
          } else {
            router.push("/chat");
          }
        }}
        onConversationsUpdate={(list) => setConversations(list)}
        readOnly={false}
      />
    </div>
  );
}
