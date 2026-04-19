"use client";

import { useState, useEffect } from "react";
import type { TelegramBotConfig, UserConfig } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Toggle } from "@/components/ui/Toggle";
import { Button } from "@/components/ui/Button";
import { AnimatedDot } from "@/components/ui/AnimatedDot";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { SaveBar } from "./SaveBar";
import type { UpdateConfigFn } from "./types";

function TelegramBotCard({
  bot,
  index: i,
  updateConfig,
  isDefault,
  canToggleOff,
  onSetDefault,
}: {
  bot: TelegramBotConfig;
  index: number;
  updateConfig: UpdateConfigFn;
  isDefault: boolean;
  canToggleOff: boolean;
  onSetDefault: () => void;
}) {
  const configured = !!bot.botToken;
  const [expanded, setExpanded] = useState(!configured);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    latencyMs?: number;
    error?: string;
    detail?: string;
  } | null>(null);

  const handleTest = async (e?: React.MouseEvent) => {
    e?.stopPropagation();
    if (!bot.botToken) return;
    setTesting(true);
    setTestResult(null);
    try {
      const res = await endpoints.testTelegramBot({
        botToken: bot.botToken,
        notificationChatId: bot.notificationChatId || undefined,
      });
      setTestResult(res);
    } catch {
      setTestResult({ ok: false, error: "Request failed" });
    } finally {
      setTesting(false);
    }
  };

  // Auto-test when tab opens (component mounts with a configured token)
  useEffect(() => {
    if (bot.botToken) handleTest();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
      {/* Compact header */}
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        <AnimatedDot
          status={
            testing
              ? "pending"
              : testResult
                ? testResult.ok
                  ? "ok"
                  : "error"
                : bot.enabled
                  ? "ok"
                  : "idle"
          }
        />
        <span className="font-medium text-sm flex-1 truncate">{bot.label || `Bot ${i + 1}`}</span>
        {isDefault && (
          <span className="text-xs font-mono px-1.5 py-0.5 rounded bg-violet/15 text-violet-light">
            default
          </span>
        )}
        {bot.botToken && (
          <span className="text-xs text-on-surface-variant font-mono hidden sm:block">
            token set
          </span>
        )}
        {testResult && (
          <span
            className={`flex items-center gap-1.5 text-xs font-mono px-2 py-0.5 rounded ${testResult.ok ? "text-cyan bg-cyan/10" : "text-error bg-error/10"}`}
          >
            <AnimatedDot status={testResult.ok ? "ok" : "error"} />
            {testResult.ok ? "OK" : "FAIL"}
          </span>
        )}
        <svg
          className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${expanded ? "rotate-180" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-white/10 pt-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Label"
              value={bot.label}
              onChange={(e) =>
                updateConfig((c) => {
                  c.integrations.telegramBots[i].label = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Bot Token"
              type="password"
              value={bot.botToken}
              onChange={(e) =>
                updateConfig((c) => {
                  c.integrations.telegramBots[i].botToken = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Notification Chat ID"
              tooltip="You can find your chat ID on your telegram profile page, it's called ID and it's a 9 digit number"
              value={bot.notificationChatId}
              onChange={(e) =>
                updateConfig((c) => {
                  c.integrations.telegramBots[i].notificationChatId = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Poll Interval (seconds)"
              type="number"
              value={bot.pollIntervalSeconds}
              onChange={(e) =>
                updateConfig((c) => {
                  c.integrations.telegramBots[i].pollIntervalSeconds =
                    parseInt(e.target.value) || 5;
                  return c;
                })
              }
            />
          </div>
          <Input
            label="Allowed Chat IDs (comma-separated)"
            value={bot.allowedChatIds.join(", ")}
            onChange={(e) =>
              updateConfig((c) => {
                c.integrations.telegramBots[i].allowedChatIds = e.target.value
                  .split(",")
                  .map((s) => s.trim())
                  .filter(Boolean);
                return c;
              })
            }
          />
          {testResult && (
            <div
              className={`rounded-lg px-3 py-2 text-sm border ${testResult.ok ? "bg-cyan/10 text-cyan border-cyan/20" : "bg-error/10 text-error border-error/20"}`}
            >
              <div className="flex items-center gap-2">
                <span>{testResult.ok ? "Connected" : "Failed"}</span>
                {testResult.latencyMs != null && (
                  <span className="text-xs opacity-80">{testResult.latencyMs}ms</span>
                )}
              </div>
              {testResult.detail && <p className="text-xs mt-1 opacity-90">{testResult.detail}</p>}
              {testResult.error && <p className="text-xs mt-1 opacity-90">{testResult.error}</p>}
            </div>
          )}
          <div className="flex gap-2 pt-2 items-center flex-wrap">
            <Toggle
              label="Enabled"
              checked={bot.enabled}
              onChange={(v) =>
                updateConfig((c) => {
                  c.integrations.telegramBots[i].enabled = v;
                  return c;
                })
              }
            />
            <Toggle
              label="Default"
              checked={isDefault}
              onChange={(v) => {
                if (v) onSetDefault();
                // Cannot turn off if canToggleOff is false (only one bot, or is last default)
              }}
              disabled={isDefault && !canToggleOff}
            />
            <Button
              variant="secondary"
              size="sm"
              loading={testing}
              onClick={handleTest}
              disabled={!bot.botToken}
              className="ml-4"
            >
              Test bot
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto hover:text-error"
              onClick={() =>
                updateConfig((c) => {
                  c.integrations.telegramBots.splice(i, 1);
                  return c;
                })
              }
            >
              Remove
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export function TelegramTab({
  config,
  updateConfig,
  saving,
  saveFullConfig,
  saveStatus,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveStatus: string | null;
}) {
  return (
    <div className="space-y-6">
      <SectionHeader
        title="Telegram Bots"
        description="Connect Telegram bots for chat and notifications"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              updateConfig((c) => {
                c.integrations.telegramBots.push({
                  label: "",
                  botToken: "",
                  allowedChatIds: [],
                  notificationChatId: "",
                  enabled: true,
                  pollIntervalSeconds: 5,
                });
                return c;
              })
            }
          >
            Add Bot
          </Button>
        }
      />
      {config.integrations.telegramBots.map((bot, i) => (
        <TelegramBotCard
          key={i}
          bot={bot}
          index={i}
          updateConfig={updateConfig}
          isDefault={
            config.integrations.telegramBots.length === 1
              ? true
              : config.integrations.defaultNotificationBotId === bot.id
          }
          canToggleOff={config.integrations.telegramBots.length > 1}
          onSetDefault={() =>
            updateConfig((c) => {
              c.integrations.defaultNotificationBotId = bot.id ?? "";
              return c;
            })
          }
        />
      ))}
      {config.integrations.telegramBots.length === 0 && (
        <p className="text-on-surface-variant text-sm">No Telegram bots configured.</p>
      )}
      {/* Default Notification Bot section removed - managed via Default toggle in each card */}
      <SaveBar
        onClick={saveFullConfig}
        loading={saving}
        label="Save Telegram Config"
        status={saveStatus}
      />
    </div>
  );
}
