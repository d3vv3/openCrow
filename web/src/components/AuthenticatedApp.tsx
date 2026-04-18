"use client";

import { useEffect, useState } from "react";
import ChatShell from "@/components/ChatShell";
import ConfigStudio from "@/components/ConfigStudio";
import OverviewView from "@/components/OverviewView";
import TerminalView from "@/components/TerminalView";
import { clearTokens, endpoints, type ConversationDTO } from "@/lib/api";

type Section = "chat" | "config" | "overview" | "terminal";

function ChatIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="shrink-0">
      <path d="M2 3a1 1 0 011-1h10a1 1 0 011 1v7a1 1 0 01-1 1H5l-3 3V3z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

function GearIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="shrink-0">
      <path d="M6.5 1.5h3l.5 2 1.5.87 2-.5 1.5 2.6-1.5 1.5v1.96l1.5 1.5-1.5 2.6-2-.5-1.5.87-.5 2h-3l-.5-2-1.5-.87-2 .5-1.5-2.6 1.5-1.5V7.97l-1.5-1.5 1.5-2.6 2 .5 1.5-.87.5-2z" stroke="currentColor" strokeWidth="1.2" strokeLinejoin="round" />
      <circle cx="8" cy="8" r="2" stroke="currentColor" strokeWidth="1.2" />
    </svg>
  );
}

function LogoutIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="shrink-0">
      <path d="M6 2H3a1 1 0 00-1 1v10a1 1 0 001 1h3M10.5 11.5L14 8l-3.5-3.5M14 8H6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function OverviewIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="shrink-0">
      <rect x="1.5" y="1.5" width="5.5" height="5.5" rx="1" stroke="currentColor" strokeWidth="1.4" />
      <rect x="9" y="1.5" width="5.5" height="5.5" rx="1" stroke="currentColor" strokeWidth="1.4" />
      <rect x="1.5" y="9" width="5.5" height="5.5" rx="1" stroke="currentColor" strokeWidth="1.4" />
      <rect x="9" y="9" width="5.5" height="5.5" rx="1" stroke="currentColor" strokeWidth="1.4" />
    </svg>
  );
}

function TerminalIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" className="shrink-0">
      <rect x="1" y="2.5" width="14" height="11" rx="1.5" stroke="currentColor" strokeWidth="1.4" />
      <path d="M4 6l2.5 2L4 10" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M9 10h3" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

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

