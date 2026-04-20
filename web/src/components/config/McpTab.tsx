"use client";

import type { UserConfig } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { emptyMCPServer } from "./types";
import type { UpdateConfigFn } from "./types";
import { McpServerCard } from "./McpServerCard";
import { SaveBar } from "./SaveBar";

export function McpTab({
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
      <SectionHeader
        title="MCP Servers"
        description="Configure external MCP servers (config/UI only for now)"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              updateConfig((c) => {
                c.mcp.servers.push({ ...emptyMCPServer });
                return c;
              })
            }
          >
            Add MCP Server
          </Button>
        }
      />
      {config.mcp.servers.map((server, i) => (
        <McpServerCard key={server.id ?? i} server={server} index={i} updateConfig={updateConfig} />
      ))}
      {config.mcp.servers.length === 0 && (
        <p className="text-on-surface-variant text-sm">No MCP servers configured.</p>
      )}
      <SaveBar onClick={onSave} loading={saving} label="Save MCP Config" status={saveStatus} />
    </div>
  );
}
