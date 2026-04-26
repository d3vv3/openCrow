"use client";

import { useState, useEffect } from "react";
import type { CompanionAppConfig, DeviceRegistration } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Toggle } from "@/components/ui/Toggle";
import { Button } from "@/components/ui/Button";
import type { UpdateConfigFn } from "./types";
import { DeviceQRModal } from "./DeviceQRModal";

function useIsOnline(lastSeenAt?: string) {
  const [isOnline, setIsOnline] = useState(false);
  useEffect(() => {
    const check = () =>
      setIsOnline(
        lastSeenAt ? Date.now() - new Date(lastSeenAt).getTime() < 10 * 60 * 1000 : false,
      );
    check();
    // Re-evaluate every 30s so the badge flips without a page reload
    const id = setInterval(check, 30_000);
    return () => clearInterval(id);
  }, [lastSeenAt]);
  return isOnline;
}

export function CompanionAppCard({
  app,
  index: i,
  registration,
  serverUrl,
  updateConfig,
}: {
  app: CompanionAppConfig;
  index: number;
  registration?: DeviceRegistration;
  serverUrl: string;
  updateConfig: UpdateConfigFn;
}) {
  const [generating, setGenerating] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [qrPayload, setQrPayload] = useState<string | null>(null);
  const isOnline = useIsOnline(registration?.lastSeenAt);

  const handleRemove = async () => {
    setRemoving(true);
    try {
      if (app.id) await endpoints.deleteDevice(app.id);
    } catch (e) {
      console.error("Failed to delete device on server:", e);
    } finally {
      setRemoving(false);
    }
    updateConfig((c) => {
      c.integrations.companionApps.splice(i, 1);
      return c;
    });
  };

  const handleRepair = async () => {
    setGenerating(true);
    try {
      const res = await endpoints.createDeviceTokens(app.label || app.name);
      setQrPayload(
        JSON.stringify({
          id: app.id,
          server: serverUrl,
          accessToken: res.tokens.accessToken,
          refreshToken: res.tokens.refreshToken,
        }),
      );
    } catch (e) {
      console.error(e);
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden p-4 space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <Input
          label="Display Name"
          value={app.label ?? ""}
          onChange={(e) =>
            updateConfig((c) => {
              c.integrations.companionApps[i].label = e.target.value;
              return c;
            })
          }
          placeholder="e.g. Pixel 8 Pro"
        />
        <Input
          label="Identifier"
          value={app.name}
          onChange={(e) =>
            updateConfig((c) => {
              c.integrations.companionApps[i].name = e.target.value;
              return c;
            })
          }
          placeholder="e.g. pixel8"
        />
      </div>
      {registration && registration.capabilities.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pt-1">
          {registration.capabilities.map((cap) => (
            <span
              key={cap.name}
              title={cap.description}
              className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-primary/10 text-primary border border-primary/20"
            >
              {cap.name}
            </span>
          ))}
        </div>
      )}
      <div className="flex gap-2 pt-2 items-center flex-wrap">
        <Toggle
          label="Enabled"
          checked={app.enabled}
          onChange={(v) =>
            updateConfig((c) => {
              c.integrations.companionApps[i].enabled = v;
              return c;
            })
          }
        />
        <span
          className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${isOnline ? "bg-green-500/15 text-green-400" : "bg-white/5 text-on-surface-variant"}`}
        >
          {isOnline ? "online" : "offline"}
        </span>
        <span className="text-xs text-on-surface-variant font-mono px-2 py-1 bg-white/5 rounded">
          ID: {app.id || "legacy"}
        </span>
        <Button variant="ghost" size="sm" onClick={handleRepair} loading={generating}>
          Re-Pair
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto hover:text-error"
          onClick={handleRemove}
          loading={removing}
        >
          Remove
        </Button>
      </div>
      {qrPayload && <DeviceQRModal payload={qrPayload} onClose={() => setQrPayload(null)} />}
    </div>
  );
}
