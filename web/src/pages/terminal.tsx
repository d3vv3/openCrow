// ─── openCrow Terminal Page ───

"use client";

import TerminalView from "@/components/TerminalView";

export default function TerminalPage() {
  return (
    <div className="flex-1 overflow-hidden p-8 flex flex-col animate-in fade-in slide-in-from-bottom-3 duration-300">
      <TerminalView />
    </div>
  );
}
