"use client";

import { useState } from "react";
import QRCode from "react-qr-code";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import type { UpdateConfigFn } from "./types";

export function AddDeviceModal({
  serverUrl,
  updateConfig,
  saveFullConfig,
  onClose,
}: {
  serverUrl: string;
  updateConfig: UpdateConfigFn;
  saveFullConfig: () => void;
  onClose: () => void;
}) {
  const [name, setName] = useState("");
  const [label, setLabel] = useState("");
  const [generating, setGenerating] = useState(false);
  const [qrPayload, setQrPayload] = useState<string | null>(null);

  const generate = async () => {
    const trimName = name.trim();
    const trimLabel = label.trim() || trimName;
    if (!trimName) return;
    setGenerating(true);
    try {
      const res = await endpoints.createDeviceTokens(trimLabel);
      const deviceId = "dev_" + Math.random().toString(36).substring(2, 9);
      setQrPayload(
        JSON.stringify({
          id: deviceId,
          server: serverUrl,
          accessToken: res.tokens.accessToken,
          refreshToken: res.tokens.refreshToken,
        }),
      );
      updateConfig((c) => {
        if (!c.integrations.companionApps) c.integrations.companionApps = [];
        c.integrations.companionApps.push({
          id: deviceId,
          name: trimName,
          label: trimLabel,
          enabled: true,
        });
        return c;
      });
      saveFullConfig();
    } catch (e) {
      console.error(e);
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-surface-high border border-white/10 rounded-2xl p-6 w-full max-w-sm shadow-2xl flex flex-col gap-6">
        <div className="flex justify-between items-center">
          <h3 className="text-lg font-medium text-on-surface">Pair Companion App</h3>
          <button onClick={onClose} className="text-on-surface-variant hover:text-on-surface p-1">
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
        ) : (
          <div className="flex flex-col items-center gap-6">
            <div className="bg-white p-4 rounded-xl">
              <QRCode value={qrPayload} size={200} />
            </div>
            <p className="text-sm text-center text-on-surface-variant">
              Scan this QR code from the Companion App to securely pair it to your openCrow server.
            </p>
            <Button variant="secondary" className="w-full" onClick={onClose}>
              Done
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
