// ─── openCrow Memory Graph Page ───

"use client";

import { MemoryGraphTab } from "@/components/config/MemoryGraphTab";

export default function MemoryPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-3 duration-300">
      <MemoryGraphTab />
    </div>
  );
}
