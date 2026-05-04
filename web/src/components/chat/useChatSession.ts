import {
  useState,
  useEffect,
  useRef,
  useCallback,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";
import {
  endpoints,
  type ConversationDTO,
  type MessageDTO,
  type ProviderConfig,
  type ToolCallRecord,
  type TokenUsage,
} from "@/lib/api";
import {
  buildCompletionMessage,
  toAttachmentPayload,
  toOptimisticAttachments,
  type PickedAttachmentFile,
} from "./attachments";
import { useAppStore } from "@/lib/store";

export function useChatSession({
  activeConversationId,
  onActiveConversationChange,
  onConversationsUpdate,
}: {
  activeConversationId: string | null;
  onActiveConversationChange: (id: string | null) => void;
  onConversationsUpdate: (conversations: ConversationDTO[]) => void;
}) {
  // State
  const [conversations, setConversations] = useState<ConversationDTO[]>([]);
  const [messages, setMessages] = useState<MessageDTO[]>([]);
  const [toolCallHistory, setToolCallHistory] = useState<ToolCallRecord[]>([]);
  const [composing, setComposing] = useState("");
  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [sending, setSending] = useState(false);
  const [lastUsage, setLastUsage] = useState<TokenUsage | null>(null);
  const [streamingMsgId, setStreamingMsgId] = useState<string | null>(null);
  const [loadingConvs] = useState(true);
  const [loadingMsgs, setLoadingMsgs] = useState(false);
  const [attachedFiles, setAttachedFiles] = useState<PickedAttachmentFile[]>([]);
  const [regeneratingId, setRegeneratingId] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [recording, setRecording] = useState(false);
  const [transcribing, setTranscribing] = useState(false);
  const setChatBusy = useAppStore((s) => s.setChatBusy);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const composeRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const skipNextMsgLoad = useRef(false);
  const conversationsRef = useRef<ConversationDTO[]>([]);
  // Refs for polling guards -- avoid stale-closure issues in intervals
  const sendingRef = useRef(false);
  const streamingRef = useRef(false);

  // Keep ref in sync so handleSend can build the updated list without stale closure
  useEffect(() => {
    conversationsRef.current = conversations;
  }, [conversations]);

  // Keep polling guard refs in sync
  useEffect(() => {
    sendingRef.current = sending;
  }, [sending]);
  useEffect(() => {
    streamingRef.current = streamingMsgId !== null;
  }, [streamingMsgId]);

  // ─── Sync conversations from store (layout owns fetching) ───
  const storeConversations = useAppStore((s) => s.conversations);
  useEffect(() => {
    setConversations(storeConversations);
  }, [storeConversations]);

  // ─── Load messages when active conversation changes ───
  useEffect(() => {
    if (!activeConversationId) {
      setMessages([]);
      setToolCallHistory([]);
      composeRef.current?.focus();
      return;
    }
    if (skipNextMsgLoad.current) {
      skipNextMsgLoad.current = false;
      return;
    }
    setLoadingMsgs(true);
    Promise.all([
      endpoints.getMessages(activeConversationId),
      endpoints.getToolCalls(activeConversationId),
    ])
      .then(([msgs, calls]) => {
        setMessages(msgs ?? []);
        setToolCallHistory(calls ?? []);
      })
      .catch(() => {})
      .finally(() => {
        setLoadingMsgs(false);
        composeRef.current?.focus();
      });
  }, [activeConversationId]);

  // ─── Load providers from config ───
  useEffect(() => {
    endpoints
      .getConfig()
      .then((cfg) => {
        const enabled = cfg.llm.providers.filter((p) => p.enabled);
        setProviders(enabled);
        if (enabled.length > 0) setSelectedProvider(enabled[0].name);
      })
      .catch(() => {});
  }, []);

  // ─── Poll active conversation messages every 5 s (skip while streaming / sending) ───
  const activeConversationIdRef = useRef<string | null>(null);
  useEffect(() => {
    activeConversationIdRef.current = activeConversationId;
  }, [activeConversationId]);

  useEffect(() => {
    const id = setInterval(() => {
      const convId = activeConversationIdRef.current;
      if (!convId || sendingRef.current || streamingRef.current) return;
      Promise.all([endpoints.getMessages(convId), endpoints.getToolCalls(convId)])
        .then(([msgs, calls]) => {
          // Only apply if the active conversation hasn't changed while we were fetching
          if (activeConversationIdRef.current !== convId) return;
          setMessages(msgs ?? []);
          setToolCallHistory(calls ?? []);
        })
        .catch(() => {});
    }, 5000);
    return () => clearInterval(id);
  }, []); // intentionally empty -- uses refs

  // ─── Auto-scroll ───
  // Instant when switching conversations, smooth for new messages arriving
  const instantScrollRef = useRef(false);
  useEffect(() => {
    instantScrollRef.current = true;
  }, [activeConversationId]);
  useEffect(() => {
    if (messages.length === 0) return;
    const behavior = instantScrollRef.current ? "instant" : "smooth";
    instantScrollRef.current = false;
    messagesEndRef.current?.scrollIntoView({ behavior: behavior as ScrollBehavior });
  }, [messages]);

  // ─── Copy message content ───
  const handleCopy = useCallback((msgId: string, content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      setCopiedId(msgId);
      setTimeout(() => setCopiedId(null), 3000);
    });
  }, []);

  // ─── Regenerate assistant message ───
  const handleRegenerate = useCallback(
    async (msgId: string, convId: string) => {
      if (!convId || regeneratingId) return;
      const targetMessage = messages.find((m) => m.id === msgId);
      const regenerateAt = targetMessage ? new Date(targetMessage.createdAt).getTime() : null;
      setRegeneratingId(msgId);
      // Replace message content with empty streaming placeholder
      setMessages((prev) => prev.map((m) => (m.id === msgId ? { ...m, content: "" } : m)));
      if (regenerateAt != null) {
        // Find the last user message before the target to use as the lower cutoff.
        // Tool calls from the current turn have timestamps >= that user message and
        // <= the assistant message, so we remove them now to avoid showing stale results.
        const lastUserBefore = [...messages]
          .filter((m) => m.role === "user" && new Date(m.createdAt).getTime() < regenerateAt)
          .pop();
        const cutoff = lastUserBefore ? new Date(lastUserBefore.createdAt).getTime() : regenerateAt;
        setToolCallHistory((prev) =>
          prev.filter((tc) => new Date(tc.createdAt).getTime() < cutoff),
        );
      }
      try {
        await endpoints.regenerateMessage(convId, msgId, (token) => {
          setMessages((prev) =>
            prev.map((m) => (m.id === msgId ? { ...m, content: m.content + token } : m)),
          );
        });
        const [msgs, calls] = await Promise.all([
          endpoints.getMessages(convId),
          endpoints.getToolCalls(convId),
        ]);
        setMessages(msgs ?? []);
        setToolCallHistory(calls ?? []);
      } catch (err) {
        console.error("Regenerate failed:", err);
        const msgs = await endpoints.getMessages(convId).catch(() => null);
        if (msgs) setMessages(msgs);
      } finally {
        setRegeneratingId(null);
      }
    },
    [messages, regeneratingId],
  );

  // ─── Send message ───
  const handleSend = useCallback(async () => {
    if ((!composing.trim() && attachedFiles.length === 0) || sending) return;
    const userContent = composing.trim();
    // Snapshot and clear attachments
    const currentAttachments = attachedFiles;
    setComposing("");
    setAttachedFiles([]);
    if (composeRef.current) composeRef.current.style.height = "36px";
    setSending(true);
    setChatBusy(true);

    const fullContent = userContent;
    const completionMessage = buildCompletionMessage(userContent, currentAttachments);
    const attachmentPayload = toAttachmentPayload(currentAttachments);
    const optimisticAttachments = toOptimisticAttachments(currentAttachments);

    let conversationId = activeConversationId;
    if (!conversationId) {
      const autoTitle = userContent.slice(0, 48) + (userContent.length > 48 ? "..." : "");
      try {
        const conv = await endpoints.createConversation(autoTitle || "New chat");
        conversationId = conv.id;
        const updatedList = [conv, ...conversationsRef.current];
        setConversations(updatedList);
        onConversationsUpdate(updatedList);
        onActiveConversationChange(conv.id);
        skipNextMsgLoad.current = true;
      } catch {
        setSending(false);
        return;
      }
    }

    if (!conversationId) {
      setSending(false);
      return;
    }

    // Optimistic user message
    const optimisticUser: MessageDTO = {
      id: `temp-${Date.now()}`,
      conversationId,
      role: "user",
      content: fullContent,
      attachments: optimisticAttachments,
      createdAt: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, optimisticUser]);

    try {
      // Persist user message
      const savedUser = await endpoints.createMessage(
        conversationId,
        "user",
        fullContent,
        attachmentPayload,
      );
      setMessages((prev) => prev.map((m) => (m.id === optimisticUser.id ? savedUser : m)));

      const providerOrder = selectedProvider ? [selectedProvider] : undefined;

      // Add a streaming assistant placeholder
      const streamId = `stream-${Date.now()}`;
      setStreamingMsgId(streamId);
      setMessages((prev) => [
        ...prev,
        {
          id: streamId,
          conversationId,
          role: "assistant",
          content: "",
          createdAt: new Date().toISOString(),
        },
      ]);

      let fullOutput = "";
      const liveToolIdsByName = new Map<string, string>();
      const finalOutput = await endpoints.streamComplete(
        conversationId,
        completionMessage,
        (token: string) => {
          fullOutput += token;
          setMessages((prev) =>
            prev.map((m) => (m.id === streamId ? { ...m, content: fullOutput } : m)),
          );
        },
        providerOrder,
        (name: string, args: string, kind?: "TOOL" | "MCP" | "DEVICE") => {
          const liveId = `live-${Date.now()}-${Math.random()}`;
          liveToolIdsByName.set(name, liveId);
          // Add optimistic live entry
          setToolCallHistory((prev) => [
            ...prev,
            {
              id: liveId,
              toolName: name,
              kind: kind ?? "TOOL",
              arguments: (() => {
                try {
                  return JSON.parse(args);
                } catch {
                  return {};
                }
              })(),
              createdAt: new Date().toISOString(),
            },
          ]);
        },
        (name: string, result: string, isError?: boolean) => {
          const liveId = liveToolIdsByName.get(name);
          if (!liveId) return;
          setToolCallHistory((prev) =>
            prev.map((tc) =>
              tc.id === liveId
                ? {
                    ...tc,
                    output: result,
                    error: isError ? result || "tool failed" : undefined,
                  }
                : tc,
            ),
          );
        },
        (usage: TokenUsage) => {
          setLastUsage(usage);
        },
      );
      setStreamingMsgId(null);
      // Use whichever is longer: accumulated tokens or server's confirmed output
      const resolvedOutput =
        finalOutput && finalOutput.length > fullOutput.length ? finalOutput : fullOutput;
      if (resolvedOutput && resolvedOutput !== fullOutput) {
        setMessages((prev) =>
          prev.map((m) => (m.id === streamId ? { ...m, content: resolvedOutput } : m)),
        );
      }
      // Refresh persisted messages + tool calls from server (replaces optimistic stream IDs)
      if (conversationId) {
        Promise.all([endpoints.getMessages(conversationId), endpoints.getToolCalls(conversationId)])
          .then(([msgs, calls]) => {
            setMessages(msgs ?? []);
            setToolCallHistory(calls ?? []);
          })
          .catch(() => {});
      }
    } catch (err) {
      console.error("Chat send failed", err);
      const details =
        err && typeof err === "object" && "message" in err
          ? String((err as { message?: unknown }).message ?? "")
          : "";
      setMessages((prev) => [
        ...prev,
        {
          id: `err-${Date.now()}`,
          conversationId,
          role: "system",
          content: details
            ? `Failed to get a response: ${details}`
            : "Failed to get a response. Please try again.",
          createdAt: new Date().toISOString(),
        },
      ]);
    } finally {
      setSending(false);
      setChatBusy(false);
      setStreamingMsgId(null);
      composeRef.current?.focus();
    }
  }, [
    activeConversationId,
    attachedFiles,
    composing,
    onActiveConversationChange,
    onConversationsUpdate,
    selectedProvider,
    sending,
    setChatBusy,
  ]);

  const handleFilesPicked = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    if (files.length === 0) return;
    files.forEach((file) => {
      const reader = new FileReader();
      reader.onload = () => {
        setAttachedFiles((prev) => [...prev, { file, dataUrl: reader.result as string }]);
      };
      reader.readAsDataURL(file);
    });
    e.target.value = "";
    composeRef.current?.focus();
  }, []);

  const removeAttachment = useCallback((index: number) => {
    setAttachedFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  // ─── Key handler ───
  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend],
  );

  const handleMic = useCallback(async () => {
    if (transcribing) return;

    if (recording) {
      // Stop recording
      mediaRecorderRef.current?.stop();
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const mr = new MediaRecorder(stream);
      audioChunksRef.current = [];
      mr.ondataavailable = (e) => {
        if (e.data.size > 0) audioChunksRef.current.push(e.data);
      };
      mr.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());
        setRecording(false);
        setTranscribing(true);
        try {
          const blob = new Blob(audioChunksRef.current, { type: "audio/webm" });
          const res = await endpoints.transcribeAudio(blob);
          if (res.transcript) {
            setComposing((prev) => (prev ? prev + " " + res.transcript : res.transcript));
            setTimeout(() => composeRef.current?.focus(), 50);
          }
        } catch {
          // silently ignore transcription errors
        } finally {
          setTranscribing(false);
        }
      };
      mr.start();
      mediaRecorderRef.current = mr;
      setRecording(true);
    } catch {
      // microphone permission denied or unavailable
    }
  }, [recording, transcribing]);
  return {
    conversations,
    messages,
    toolCallHistory,
    composing,
    setComposing,
    providers,
    selectedProvider,
    setSelectedProvider,
    sending,
    lastUsage,
    streamingMsgId,
    loadingConvs,
    loadingMsgs,
    attachedFiles,
    setAttachedFiles,
    regeneratingId,
    copiedId,
    recording,
    transcribing,
    messagesEndRef,
    composeRef,
    fileInputRef,
    handleSend,
    handleRegenerate,
    handleCopy,
    handleMic,
    handleFilesPicked,
    removeAttachment,
    handleKeyDown,
  };
}
