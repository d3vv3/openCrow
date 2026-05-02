// ─── openCrow Overview Page ───

"use client";

import OverviewView from "@/components/OverviewView";

export default function OverviewPage() {
  return (
    <div className="flex-1 overflow-y-auto p-8 animate-in fade-in slide-in-from-bottom-3 duration-300">
      <OverviewView />
    </div>
  );
}
