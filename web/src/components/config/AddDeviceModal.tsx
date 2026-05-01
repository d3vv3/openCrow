"use client";

import { useState, useEffect, useRef } from "react";
import QRCode from "react-qr-code";
import { endpoints, ApiError } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import type { UserConfig, DeviceRegistration } from "@/lib/api";

// Animated checkmark SVG drawn with stroke-dashoffset trick
function AnimatedCheckmark() {
  return (
    <div className="flex items-center justify-center">
      <svg
        width="72"
        height="72"
        viewBox="0 0 72 72"
        fill="none"
        className="animate-in zoom-in duration-300 text-green-500 dark:text-green-400"
      >
        <circle
          cx="36"
          cy="36"
          r="33"
          stroke="currentColor"
          strokeWidth="3"
          fill="none"
          className="opacity-20"
        />
        <circle
          cx="36"
          cy="36"
          r="33"
          stroke="currentColor"
          strokeWidth="3"
          fill="none"
          strokeDasharray="207"
          strokeDashoffset="207"
          strokeLinecap="round"
          style={{ animation: "drawCircle 0.5s ease forwards" }}
        />
        <polyline
          points="21,37 31,47 51,27"
          stroke="currentColor"
          strokeWidth="4"
          fill="none"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeDasharray="45"
          strokeDashoffset="45"
          style={{ animation: "drawCheck 0.4s ease 0.4s forwards" }}
        />
      </svg>
      <style>{`
        @keyframes drawCircle {
          to { stroke-dashoffset: 0; }
        }
        @keyframes drawCheck {
          to { stroke-dashoffset: 0; }
        }
      `}</style>
    </div>
  );
}

