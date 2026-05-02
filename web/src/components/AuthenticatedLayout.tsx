"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Image from "next/image";
import {
  ChatIcon,
  LogoutIcon,
  MemoryIcon,
  OverviewIcon,
  TerminalIcon,
  ToolIcon,
  TrashIcon,
} from "@/components/ui/icons";
import { clearTokens, endpoints, getOpenCrowVersion } from "@/lib/api";
import { useAppStore } from "@/lib/store";
import ChatShell from "@/components/ChatShell";

// ─── Helpers ───

function automationLabel(kind?: string) {
  switch (kind) {
    case "scheduled_task":
      return "Scheduled";
    case "heartbeat":
      return "Heartbeat";
    default:
      return "Automatic";
  }
}

// ─── Sub-components ───

function MobileMenuButton({
  onClick,
  ariaLabel,
  icon,
  className = "",
  style,
}: {
  onClick: () => void;
  ariaLabel: string;
  icon: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
}) {
  return (
    <button
      onClick={onClick}
      aria-label={ariaLabel}
      style={style}
      className={`flex items-center justify-center w-9 h-9 rounded-lg bg-surface-lowest/80 border border-violet/30 backdrop-blur-xl shadow-lg transition-all duration-200 hover:border-violet/60 md:hidden ${className}`}
    >
      {icon}
    </button>
  );
}

function SidebarNavButton({
  path,
  icon,
  label,
  tooltip,
  currentPath,
  onNavigate,
}: {
  path: string;
  icon: React.ReactNode;
  label: string;
  tooltip?: string;
  currentPath: string;
  onNavigate?: () => void;
}) {
  const router = useRouter();
  const active = currentPath.startsWith(path);

  return (
    <button
      onClick={() => {
        router.push(path);
        onNavigate?.();
      }}
      title={tooltip}
      className={`flex w-full items-center gap-3 cursor-pointer rounded-lg px-4 py-2.5 text-base transition-colors duration-150 ${
        active
          ? "text-violet-light"
          : "text-on-surface-variant hover:bg-surface-mid/50 hover:text-on-surface"
      }`}
    >
      {icon}
      {label}
    </button>
  );
}

// ─── Main Layout ───

