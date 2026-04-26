"use client";

import { useCallback, useRef, useState } from "react";
import Image from "next/image";
import ChatShell from "@/components/ChatShell";
import ConfigStudio from "@/components/ConfigStudio";
import OverviewView from "@/components/OverviewView";
import TerminalView from "@/components/TerminalView";
import {
  ChatIcon,
  LogoutIcon,
  OverviewIcon,
  TerminalIcon,
  ToolIcon,
  TrashIcon,
} from "@/components/ui/icons";
import { clearTokens, endpoints, type ConversationDTO } from "@/lib/api";

type Section = "chat" | "config" | "overview" | "terminal";

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

type SidebarNavButtonProps = {
  section: Section;
  icon: React.ReactNode;
  label: string;
  tooltip?: string;
  activeSection: Section;
  setActiveSection: (section: Section) => void;
  setRequestedConfigTab: (tab: string | undefined) => void;
};

function SidebarNavButton({
  section,
  icon,
  label,
  tooltip,
  activeSection,
  setActiveSection,
  setRequestedConfigTab,
  onNavigate,
}: SidebarNavButtonProps & { onNavigate?: () => void }) {
  const active = activeSection === section;
  return (
    <button
      onClick={() => {
        if (section === "config") setRequestedConfigTab(undefined);
        setActiveSection(section);
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

export default function AuthenticatedApp({
  onLogout,
  openCrowVersion,
}: {
  onLogout?: () => void;
  openCrowVersion: string;
}) {
  const [activeSection, setActiveSection] = useState<Section>("chat");
  const [requestedConfigTab, setRequestedConfigTab] = useState<string | undefined>(undefined);
  const [conversations, setConversations] = useState<ConversationDTO[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [loadingConversations, setLoadingConversations] = useState(true);
  const [showSystemChats, setShowSystemChats] = useState<boolean>(() => {
    try {
      return localStorage.getItem("showSystemChats") === "true";
    } catch {
      return false;
    }
  });
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const initialLoadDone = useRef(false);

  // Single source of truth for conversations lives in useChatSession (via ChatShell).
  // This callback receives the authoritative list and handles first-load side effects.
  const handleConversationsUpdate = useCallback((list: ConversationDTO[]) => {
    setConversations(list);
    if (!initialLoadDone.current) {
      initialLoadDone.current = true;
      setLoadingConversations(false);
      if (list.length > 0) {
        setActiveConversationId((prev) => prev ?? list[0].id);
      }
    }
  }, []);

  function closeSidebar() {
    setSidebarOpen(false);
  }

  async function handleLogout() {
    try {
      await endpoints.logout();
    } catch {
      // best-effort server invalidation; always clear local auth state
    }
    clearTokens();
    if (onLogout) onLogout();
    else window.location.href = "/";
  }

  // Map section -> ConfigStudio tab key (for sections that render ConfigStudio)
  const sectionTitles: Record<Section, string> = {
    chat: "Chat",
    config: "Configuration",
    overview: "Overview",
    terminal: "Sandboxed terminal",
  };

  const visibleConversations = (
    showSystemChats ? conversations : conversations.filter((chat) => !chat.isAutomatic)
  ).sort((a, b) => {
    if (a.channel && !b.channel) return -1;
    if (!a.channel && b.channel) return 1;
    return 0;
  });

  const activeConversation = conversations.find((c) => c.id === activeConversationId);
  const isReadOnly = !!activeConversation?.channel;

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
            <p className="mt-1 font-mono text-xs text-on-surface-variant">{openCrowVersion}</p>
          </div>

          {/* Conversation nav */}
          <div className="relative mt-2 flex-1 overflow-hidden px-3 pb-2">
            <button
              onClick={() => {
                setActiveSection("chat");
                setRequestedConfigTab(undefined);
                setActiveConversationId(null);
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
                {loadingConversations ? (
                  <p className="px-2 py-4 text-xs text-on-surface-variant">Loading chats...</p>
                ) : visibleConversations.length === 0 ? (
                  <p className="px-2 py-4 text-xs text-on-surface-variant">
                    {showSystemChats ? "No chats yet" : "No user or heartbeat chats yet"}
                  </p>
                ) : (
                  <div className="space-y-1 p-0.5">
                    {visibleConversations.map((chat) => {
                      const isActive = activeSection === "chat" && activeConversationId === chat.id;
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
                              setActiveSection("chat");
                              setRequestedConfigTab(undefined);
                              setActiveConversationId(chat.id);
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
                                  pinned · read-only
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
                              setConversations((prev) => prev.filter((c) => c.id !== chat.id));
                              if (activeConversationId === chat.id) setActiveConversationId(null);
                            }}
                            className={`shrink-0 cursor-pointer px-2 py-2 transition-all ${
                              isAutomatic
                                ? "text-on-surface-variant/60 opacity-100 group-hover:text-error"
                                : "text-on-surface-variant opacity-0 group-hover:opacity-100 hover:text-red-400"
                            }`}
                            title="Delete conversation"
                          >
                            <TrashIcon />
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
                  onClick={() => {
                    setShowSystemChats((prev) => {
                      const next = !prev;
                      try {
                        localStorage.setItem("showSystemChats", String(next));
                      } catch {}
                      if (!next && activeConversationId) {
                        const active = conversations.find(
                          (chat) => chat.id === activeConversationId,
                        );
                        if (active?.isAutomatic && active.automationKind !== "heartbeat")
                          setActiveConversationId(null);
                      }
                      return next;
                    });
                  }}
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
              section="overview"
              icon={<OverviewIcon />}
              label="Overview"
              activeSection={activeSection}
              setActiveSection={setActiveSection}
              setRequestedConfigTab={setRequestedConfigTab}
              onNavigate={closeSidebar}
            />
            <SidebarNavButton
              section="terminal"
              icon={<TerminalIcon />}
              label="Sandboxed terminal"
              tooltip="Persistent Alpine Linux sandbox used by agents and workers when calling execute_shell_command. Installed packages and files persist across sessions."
              activeSection={activeSection}
              setActiveSection={setActiveSection}
              setRequestedConfigTab={setRequestedConfigTab}
              onNavigate={closeSidebar}
            />
            <SidebarNavButton
              section="config"
              icon={<ToolIcon />}
              label="Configuration"
              activeSection={activeSection}
              setActiveSection={setActiveSection}
              setRequestedConfigTab={setRequestedConfigTab}
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
        {activeSection === "chat" ? (
          <div
            key="chat"
            className="flex-1 flex flex-col min-w-0 overflow-hidden animate-in fade-in duration-200"
          >
            <ChatShell
              activeConversationId={activeConversationId}
              onActiveConversationChange={setActiveConversationId}
              onConversationsUpdate={handleConversationsUpdate}
              readOnly={isReadOnly}
            />
          </div>
        ) : activeSection === "overview" ? (
          <div
            key="overview"
            className="flex-1 overflow-y-auto p-8 animate-in fade-in slide-in-from-bottom-3 duration-300"
          >
            <OverviewView />
          </div>
        ) : activeSection === "terminal" ? (
          <div
            key="terminal"
            className="flex-1 overflow-hidden p-8 flex flex-col animate-in fade-in slide-in-from-bottom-3 duration-300"
          >
            <TerminalView />
          </div>
        ) : activeSection === "config" ? (
          <div
            key="config"
            className="flex-1 flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-3 duration-300"
          >
            <header className="shrink-0 flex items-center justify-between bg-surface/80 px-8 py-4 backdrop-blur-xl border-b border-outline-ghost">
              <h2 className="font-display text-3xl font-semibold text-on-surface">
                {sectionTitles[activeSection]}
              </h2>
            </header>
            <main className="flex-1 overflow-y-auto p-8">
              <ConfigStudio requestedTab={requestedConfigTab} />
            </main>
          </div>
        ) : null}
      </div>
    </div>
  );
}
