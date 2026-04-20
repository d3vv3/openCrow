"use client";

import QRCode from "react-qr-code";
import { Button } from "@/components/ui/Button";

export function DeviceQRModal({ payload, onClose }: { payload: string; onClose: () => void }) {
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
        <div className="flex flex-col items-center gap-6">
          <div className="bg-white p-4 rounded-xl">
            <QRCode value={payload} size={200} />
          </div>
          <p className="text-sm text-center text-on-surface-variant">
            Scan this QR code from the Companion App to securely pair it to your openCrow server.
          </p>
          <Button variant="secondary" className="w-full" onClick={onClose}>
            Done
          </Button>
        </div>
      </div>
    </div>
  );
}
