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
        description="Configure external MCP servers. The built-in Config server is always enabled."
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

      {/* Built-in config MCP server -- always on, cannot be removed */}
      <div className="rounded-lg border border-outline-variant bg-surface-container p-4 opacity-80">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-on-surface">openCrow Config</p>
            <p className="text-xs text-on-surface-variant mt-0.5">
              Built-in . always enabled . <span className="font-mono">/v1/mcp/config</span>
            </p>
            <p className="text-xs text-on-surface-variant mt-1">
              Handles setup of email, Telegram, DAV, MCP servers, devices, skills, tasks, and
              heartbeat.
            </p>
          </div>
          <span className="text-xs rounded-full bg-primary/10 text-primary px-2 py-0.5 font-medium">
            System
          </span>
        </div>
      </div>

      {config.mcp.servers.map((server, i) => (
        <McpServerCard key={server.id ?? i} server={server} index={i} updateConfig={updateConfig} />
      ))}
      {config.mcp.servers.length === 0 && (
        <p className="text-on-surface-variant text-sm">No external MCP servers configured.</p>
      )}
      <SaveBar onClick={onSave} loading={saving} label="Save MCP Config" status={saveStatus} />
    </div>
  );
}