export default function AuthenticatedApp() {
  const [activeSection, setActiveSection] = useState<Section>("chat");
  const [requestedConfigTab, setRequestedConfigTab] = useState<string | undefined>(undefined);
  const [conversations, setConversations] = useState<ConversationDTO[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [loadingConversations, setLoadingConversations] = useState(true);
  const [showSystemChats, setShowSystemChats] = useState(false);

  useEffect(() => {
    setLoadingConversations(true);
    endpoints
      .listConversations()
      .then((items) => {
        setConversations(items);
        if (!activeConversationId && items.length > 0) setActiveConversationId(items[0].id);
      })
      .catch(() => {
        setConversations([]);
        setActiveConversationId(null);
      })
      .finally(() => setLoadingConversations(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function handleLogout() {
    clearTokens();
    window.location.reload();
  }

  // Map section -> ConfigStudio tab key (for sections that render ConfigStudio)
  const sectionTitles: Record<Section, string> = {
    chat: "Chat",
    config: "Configuration",
    overview: "Overview",
    terminal: "Terminal",
  };

  const visibleConversations = showSystemChats
    ? conversations
    : conversations.filter((chat) => !chat.isAutomatic || chat.automationKind === "heartbeat");

  function SidebarButton({
    section,
    icon,
    label,
  }: {
    section: Section;
    icon: React.ReactNode;
    label: string;
  }) {
    const active = activeSection === section;
    return (
      <button
        onClick={() => {
          if (section === "config") setRequestedConfigTab(undefined);
          setActiveSection(section);
        }}
        className={`flex w-full items-center gap-3 rounded-sm px-4 py-2.5 text-sm transition-colors duration-150 ${
          active ? "text-violet-light" : "text-on-surface-variant hover:bg-surface-mid/50 hover:text-on-surface"
        }`}
      >
        {icon}
        {label}
      </button>
    );
  }

  return (
    <div className="h-screen overflow-hidden bg-surface">
      {/* ── Sidebar ── */}
      <aside className="fixed inset-y-0 left-0 z-40 flex w-[280px] flex-col bg-surface-low">
        {/* Atmospheric glow */}
        <div className="pointer-events-none absolute inset-0 overflow-hidden">
          <div className="absolute -left-20 -top-20 h-60 w-60 rounded-full bg-violet/[0.06] blur-3xl" />
          <div className="absolute -bottom-16 -left-10 h-48 w-48 rounded-full bg-cyan/[0.04] blur-3xl" />
        </div>

        {/* Branding */}
        <div className="relative px-5 pt-6 pb-4">
          <h1 className="font-display text-3xl font-bold tracking-tight">
            <span className="text-on-surface-variant">open</span>
            <span className="text-violet-light">Crow</span>
          </h1>
          <p className="mt-1 font-mono text-xs text-on-surface-variant">
            v0.1.0 . Active
          </p>
        </div>

        {/* Conversation nav */}
        <div className="relative mt-2 flex-1 overflow-hidden px-3 pb-2">
          <button
            onClick={() => {
              setActiveSection("chat");
              setRequestedConfigTab(undefined);
              setActiveConversationId(null);
            }}
            className="mb-3 flex w-full items-center justify-center gap-2 rounded-sm bg-violet px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-violet/90"
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
                <div className="space-y-1">
                {visibleConversations.map((chat) => {
                  const isActive = activeSection === "chat" && activeConversationId === chat.id;
                  const isAutomatic = !!chat.isAutomatic;
                  return (
                    <div
                      key={chat.id}
                      className={`group relative flex items-center rounded-sm transition-all ${
                        isAutomatic
                          ? isActive
                            ? "bg-surface-mid shadow-[inset_0_0_0_1px_var(--color-outline-ghost)]"
                            : "bg-surface-low hover:bg-surface-mid/70"
                          : isActive
                            ? "bg-surface-mid"
                            : "hover:bg-surface-mid/50"
                      }`}
                    >
                      <div className={`absolute inset-y-2 left-0 w-[3px] rounded-r-sm ${isActive ? "bg-violet" : isAutomatic ? "bg-warning/70" : "bg-cyan/70"}`} />
                      <button
                        onClick={() => {
                          setActiveSection("chat");
                          setRequestedConfigTab(undefined);
                          setActiveConversationId(chat.id);
                        }}
                        className={`flex-1 min-w-0 px-3 py-2 text-left ${
                          isAutomatic
                            ? isActive
                              ? "text-on-surface"
                              : "text-on-surface hover:text-on-surface"
                            : isActive
                              ? "text-violet-light"
                              : "text-on-surface-variant hover:text-on-surface"
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <p className={`truncate text-sm font-medium ${isAutomatic ? "text-on-surface" : ""}`}>{chat.title || "Untitled chat"}</p>
                        </div>
                        <div className="mt-1 flex items-center gap-2">
                          {isAutomatic && (
                            <span className={`inline-flex items-center rounded-sm px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider ${
                              isActive
                                ? "bg-violet/12 text-violet"
                                : "bg-warning/12 text-warning"
                            }`}>
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
                        className={`shrink-0 px-2 py-2 transition-all ${
                          isAutomatic
                            ? "text-on-surface-variant/60 opacity-100 group-hover:text-error"
                            : "text-on-surface-variant opacity-0 group-hover:opacity-100 hover:text-red-400"
                        }`}
                        title="Delete conversation"
                      >
                        <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                          <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 10h8l1-10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>
                      </button>
                    </div>
                  );
                })}
                </div>
              )}
            </div>

            <label className="mt-2 shrink-0 flex items-center justify-between gap-3 rounded-sm border-t border-outline-ghost px-2 py-3 text-xs font-mono text-on-surface-variant">
              <span>Show system chats</span>
              <button
                type="button"
                role="switch"
                aria-checked={showSystemChats}
                onClick={() => {
                  setShowSystemChats((prev) => {
                    const next = !prev;
                    if (!next && activeConversationId) {
                      const active = conversations.find((chat) => chat.id === activeConversationId);
                      if (active?.isAutomatic && active.automationKind !== "heartbeat") setActiveConversationId(null);
                    }
                    return next;
                  });
                }}
                className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${showSystemChats ? "bg-violet" : "bg-surface-high"}`}
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
          <SidebarButton section="overview" icon={<OverviewIcon />} label="Overview" />
          <SidebarButton section="terminal" icon={<TerminalIcon />} label="Terminal" />
          <SidebarButton section="config" icon={<GearIcon />} label="Config" />
          <button
            onClick={handleLogout}
            className="flex w-full items-center gap-3 rounded-sm px-4 py-2.5 text-sm text-on-surface-variant transition-colors duration-150 hover:text-error"
          >
            <LogoutIcon />
            Logout
          </button>
        </div>
      </aside>

      {/* ── Main Content ── */}
      <div className="ml-[280px] flex h-screen flex-col">
        {activeSection === "chat" ? (
          <ChatShell
            activeConversationId={activeConversationId}
            onActiveConversationChange={setActiveConversationId}
            onConversationsUpdate={setConversations}
          />
        ) : activeSection === "overview" ? (
          <div className="flex-1 overflow-y-auto p-8">
            <OverviewView />
          </div>
        ) : activeSection === "terminal" ? (
          <div className="flex-1 overflow-hidden p-8 flex flex-col">
            <TerminalView />
          </div>
        ) : activeSection === "config" ? (
          <>
            <header className="shrink-0 flex items-center justify-between bg-surface/80 px-8 py-4 backdrop-blur-xl border-b border-outline-ghost">
              <h2 className="font-display text-xl text-on-surface">
                {sectionTitles[activeSection]}
              </h2>
            </header>
            <main className="flex-1 overflow-y-auto p-8">
              <ConfigStudio requestedTab={requestedConfigTab} />
            </main>
          </>
        ) : null}
      </div>
    </div>
  );
}
