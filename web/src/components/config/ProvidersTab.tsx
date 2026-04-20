"use client";

import type { UserConfig } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { emptyProvider } from "./types";
import type { UpdateConfigFn, ProviderProbeStatus } from "./types";
import { ProviderCard } from "./ProviderCard";
import { SaveBar } from "./SaveBar";

export function ProvidersTab({
  config,
  updateConfig,
  saving,
  onSave,
  saveStatus,
  providerStatuses,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  onSave: () => Promise<void>;
  saveStatus: string | null;
  providerStatuses: Record<string, ProviderProbeStatus>;
}) {
  return (
    <div className="space-y-6">
      <SectionHeader
        title="LLM Providers"
        description="Configure AI model providers and fallback order"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              updateConfig((c) => {
                c.llm.providers.push({ ...emptyProvider });
                return c;
              })
            }
          >
            Add Provider
          </Button>
        }
      />
      {config.llm.providers.map((prov, i) => (
        <ProviderCard
          key={i}
          prov={prov}
          index={i}
          updateConfig={updateConfig}
          probeStatus={providerStatuses[prov.name] ?? null}
        />
      ))}
      {config.llm.providers.length === 0 && (
        <p className="text-on-surface-variant text-sm">No providers configured.</p>
      )}
      <SaveBar onClick={onSave} loading={saving} label="Save Providers" status={saveStatus} />
    </div>
  );
}
