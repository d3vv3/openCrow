"use client";

import { useState, useEffect, useCallback } from "react";
import type { UserConfig, DeviceRegistration, DeviceSession } from "@/lib/api";
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
  saveWithUpdate,
  saveStatus,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveWithUpdate: (updater: (draft: UserConfig) => UserConfig) => void;
  saveStatus: string | null;
}) {
  const [registrations, setRegistrations] = useState<Record<string, DeviceRegistration>>({});
  const [sessions, setSessions] = useState<DeviceSession[]>([]);
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
  const [serverUrl, setServerUrl] = useState(() => getApiBase());
  const [isAddingDevice, setIsAddingDevice] = useState(false);
  const [removingOrphan, setRemovingOrphan] = useState<string | null>(null);
  const [now] = useState(() => Date.now());

  const companionApps = config.integrations.companionApps || [];
  const configuredIds = new Set(companionApps.map((a) => a.id).filter(Boolean));
  // Orphan registrations: in DB but not in config
  const orphanRegistrations = Object.values(registrations).filter(
    (r) => !configuredIds.has(r.deviceId),
  );
  // Orphan sessions: session exists but has no device registration (cancelled pairings etc.)
  // Exclude the current browser session and any paired device sessions.
  const registeredDeviceIds = new Set(Object.keys(registrations));
  const orphanSessions = sessions.filter(
    (s) => !registeredDeviceIds.has(s.id) && s.id !== currentSessionId,
  );

  const loadRegistrations = useCallback(() => {
    endpoints
      .listDeviceRegistrations()
      .then((res) => {
        const map: Record<string, DeviceRegistration> = {};
        for (const r of res.registrations ?? []) map[r.deviceId] = r;
        setRegistrations(map);
      })
      .catch(() => {});
    endpoints
      .listSessions()
      .then((res) => {
        setSessions(res.sessions ?? []);
        if (res.currentSessionId) setCurrentSessionId(res.currentSessionId);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    loadRegistrations();
    // Refresh every 30 s to keep online status and capabilities up to date
    const id = setInterval(loadRegistrations, 30_000);
    return () => clearInterval(id);
  }, [loadRegistrations]);

  const handleRemoveOrphan = async (deviceId: string) => {
    setRemovingOrphan(deviceId);
    try {
      await endpoints.deleteDevice(deviceId);
      setRegistrations((prev) => {
        const next = { ...prev };
        delete next[deviceId];
        return next;
      });
      // Server also deletes the session for this device -- remove it from local state too
      setSessions((prev) => prev.filter((s) => s.id !== deviceId));
    } catch (e) {
      console.error("Failed to remove orphan device:", e);
    } finally {
      setRemovingOrphan(null);
    }
  };

  const handleRemoveOrphanSession = async (sessionId: string) => {
    setRemovingOrphan(sessionId);
    try {
      await endpoints.deleteSession(sessionId);
      setSessions((prev) => prev.filter((s) => s.id !== sessionId));
    } catch (e) {
      console.error("Failed to remove orphan session:", e);
    } finally {
      setRemovingOrphan(null);
    }
  };

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

      {orphanRegistrations.length > 0 && (
        <>
          <div className="h-px bg-white/10" />
          <div className="space-y-4">
            <SectionHeader
              title="Orphan Registrations"
              description="Devices registered in the database but not in your config. You can safely remove these."
            />
            {orphanRegistrations.map((reg) => {
              const lastSeen = new Date(reg.lastSeenAt);
              const isOnline = now - lastSeen.getTime() < 10 * 60 * 1000;
              return (
                <div
                  key={reg.deviceId}
                  className="flex items-center gap-3 p-4 rounded-lg border border-white/10 bg-surface-mid"
                >
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-mono text-on-surface-variant">
                      {reg.deviceId}
                    </span>
                    <div className="flex items-center gap-2 mt-1">
                      <span
                        className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${isOnline ? "bg-green-500/15 text-green-400" : "bg-white/5 text-on-surface-variant"}`}
                      >
                        {isOnline ? "online" : "offline"}
                      </span>
                      <span className="text-[10px] text-on-surface-variant">
                        last seen {lastSeen.toLocaleString()}
                      </span>
                      <span className="text-[10px] text-on-surface-variant">
                        {reg.capabilities?.length ?? 0} capabilities
                      </span>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="hover:text-error shrink-0"
                    loading={removingOrphan === reg.deviceId}
                    onClick={() => handleRemoveOrphan(reg.deviceId)}
                  >
                    Remove
                  </Button>
                </div>
              );
            })}
          </div>
        </>
      )}

      {orphanSessions.length > 0 && (
        <>
          <div className="h-px bg-white/10" />
          <div className="space-y-4">
            <SectionHeader
              title="Orphan Sessions"
              description="Sessions that were created (e.g. during pairing) but never completed registration. Safe to remove."
            />
            {orphanSessions.map((session) => {
              const lastSeen = new Date(session.lastSeenAt);
              const created = new Date(session.createdAt);
              return (
                <div
                  key={session.id}
                  className="flex items-center gap-3 p-4 rounded-lg border border-white/10 bg-surface-mid"
                >
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-mono text-on-surface-variant">
                      {session.deviceLabel || session.id}
                    </span>
                    <div className="flex items-center gap-2 mt-1">
                      <span className="text-[10px] text-on-surface-variant">
                        created {created.toLocaleString()}
                      </span>
                      <span className="text-[10px] text-on-surface-variant">
                        last seen {lastSeen.toLocaleString()}
                      </span>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="hover:text-error shrink-0"
                    loading={removingOrphan === session.id}
                    onClick={() => handleRemoveOrphanSession(session.id)}
                  >
                    Remove
                  </Button>
                </div>
              );
            })}
          </div>
        </>
      )}

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
          saveWithUpdate={saveWithUpdate}
          onClose={() => setIsAddingDevice(false)}
          onPaired={() => {
            // Immediately refresh registrations so the card shows capabilities
            loadRegistrations();
          }}
        />
      )}
    </div>
  );
}
