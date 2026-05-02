// ─── openCrow Zustand Store ───
// Shared state for the authenticated app shell: conversation list and sidebar preferences.

import { create } from "zustand";
import type { ConversationDTO } from "./api";

export interface AppStore {
  conversations: ConversationDTO[];
  setConversations: (list: ConversationDTO[]) => void;
  showSystemChats: boolean;
  setShowSystemChats: (show: boolean) => void;
  /** Active conversation ID for the persistent ChatShell instance. */
  activeChatId: string | null;
  setActiveChatId: (id: string | null) => void;
}

export const useAppStore = create<AppStore>((set) => ({
  conversations: [],
  setConversations: (list) => set({ conversations: list }),
  activeChatId: null,
  setActiveChatId: (id) => set({ activeChatId: id }),
  showSystemChats: (() => {
    try {
      return localStorage.getItem("showSystemChats") === "true";
    } catch {
      return false;
    }
  })(),
  setShowSystemChats: (show) => {
    try {
      localStorage.setItem("showSystemChats", String(show));
    } catch {
      /* localStorage unavailable */
    }
    set({ showSystemChats: show });
  },
}));
