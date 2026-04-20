"use client";

import { useState, useEffect } from "react";
import type { UserConfig, DeviceRegistration } from "@/lib/api";
import { endpoints, getApiBase } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { SaveBar } from "./SaveBar";
import type { UpdateConfigFn } from "./types";
import { CompanionAppCard } from "./CompanionAppCard";
import { AddDeviceModal } from "./AddDeviceModal";
import { DeviceTasksPanel } from "./DeviceTasksPanel";

export function DevicesTab({
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
  const [registrations, setRegistrations] = useState<Record<string, DeviceRegistration>>({});
  const [serverUrl, setServerUrl] = useState(() => getApiBase());
  const [isAddingDevice, setIsAddingDevice] = useState(false);

  const companionApps = config.integrations.companionApps || [];

  useEffect(() => {
    endpoints
      .listDeviceRegistrations()
      .then((res) => {
        const map: Record<string, DeviceRegistration> = {};
        for (const r of res.registrations ?? []) map[r.deviceId] = r;
        setRegistrations(map);
      })
      .catch(() => {});
  }, []);

  return (
    <div className="space-y-8">
      <div className="space-y-6">
        <SectionHeader
          title="Companion Apps / Devices"
          description="Register companion apps that can poll for remote tasks"
          action={
            <Button variant="secondary" size="sm" onClick={() => setIsAddingDevice(true)}>
              Add Device
            </Button>
          }
        />

        <div className="flex items-center gap-3 p-3 rounded-lg bg-surface-mid border border-white/5">
          <div className="flex-1">
            <Input
              label="Server URL for pairing"
              value={serverUrl}
              onChange={(e) => setServerUrl(e.target.value)}
              placeholder="http://192.168.1.x:8080"
            />
          </div>
          <p className="text-xs text-on-surface-variant mt-4 max-w-xs">
            The URL your phone will use to reach this server. Domain or IP.
          </p>
        </div>

        {companionApps.map((app, i) => (
          <CompanionAppCard
            key={app.id || i}
            app={app}
            index={i}
            registration={app.id ? registrations[app.id] : undefined}
            serverUrl={serverUrl}
            updateConfig={updateConfig}
          />
        ))}
        {!companionApps.length && (
          <p className="text-on-surface-variant text-sm">No companion apps configured.</p>
        )}
        <SaveBar
          onClick={saveFullConfig}
          loading={saving}
          label="Save Device Config"
          status={saveStatus}
        />
      </div>

      <div className="h-px bg-white/10" />

      <div className="space-y-6">
        <SectionHeader
          title="Pending Device Tasks"
          description="Tasks waiting to be polled by the companion apps"
        />
        <DeviceTasksPanel companionApps={companionApps} />
      </div>

      {isAddingDevice && (
        <AddDeviceModal
          serverUrl={serverUrl}
          updateConfig={updateConfig}
          saveFullConfig={saveFullConfig}
          onClose={() => setIsAddingDevice(false)}
        />
      )}
    </div>
  );
}
