"use client";

import type { UserConfig } from "@/lib/api";
import { TextArea } from "@/components/ui/TextArea";
import { Card } from "@/components/ui/Card";
import { SectionHeader } from "@/components/ui/SectionHeader";
import type { UpdateConfigFn } from "./types";
import { SaveBar } from "./SaveBar";

export function SoulTab({
  config,
  updateConfig,
  saving,
  onSave,
  saveStatus,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  onSave: () => Promise<void>;
  saveStatus: string | null;
}) {
  return (
    <div className="space-y-6">
      <SectionHeader title="Soul" description="System prompt and assistant behavior" />
      <Card title="System Prompt">
        <TextArea
          value={config.prompts.systemPrompt}
          onChange={(e) =>
            updateConfig((c) => {
              c.prompts.systemPrompt = e.target.value;
              return c;
            })
          }
          rows={12}
          className="font-mono"
        />
      </Card>
      <SaveBar onClick={onSave} loading={saving} label="Save Soul Config" status={saveStatus} />
    </div>
  );
}
