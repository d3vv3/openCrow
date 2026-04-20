"use client";

import type { UserConfig } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { TextArea } from "@/components/ui/TextArea";
import { Toggle } from "@/components/ui/Toggle";
import { Select } from "@/components/ui/Select";
import { Card } from "@/components/ui/Card";
import { SectionHeader } from "@/components/ui/SectionHeader";
import type { UpdateConfigFn } from "./types";
import { SaveBar } from "./SaveBar";

export function HeartbeatTab({
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
  const providerOptions = [
    { value: "", label: "-- same as chat --" },
    ...config.llm.providers
      .filter((p) => p.enabled && p.name)
      .map((p) => ({ value: p.model || p.name, label: `${p.name} . ${p.model || "default"}` })),
  ];

  return (
    <div className="space-y-6">
      <SectionHeader title="Heartbeat" description="Autonomous heartbeat configuration" />
      <Card title="Configuration">
        <div className="space-y-4">
          <Toggle
            label="Enabled"
            checked={config.heartbeat.enabled}
            onChange={(v) =>
              updateConfig((c) => {
                c.heartbeat.enabled = v;
                return c;
              })
            }
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input
              label="Interval (seconds)"
              type="number"
              value={config.heartbeat.intervalSeconds}
              onChange={(e) =>
                updateConfig((c) => {
                  c.heartbeat.intervalSeconds = parseInt(e.target.value) || 0;
                  return c;
                })
              }
            />
            <Select
              label="Model"
              options={providerOptions}
              value={config.heartbeat.model}
              onChange={(e) =>
                updateConfig((c) => {
                  c.heartbeat.model = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Active Hours Start"
              type="time"
              value={config.heartbeat.activeHoursStart}
              onChange={(e) =>
                updateConfig((c) => {
                  c.heartbeat.activeHoursStart = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Active Hours End"
              type="time"
              value={config.heartbeat.activeHoursEnd}
              onChange={(e) =>
                updateConfig((c) => {
                  c.heartbeat.activeHoursEnd = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Timezone"
              value={config.heartbeat.timezone}
              onChange={(e) =>
                updateConfig((c) => {
                  c.heartbeat.timezone = e.target.value;
                  return c;
                })
              }
            />
          </div>
        </div>
      </Card>
      <Card title="Heartbeat Prompt">
        <TextArea
          value={config.prompts.heartbeatPrompt}
          onChange={(e) =>
            updateConfig((c) => {
              c.prompts.heartbeatPrompt = e.target.value;
              return c;
            })
          }
          rows={8}
          className="font-mono"
        />
      </Card>
      <SaveBar onClick={onSave} loading={saving} label="Save Heartbeat" status={saveStatus} />
    </div>
  );
}
