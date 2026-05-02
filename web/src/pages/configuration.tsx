// ─── openCrow Configuration Page ───

"use client";

import ConfigStudio from "@/components/ConfigStudio";

export default function ConfigurationPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden animate-in fade-in slide-in-from-bottom-3 duration-300">
      <header className="shrink-0 flex items-center justify-between bg-surface/80 px-8 py-4 backdrop-blur-xl border-b border-outline-ghost">
        <h2 className="font-display text-3xl font-semibold text-on-surface">Configuration</h2>
      </header>
      <main className="flex-1 overflow-y-auto p-8">
        <ConfigStudio />
      </main>
    </div>
  );
}