export function AddDeviceModal({
  serverUrl,
  saveWithUpdate,
  onClose,
  onPaired,
}: {
  serverUrl: string;
  saveWithUpdate: (updater: (draft: UserConfig) => UserConfig) => void;
  onClose: () => void;
  onPaired?: (registration: DeviceRegistration) => void;
}) {
  const [name, setName] = useState("");
  const [label, setLabel] = useState("");
  const [generating, setGenerating] = useState(false);
  const [qrPayload, setQrPayload] = useState<string | null>(null);
  const [deviceId, setDeviceId] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [pairedRegistration, setPairedRegistration] = useState<DeviceRegistration | null>(null);
  const [qrZoomed, setQrZoomed] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Start polling for device registration once QR is shown
  useEffect(() => {
    if (!qrPayload || !deviceId || pairedRegistration) return;

    pollRef.current = setInterval(async () => {
      try {
        const res = await endpoints.listDeviceRegistrations();
        const reg = (res.registrations ?? []).find((r) => r.deviceId === deviceId);
        if (reg) {
          clearInterval(pollRef.current!);
          pollRef.current = null;
          setPairedRegistration(reg);
          onPaired?.(reg);
        }
      } catch {
        // ignore poll errors
      }
    }, 2000);

    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [qrPayload, deviceId, pairedRegistration, onPaired]);

  const generate = async () => {
    const trimName = name.trim();
    const trimLabel = label.trim() || trimName;
    if (!trimName) return;
    setGenerating(true);
    setError(null);
    try {
      const res = await endpoints.createDeviceTokens(trimLabel);
      // Extract session ID from the access token payload (it's the `sid` claim).
      let sid: string | null = null;
      try {
        const payload = JSON.parse(atob(res.tokens.accessToken.split(".")[1]));
        sid = payload.sid ?? null;
      } catch {
        // ignore -- session cleanup on cancel will be best-effort
      }
      const newDeviceId = "dev_" + Math.random().toString(36).substring(2, 9);
      setDeviceId(newDeviceId);
      setSessionId(sid);
      setQrPayload(
        JSON.stringify({
          id: newDeviceId,
          server: serverUrl,
          accessToken: res.tokens.accessToken,
          refreshToken: res.tokens.refreshToken,
        }),
      );
      const newDevice = { id: newDeviceId, name: trimName, label: trimLabel, enabled: true };
      saveWithUpdate((c) => {
        if (!c.integrations.companionApps) c.integrations.companionApps = [];
        c.integrations.companionApps.push(newDevice);
        return c;
      });
    } catch (e) {
      if (e instanceof ApiError && e.status === 429) {
        setError(
          "Session limit reached. Please delete an existing device before pairing a new one.",
        );
      } else {
        setError("Failed to generate pairing QR. Please try again.");
      }
      console.error(e);
    } finally {
      setGenerating(false);
    }
  };

  const handleClose = () => {
    if (pollRef.current) clearInterval(pollRef.current);
    // If a session was created but the device never paired, delete the orphan session.
    if (sessionId && !pairedRegistration) {
      endpoints.deleteSession(sessionId).catch(() => {});
    }
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-surface-high border border-white/10 rounded-2xl p-6 w-full max-w-sm shadow-2xl flex flex-col gap-6">
        <div className="flex justify-between items-center">
          <h3 className="text-lg font-medium text-on-surface">Pair Companion App</h3>
          <button
            onClick={handleClose}
            className="text-on-surface-variant hover:text-on-surface p-1"
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            >
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>

        {!qrPayload ? (
          /* ── Step 1: enter name / label ── */
          <div className="space-y-4">
            <p className="text-sm text-on-surface-variant">
              Give this device a name and label to generate a pairing QR code.
            </p>
            <Input
              label="Display Name"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. Pixel 8 Pro"
              autoFocus
            />
            <Input
              label="Identifier"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. pixel8"
            />
            {error && (
              <div className="rounded-lg bg-red-500/10 border border-red-500/30 px-4 py-3 text-sm text-red-400">
                {error}
              </div>
            )}
            <Button
              variant="primary"
              className="w-full"
              onClick={generate}
              loading={generating}
              disabled={!name.trim()}
            >
              Generate QR Code
            </Button>
          </div>
        ) : pairedRegistration ? (
          /* ── Step 3: paired success ── */
          <div className="flex flex-col items-center gap-5 animate-in fade-in duration-300">
            <AnimatedCheckmark />
            <div className="text-center">
              <p className="text-green-600 dark:text-green-400 font-semibold text-base">
                Device paired!
              </p>
              <p className="text-sm text-on-surface-variant mt-1">
                {label || name} is now connected with{" "}
                <span className="text-on-surface font-medium">
                  {pairedRegistration.capabilities?.length ?? 0} capabilities
                </span>
                .
              </p>
            </div>
            {/* Zoomed-in QR as confirmation artefact */}
            <button
              onClick={() => setQrZoomed((z) => !z)}
              title="Toggle QR size"
              className="bg-white p-3 rounded-xl shadow-[0_0_0_2px_theme(colors.green.500/0.33)] transition-all duration-300 cursor-pointer hover:shadow-[0_0_0_3px_theme(colors.green.500/0.53)]"
            >
              <QRCode value={qrPayload} size={qrZoomed ? 220 : 120} />
            </button>
            <p className="text-[10px] text-on-surface-variant font-mono">{deviceId}</p>
            {pairedRegistration.capabilities && pairedRegistration.capabilities.length > 0 && (
              <div className="flex flex-wrap justify-center gap-1.5">
                {pairedRegistration.capabilities.map((cap) => (
                  <span
                    key={cap.name}
                    className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-green-500/10 text-green-600 dark:text-green-400 border border-green-500/30 dark:border-green-500/20"
                  >
                    {cap.name}
                  </span>
                ))}
              </div>
            )}
            <Button variant="primary" className="w-full" onClick={handleClose}>
              Done
            </Button>
          </div>
        ) : (
          /* ── Step 2: show QR, waiting for scan ── */
          <div className="flex flex-col items-center gap-5">
            <div className="bg-white p-4 rounded-xl">
              <QRCode value={qrPayload} size={200} />
            </div>
            <div className="flex items-center gap-2 text-xs text-on-surface-variant">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-cyan/60 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-cyan/80" />
              </span>
              Waiting for device to scan...
            </div>
            <p className="text-sm text-center text-on-surface-variant">
              Scan this QR code from the Companion App to securely pair it to your openCrow server.
            </p>
            <Button variant="secondary" className="w-full" onClick={handleClose}>
              Cancel
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