export default function AuthenticatedLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const currentPath = router.pathname;

  const conversations = useAppStore((s) => s.conversations);
  const setConversations = useAppStore((s) => s.setConversations);
  const showSystemChats = useAppStore((s) => s.showSystemChats);
  const setShowSystemChats = useAppStore((s) => s.setShowSystemChats);
  const activeChatId = useAppStore((s) => s.activeChatId);
  const setActiveChatId = useAppStore((s) => s.setActiveChatId);

  const [sidebarOpen, setSidebarOpen] = useState(false);
  const version = getOpenCrowVersion();

  // Fetch conversations on mount if the store is empty (e.g. user lands on a non-chat page)
  useEffect(() => {
    if (conversations.length > 0) return;
    endpoints
      .listConversations()
      .then((data) => {
        if (data) setConversations(data);
      })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  function closeSidebar() {
    setSidebarOpen(false);
  }

  async function handleLogout() {
    try {
      await endpoints.logout();
    } catch {
      // best-effort
    }
    clearTokens();
    // Full reload to clear all in-memory state
    window.location.href = "/";
  }

  // ── Conversation list filtering ──
  const visibleConversations = (
    showSystemChats ? conversations : conversations.filter((chat) => !chat.isAutomatic)
  ).sort((a, b) => {
    if (a.channel && !b.channel) return -1;
    if (!a.channel && b.channel) return 1;
    return 0;
  });

  // Derive active conversation ID from URL for highlight
  let activeConversationId: string | null = null;
  if (currentPath.startsWith("/chat/")) {
    activeConversationId = router.query.id as string;
  }

  return (
    <div className="h-screen overflow-hidden bg-surface">
      {/* ── Mobile hamburger ── */}
      {!sidebarOpen && (
        <MobileMenuButton
          onClick={() => setSidebarOpen(true)}
          ariaLabel="Open sidebar"
          className="fixed top-4 left-4 z-50"
          icon={
            <svg
              width="18"
              height="18"
              viewBox="0 0 18 18"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
            >
              <line x1="2" y1="4.5" x2="16" y2="4.5" />
              <line x1="2" y1="9" x2="16" y2="9" />
              <line x1="2" y1="13.5" x2="16" y2="13.5" />
            </svg>
          }
        />
      )}

      {/* ── Backdrop (mobile only) ── */}
      <div
        onClick={closeSidebar}
        className={`fixed inset-0 z-30 bg-black/50 backdrop-blur-sm transition-opacity duration-300 md:hidden ${
          sidebarOpen ? "opacity-100 pointer-events-auto" : "opacity-0 pointer-events-none"
        }`}
      />

      {/* ── Sidebar Wrapper ── */}
      <div
        className={`fixed inset-y-4 left-4 z-40 flex items-start transition-transform duration-300 ease-in-out md:translate-x-0 md:animate-in md:slide-in-from-left-8 md:fade-in md:duration-500 md:ease-out ${
          sidebarOpen ? "translate-x-0" : "-translate-x-[calc(100%+1rem)]"
        }`}
      >
        {/* ── Sidebar ── */}
        <aside className="flex h-full w-[300px] shrink-0 flex-col overflow-hidden rounded-2xl border border-violet bg-surface-lowest/80 backdrop-blur-2xl shadow-[var(--shadow-float)]">
          {/* Atmospheric glow */}
          <div className="pointer-events-none absolute inset-0 overflow-hidden">
            <div className="absolute -left-20 -top-20 h-60 w-60 rounded-full bg-violet/[0.06] blur-3xl" />
            <div className="absolute -bottom-16 -left-10 h-48 w-48 rounded-full bg-cyan/[0.04] blur-3xl" />
          </div>

          {/* Branding */}
          <div className="relative px-5 pt-6 pb-4">
            <h1 className="font-display text-4xl font-bold tracking-tight flex items-center gap-2">
              <span>
                <span className="text-on-surface-variant">open</span>
                <span className="text-violet-light">Crow</span>
              </span>
              <Image
                src="/crow.svg"
                alt="openCrow"
                width={50}
                height={50}
                className="opacity-90 mx-4 crow-icon"
              />
            </h1>
            <p className="mt-1 font-mono text-xs text-on-surface-variant">{version}</p>
          </div>

          {/* Conversation nav */}
          <div className="relative mt-2 flex-1 overflow-hidden px-3 pb-2">
            <button
              onClick={() => {
                router.push("/chat");
                closeSidebar();
              }}
              className="mb-3 flex w-full cursor-pointer items-center justify-center gap-2 rounded-lg bg-violet px-4 py-2.5 text-base font-medium text-white transition-colors hover:bg-violet/90"
            >
              <ChatIcon />
              New chat
            </button>

            <p className="px-2 pb-2 text-xs uppercase tracking-wider text-on-surface-variant font-mono">
              Previous chats
            </p>

            <div className="flex h-[calc(100%-76px)] flex-col">
              <div className="min-h-0 flex-1 overflow-y-auto pr-1">
                {conversations.length === 0 && (
                  <p className="px-2 py-4 text-xs text-on-surface-variant">
                    {showSystemChats ? "No chats yet" : "No user or heartbeat chats yet"}
                  </p>
                )}
                {conversations.length > 0 && visibleConversations.length === 0 && (
                  <p className="px-2 py-4 text-xs text-on-surface-variant">
                    {showSystemChats ? "No chats yet" : "No user or heartbeat chats yet"}
                  </p>
                )}
                {visibleConversations.length > 0 && (
                  <div className="space-y-1 p-0.5">
                    {visibleConversations.map((chat) => {
                      const isActive =
                        currentPath.startsWith("/chat") && activeConversationId === chat.id;
                      const isAutomatic = !!chat.isAutomatic;
                      const isChannel = !!chat.channel;
                      const rawTitle = chat.title || "Untitled chat";
                      const displayTitle = isChannel
                        ? chat.channel!.charAt(0).toUpperCase() + chat.channel!.slice(1)
                        : rawTitle
                            .replace(/^\[heartbeat\]\s*/i, "")
                            .replace(/^heartbeat:\s*/i, "")
                            .replace(/^scheduled task:\s*/i, "")
                            .trim() || "Untitled chat";
                      return (
                        <div
                          key={chat.id}
                          className={`group relative flex items-center rounded-lg transition-all ${
                            isChannel
                              ? isActive
                                ? "bg-surface-mid/80 ring-1 ring-cyan/40 shadow-[0_4px_20px_4px_color-mix(in_srgb,var(--color-cyan)_12%,transparent)]"
                                : "bg-surface-low/60 hover:bg-surface-mid/50"
                              : isAutomatic
                                ? isActive
                                  ? "bg-surface-mid/80 ring-1 ring-outline-ghost"
                                  : "bg-surface-low/60 hover:bg-surface-mid/50"
                                : isActive
                                  ? "bg-surface-mid/80 ring-1 ring-violet/20"
                                  : "hover:bg-surface-mid/40"
                          }`}
                        >
                          <div
                            className={`absolute inset-y-2 left-0 w-[3px] rounded-r-full ${isActive ? "bg-violet" : isChannel ? "bg-cyan" : isAutomatic ? "bg-warning/70" : "bg-cyan/70"}`}
                          />
                          <button
                            onClick={() => {
                              router.push(`/chat/${chat.id}`);
                              closeSidebar();
                            }}
                            className={`flex-1 min-w-0 cursor-pointer px-3 py-2 text-left ${
                              isChannel
                                ? isActive
                                  ? "text-on-surface"
                                  : "text-on-surface hover:text-on-surface"
                                : isAutomatic
                                  ? isActive
                                    ? "text-on-surface"
                                    : "text-on-surface hover:text-on-surface"
                                  : isActive
                                    ? "text-violet-light"
                                    : "text-on-surface-variant hover:text-on-surface"
                            }`}
                          >
                            <div className="flex items-center gap-2">
                              {isChannel && (
                                <svg
                                  width="10"
                                  height="10"
                                  viewBox="0 0 16 16"
                                  fill="none"
                                  stroke="currentColor"
                                  strokeWidth="2"
                                  strokeLinecap="round"
                                  className="shrink-0 text-cyan"
                                >
                                  <path d="M2 5h12v8a1 1 0 01-1 1H3a1 1 0 01-1-1V5zM5 5V3a1 1 0 011-1h4a1 1 0 011 1v2" />
                                </svg>
                              )}
                              <p
                                className={`truncate text-sm font-medium ${isChannel ? "text-on-surface" : isAutomatic ? "text-on-surface" : ""}`}
                              >
                                {displayTitle}
                              </p>
                            </div>
                            <div className="mt-1 flex items-center gap-2">
                              {isChannel && (
                                <span className="inline-flex items-center whitespace-nowrap rounded-full border border-cyan/30 bg-cyan/10 px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider text-cyan">
                                  pinned . read-only
                                </span>
                              )}
                              {isAutomatic && !isChannel && (
                                <span
                                  className={`inline-flex items-center whitespace-nowrap rounded-full border px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider ${
                                    isActive
                                      ? "border-violet/30 bg-violet/10 text-violet"
                                      : "border-warning/30 bg-warning/10 text-warning"
                                  }`}
                                >
                                  {automationLabel(chat.automationKind)}
                                </span>
                              )}
                              <p className="truncate text-xs text-on-surface-variant">
                                {new Date(chat.updatedAt || chat.createdAt).toLocaleDateString()}
                              </p>
                            </div>
                          </button>
                          <button
                            onClick={async (e) => {
                              e.stopPropagation();
                              await endpoints.deleteConversation(chat.id);
                              setConversations(conversations.filter((c) => c.id !== chat.id));
                              if (activeConversationId === chat.id) {
                                router.push("/chat");
                              }
                            }}
                            className="shrink-0 cursor-pointer p-1 mr-4 text-on-surface-variant/50 opacity-0 group-hover:opacity-100 hover:text-error transition-all"
                            title="Delete conversation"
                          >
                            <TrashIcon width="16" height="16" />
                          </button>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              <label className="mt-2 shrink-0 flex items-center justify-between gap-3 rounded-sm border-t border-outline-ghost px-2 py-3 text-sm font-mono text-on-surface-variant">
                <span>Show system chats</span>
                <button
                  type="button"
                  role="switch"
                  aria-checked={showSystemChats}
                  onClick={() => setShowSystemChats(!showSystemChats)}
                  className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors ${showSystemChats ? "bg-violet" : "bg-surface-high"}`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${showSystemChats ? "translate-x-4" : "translate-x-0.5"}`}
                  />
                </button>
              </label>
            </div>
          </div>

          {/* Bottom actions */}
          <div className="relative border-t border-outline-ghost px-2 py-3 space-y-0.5">
            <SidebarNavButton
              path="/overview"
              icon={<OverviewIcon />}
              label="Overview"
              currentPath={currentPath}
              onNavigate={closeSidebar}
            />
            <SidebarNavButton
              path="/memory"
              icon={<MemoryIcon />}
              label="Memory Graph"
              currentPath={currentPath}
              onNavigate={closeSidebar}
            />
            <SidebarNavButton
              path="/terminal"
              icon={<TerminalIcon />}
              label="Sandboxed terminal"
              tooltip="Persistent Alpine Linux sandbox used by agents and workers when calling execute_shell_command. Installed packages and files persist across sessions."
              currentPath={currentPath}
              onNavigate={closeSidebar}
            />
            <SidebarNavButton
              path="/configuration"
              icon={<ToolIcon />}
              label="Configuration"
              currentPath={currentPath}
              onNavigate={closeSidebar}
            />
            <button
              onClick={() => void handleLogout()}
              className="flex w-full cursor-pointer items-center gap-3 rounded-lg px-4 py-2.5 text-base text-on-surface-variant transition-colors duration-150 hover:text-error"
            >
              <LogoutIcon />
              Logout
            </button>
          </div>
        </aside>

        <MobileMenuButton
          onClick={closeSidebar}
          ariaLabel="Close sidebar"
          className="shrink-0"
          style={{ marginLeft: "20px", marginTop: "4px" }}
          icon={
            <svg
              width="18"
              height="18"
              viewBox="0 0 18 18"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
            >
              <line x1="4" y1="4" x2="14" y2="14" />
              <line x1="14" y1="4" x2="4" y2="14" />
            </svg>
          }
        />
      </div>

      {/* ── Main Content ── */}
      <div className="md:ml-[332px] flex h-screen flex-col">
        {/* Persistent ChatShell -- stays mounted across /chat <-> /chat/[id] */}
        {currentPath.startsWith("/chat") && (
          <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
            <ChatShell
              activeConversationId={
                // Prefer URL-derived id (handles direct navigation / page refresh)
                // and fall back to store for in-app shallow navigation.
                currentPath.startsWith("/chat/")
                  ? ((router.query.id as string) ?? activeChatId)
                  : activeChatId
              }
              onActiveConversationChange={(id) => {
                setActiveChatId(id);
                if (id) {
                  router.push(`/chat/${id}`, undefined, { shallow: true });
                } else {
                  router.push("/chat", undefined, { shallow: true });
                }
              }}
              onConversationsUpdate={(list) => setConversations(list)}
              readOnly={!!conversations.find((c) => c.id === activeChatId)?.channel}
            />
          </div>
        )}
        {!currentPath.startsWith("/chat") && children}
      </div>
    </div>
  );
}
