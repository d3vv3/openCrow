"use client";

import { useState } from "react";
import type { MemoryEntry } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { emptyMemory } from "./types";
import { MemoryRow } from "./MemoryRow";

export function MemoryTab({
  initialMemories,
  onError,
}: {
  initialMemories: MemoryEntry[];
  onError: (msg: string) => void;
}) {
  const [memories, setMemories] = useState<MemoryEntry[]>(initialMemories);
  const [loading, setLoading] = useState(false);

  const handleAdd = async () => {
    try {
      const created = await endpoints.createMemory({ ...emptyMemory });
      setMemories((prev) => [...prev, created]);
    } catch {
      onError("Failed to create memory entry");
    }
  };

  const handleDelete = async (id: string | undefined, i: number) => {
    if (id) {
      try {
        await endpoints.deleteMemory(id);
      } catch {
        onError("Failed to delete memory");
        return;
      }
    }
    setMemories((prev) => prev.filter((_, idx) => idx !== i));
  };

  const refresh = async () => {
    setLoading(true);
    try {
      const res = await endpoints.listMemories();
      setMemories(res.memories ?? []);
    } catch {
      onError("Failed to load memories");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-3">
      <SectionHeader
        title="Memory"
        description="Persistent memory entries learned by the assistant"
        action={
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={refresh} loading={loading}>
              Refresh
            </Button>
            <Button variant="secondary" size="sm" onClick={handleAdd}>
              Add Memory
            </Button>
          </div>
        }
      />
      {memories.map((mem, i) => (
        <MemoryRow
          key={mem.id ?? i}
          mem={mem}
          index={i}
          onUpdate={(updated) =>
            setMemories((prev) => {
              const next = [...prev];
              next[i] = updated;
              return next;
            })
          }
          onDelete={() => handleDelete(mem.id, i)}
        />
      ))}
      {memories.length === 0 && !loading && (
        <p className="text-on-surface-variant text-sm">
          No memory entries yet. The assistant learns memories automatically via conversation.
        </p>
      )}
    </div>
  );
}
