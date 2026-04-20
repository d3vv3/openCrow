"use client";

import type { UserConfig } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { TextArea } from "@/components/ui/TextArea";
import { Toggle } from "@/components/ui/Toggle";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { emptyGolangTool } from "./types";
import type { UpdateConfigFn } from "./types";
import { SaveBar } from "./SaveBar";

export function ToolsTab({
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
      <SectionHeader title="Built-in Tools" description="Server-provided tools (read-only)" />
      {config.tools.definitions.length === 0 ? (
        <p className="text-on-surface-variant text-sm">No built-in tools registered.</p>
      ) : (
        <div className="space-y-2">
          {config.tools.definitions.map((tool, i) => (
            <div key={i} className="border border-outline-ghost rounded-md p-3 bg-surface-low">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                  <code className="text-sm font-mono text-cyan">{tool.name}</code>
                  <Badge variant="info">{tool.source || "builtin"}</Badge>
                </div>
                <Toggle
                  label="Enabled"
                  checked={config.tools.enabledTools[tool.name] ?? false}
                  onChange={(v) =>
                    updateConfig((c) => {
                      c.tools.enabledTools[c.tools.definitions[i].name] = v;
                      return c;
                    })
                  }
                />
              </div>
              <p className="text-xs text-on-surface-variant mb-2">{tool.description}</p>
              {tool.parameters.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {tool.parameters.map((param, pi) => (
                    <span
                      key={pi}
                      className="inline-flex items-center gap-1 rounded bg-surface-mid px-2 py-0.5 text-xs font-mono text-on-surface-variant"
                    >
                      {param.name}
                      <span className="text-on-surface-variant/60">:{param.type}</span>
                      {param.required && <span className="text-error">*</span>}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <SectionHeader
        title="Golang Tools"
        description="Custom server-side Go tool implementations"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              updateConfig((c) => {
                c.tools.golangTools.push({ ...emptyGolangTool });
                return c;
              })
            }
          >
            Add Golang Tool
          </Button>
        }
      />
      {config.tools.golangTools.map((gt, i) => (
        <Card key={i} title={gt.name || `Golang Tool ${i + 1}`}>
          <div className="space-y-3">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Input
                label="Name"
                value={gt.name}
                onChange={(e) =>
                  updateConfig((c) => {
                    c.tools.golangTools[i].name = e.target.value;
                    return c;
                  })
                }
              />
              <Input
                label="Description"
                value={gt.description}
                onChange={(e) =>
                  updateConfig((c) => {
                    c.tools.golangTools[i].description = e.target.value;
                    return c;
                  })
                }
              />
            </div>
            <TextArea
              label="Source Code"
              value={gt.sourceCode}
              onChange={(e) =>
                updateConfig((c) => {
                  c.tools.golangTools[i].sourceCode = e.target.value;
                  return c;
                })
              }
              rows={12}
              className="font-mono text-xs"
            />
            <div className="flex items-center gap-4">
              <Toggle
                label="Enabled"
                checked={gt.enabled}
                onChange={(v) =>
                  updateConfig((c) => {
                    c.tools.golangTools[i].enabled = v;
                    return c;
                  })
                }
              />
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto hover:text-error"
                onClick={() =>
                  updateConfig((c) => {
                    c.tools.golangTools.splice(i, 1);
                    return c;
                  })
                }
              >
                Remove
              </Button>
            </div>
          </div>
        </Card>
      ))}
      {config.tools.golangTools.length === 0 && (
        <p className="text-on-surface-variant text-sm">
          No Golang tools. Click &quot;Add Golang Tool&quot; to create one with example code.
        </p>
      )}

      <SaveBar onClick={onSave} loading={saving} label="Save Tools" status={saveStatus} />
    </div>
  );
}
