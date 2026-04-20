"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { DAVConfig, DAVTestResult, UserConfig } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Toggle } from "@/components/ui/Toggle";
import { Button } from "@/components/ui/Button";
import { AnimatedDot } from "@/components/ui/AnimatedDot";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { SaveBar } from "./SaveBar";
import { emptyDAVConfig } from "./types";
import type { UpdateConfigFn } from "./types";

function DiscoveryBlock({ title, info }: { title: string; info?: DAVTestResult["webdav"] }) {
  if (!info) return null;
  return (
    <div className="rounded-md border border-white/10 bg-surface-low p-3 space-y-2">
      <div className="flex items-center gap-2">
        <AnimatedDot status={info.error ? "error" : info.enabled ? "ok" : "idle"} />
        <span className="text-sm font-medium">{title}</span>
      </div>
      {info.homeSet && (
        <p className="text-xs font-mono text-on-surface-variant">Home: {info.homeSet}</p>
      )}
      {info.error && <p className="text-xs font-mono text-error">{info.error}</p>}
      {!!info.collections?.length && (
        <div className="space-y-1">
          {info.collections.map((collection) => (
            <div key={collection.path} className="text-xs font-mono text-on-surface-variant">
              {collection.displayName || collection.path}
            </div>
          ))}
        </div>
      )}
      {!!info.entries?.length && (
        <div className="space-y-1">
          {info.entries.slice(0, 6).map((entry) => (
            <div key={entry.path} className="text-xs font-mono text-on-surface-variant">
              {entry.displayName || entry.path}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function davCardKey(dav: DAVConfig, index: number): string {
  return dav.id?.trim() || `idx-${index}`;
}

export function DAVTab({
  config,
  updateConfig,
  saving,
  saveFullConfig,
  saveStatus,
  isActive,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveStatus: string | null;
  isActive: boolean;
}) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [testingByKey, setTestingByKey] = useState<Record<string, boolean>>({});
  const [testResultsByKey, setTestResultsByKey] = useState<Record<string, DAVTestResult | null>>(
    {},
  );
  const wasActiveRef = useRef(false);

  const handleTest = useCallback(async (dav: DAVConfig, index: number) => {
    const key = davCardKey(dav, index);
    if (!dav.url.trim()) {
      setTestResultsByKey((prev) => ({
        ...prev,
        [key]: { ok: false, latencyMs: 0, error: "URL required" },
      }));
      return;
    }
    setTestingByKey((prev) => ({ ...prev, [key]: true }));
    setTestResultsByKey((prev) => ({ ...prev, [key]: null }));
    try {
      const result = await endpoints.testDAVConnection(dav);
      setTestResultsByKey((prev) => ({ ...prev, [key]: result }));
    } catch {
      setTestResultsByKey((prev) => ({
        ...prev,
        [key]: { ok: false, latencyMs: 0, error: "Request failed" },
      }));
    } finally {
      setTestingByKey((prev) => ({ ...prev, [key]: false }));
    }
  }, []);

  useEffect(() => {
    if (!isActive) {
      wasActiveRef.current = false;
      return;
    }
    if (wasActiveRef.current) return;

    wasActiveRef.current = true;
    const probeAll = async () => {
      const tests = config.integrations.dav
        .map((dav, index) => ({ dav, index }))
        .filter(({ dav }) => dav.url.trim().length > 0)
        .map(({ dav, index }) => handleTest(dav, index));
      if (tests.length > 0) {
        await Promise.allSettled(tests);
      }
    };
    void probeAll();
  }, [config.integrations.dav, handleTest, isActive]);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="WebDAV / CalDAV / CardDAV"
        description={
          <span>
            Configure DAV credentials, protocol toggles, and test live connectivity.{" "}
            <a
              href="https://www.davx5.com/tested-with"
              target="_blank"
              rel="noopener noreferrer"
              className="text-cyan hover:underline"
            >
              Tested providers →
            </a>
          </span>
        }
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() =>
              updateConfig((c) => {
                c.integrations.dav.push({
                  ...emptyDAVConfig,
                  name: `DAV ${c.integrations.dav.length + 1}`,
                  enabled: true,
                });
                return c;
              })
            }
          >
            Add DAV Integration
          </Button>
        }
      />

      {config.integrations.dav.map((dav, index) => {
        const key = davCardKey(dav, index);
        const configured = !!dav.url.trim();
        const isExpanded = expanded[key] ?? !configured;
        const testing = testingByKey[key] ?? false;
        const testResult = testResultsByKey[key] ?? null;

        return (
          <div
            key={key}
            className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden"
          >
            <button
              className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
              onClick={() => setExpanded((prev) => ({ ...prev, [key]: !isExpanded }))}
            >
              <AnimatedDot
                status={
                  testing ? "pending" : testResult ? (testResult.ok ? "ok" : "error") : "idle"
                }
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium truncate">
                  {dav.name?.trim() || `DAV ${index + 1}`}
                </p>
                <p className="text-xs text-on-surface-variant truncate">
                  {testResult
                    ? testResult.ok
                      ? `Connected in ${testResult.latencyMs} ms`
                      : testResult.error || "Connection failed"
                    : configured
                      ? dav.url
                      : "Not configured"}
                </p>
              </div>
              {testResult && (
                <span
                  className={`flex items-center gap-1.5 text-xs font-mono px-2 py-0.5 rounded ${testResult.ok ? "text-cyan bg-cyan/10" : "text-error bg-error/10"}`}
                >
                  <AnimatedDot status={testResult.ok ? "ok" : "error"} />
                  {testResult.ok ? "OK" : "FAIL"}
                </span>
              )}
              {!!testResult?.capabilities?.length && (
                <span className="text-xs font-mono text-cyan hidden sm:inline">
                  {testResult.capabilities.join(" . ")}
                </span>
              )}
              <svg
                className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${isExpanded ? "rotate-180" : ""}`}
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>

            {isExpanded && (
              <div className="px-4 pb-4 space-y-4 border-t border-white/10 pt-4">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Input
                    label="Integration Name"
                    value={dav.name}
                    placeholder={`DAV ${index + 1}`}
                    onChange={(e) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].name = e.target.value;
                        return c;
                      })
                    }
                  />
                  <Input
                    label="DAV URL"
                    value={dav.url}
                    placeholder="https://dav.example.com/"
                    onChange={(e) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].url = e.target.value;
                        return c;
                      })
                    }
                  />
                  <Input
                    label="Username"
                    value={dav.username}
                    onChange={(e) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].username = e.target.value;
                        return c;
                      })
                    }
                  />
                  <Input
                    label="Password"
                    type="password"
                    value={dav.password}
                    onChange={(e) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].password = e.target.value;
                        return c;
                      })
                    }
                  />
                  <Input
                    label="Poll Interval (seconds)"
                    type="number"
                    value={dav.pollIntervalSeconds}
                    onChange={(e) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].pollIntervalSeconds =
                          parseInt(e.target.value) || 900;
                        return c;
                      })
                    }
                  />
                </div>

                <div className="flex flex-wrap items-center gap-6">
                  <Toggle
                    label="Enabled"
                    checked={dav.enabled}
                    onChange={(v) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].enabled = v;
                        return c;
                      })
                    }
                  />
                  <Toggle
                    label="WebDAV"
                    checked={dav.webdavEnabled}
                    onChange={(v) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].webdavEnabled = v;
                        return c;
                      })
                    }
                  />
                  <Toggle
                    label="CalDAV"
                    checked={dav.caldavEnabled}
                    onChange={(v) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].caldavEnabled = v;
                        return c;
                      })
                    }
                  />
                  <Toggle
                    label="CardDAV"
                    checked={dav.carddavEnabled}
                    onChange={(v) =>
                      updateConfig((c) => {
                        c.integrations.dav[index].carddavEnabled = v;
                        return c;
                      })
                    }
                  />
                </div>

                {testResult?.principal && (
                  <p className="text-xs font-mono text-on-surface-variant">
                    Principal: {testResult.principal}
                  </p>
                )}

                {(testResult?.webdav || testResult?.caldav || testResult?.carddav) && (
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                    <DiscoveryBlock title="WebDAV" info={testResult.webdav} />
                    <DiscoveryBlock title="CalDAV" info={testResult.caldav} />
                    <DiscoveryBlock title="CardDAV" info={testResult.carddav} />
                  </div>
                )}

                <div className="flex gap-2 pt-2 items-center">
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={testing}
                    onClick={(e) => {
                      e.stopPropagation();
                      void handleTest(dav, index);
                    }}
                  >
                    Test connection
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="ml-auto hover:text-error"
                    onClick={(e) => {
                      e.stopPropagation();
                      updateConfig((c) => {
                        c.integrations.dav.splice(index, 1);
                        return c;
                      });
                    }}
                  >
                    Remove
                  </Button>
                </div>
              </div>
            )}
          </div>
        );
      })}

      {config.integrations.dav.length === 0 && (
        <p className="text-on-surface-variant text-sm">No DAV integrations configured.</p>
      )}

      <SaveBar
        onClick={saveFullConfig}
        loading={saving}
        label="Save DAV Config"
        status={saveStatus}
      />
    </div>
  );
}
