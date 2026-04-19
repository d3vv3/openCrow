"use client";

import { useState, useEffect } from "react";
import type { EmailAccountConfig, UserConfig } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Toggle } from "@/components/ui/Toggle";
import { Button } from "@/components/ui/Button";
import { AnimatedDot } from "@/components/ui/AnimatedDot";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { SaveBar } from "./SaveBar";
import type { UpdateConfigFn } from "./types";

function EmailAccountCard({
  acct,
  index: i,
  updateConfig,
  setError,
}: {
  acct: EmailAccountConfig;
  index: number;
  updateConfig: UpdateConfigFn;
  setError: (e: string | null) => void;
}) {
  const configured = !!(acct.imapHost && acct.imapUsername);
  const [expanded, setExpanded] = useState(!configured);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; error?: string; detail?: string } | null>(null);
  const [autoconfStatus, setAutoconfStatus] = useState<string | null>(null);

  const handleTest = async (e?: React.MouseEvent) => {
    e?.stopPropagation();
    setTesting(true);
    setTestResult(null);
    try {
      const res = await endpoints.testEmailConnection({
        imapHost: acct.imapHost,
        imapPort: acct.imapPort,
        username: acct.imapUsername ?? "",
        password: acct.imapPassword ?? "",
        useTls: acct.tls,
      });
      setTestResult(res);
    } catch {
      setTestResult({ ok: false, error: "Request failed" });
    } finally {
      setTesting(false);
    }
  };

  const runAutoconfig = async (email: string) => {
    if (!email.includes("@")) return;
    setAutoconfStatus("looking up…");
    try {
      const res = await endpoints.emailAutoconfig(email);
      if (res.imapHost) {
        updateConfig((c) => {
          const a = c.integrations.emailAccounts[i];
          a.imapHost = res.imapHost!;
          a.imapPort = res.imapPort ?? 993;
          if (!a.imapUsername) a.imapUsername = res.imapUsername ?? email;
          a.smtpHost = res.smtpHost ?? "";
          a.smtpPort = res.smtpPort ?? 587;
          if (res.useTls !== undefined) a.tls = res.useTls;
          return c;
        });
        setAutoconfStatus(`auto-configured via ${res.source}`);
      } else {
        setAutoconfStatus(null);
      }
    } catch {
      setAutoconfStatus(null);
    }
  };

  // Auto-test when tab opens (component mounts with IMAP credentials)
  useEffect(() => {
    if (acct.imapHost && acct.imapUsername) handleTest();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
      {/* Header row */}
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        <AnimatedDot status={testing ? "pending" : testResult ? (testResult.ok ? "ok" : "error") : (acct.enabled ? "ok" : "idle")} />
        <span className="font-medium text-sm flex-1 truncate">{acct.label || `Account ${i + 1}`}</span>
        {acct.address && <span className="text-xs text-on-surface-variant truncate hidden sm:block">{acct.address}</span>}
        {testResult && (
          <span className={`flex items-center gap-1.5 text-xs font-mono px-2 py-0.5 rounded ${testResult.ok ? "text-cyan bg-cyan/10" : "text-error bg-error/10"}`}>
            <AnimatedDot status={testResult.ok ? "ok" : "error"} />
            {testResult.ok ? "OK" : "FAIL"}
          </span>
        )}
        <svg className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${expanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-white/10 pt-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input label="Label" value={acct.label} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].label = e.target.value; return c; })} />
            <Input label="Address" value={acct.address} onChange={(e) => updateConfig((c) => {
              const newAddr = e.target.value;
              const prev = c.integrations.emailAccounts[i];
              // Auto-fill IMAP username if it's blank or still mirrors the old address
              if (!prev.imapUsername || prev.imapUsername === prev.address) {
                prev.imapUsername = newAddr;
              }
              prev.address = newAddr;
              return c;
            })} onBlur={(e) => { if (!acct.imapHost) runAutoconfig(e.target.value); }} />
            <Input label="IMAP Host" value={acct.imapHost} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapHost = e.target.value; return c; })} />
            <Input label="IMAP Port" type="number" value={acct.imapPort} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapPort = parseInt(e.target.value) || 0; return c; })} />
            <Input label="IMAP Username" value={acct.imapUsername ?? ""} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapUsername = e.target.value; return c; })} />
            <Input label="IMAP Password" type="password" value={acct.imapPassword ?? ""} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapPassword = e.target.value; return c; })} />
            <Input label="SMTP Host" value={acct.smtpHost} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].smtpHost = e.target.value; return c; })} />
            <Input label="SMTP Port" type="number" value={acct.smtpPort} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].smtpPort = parseInt(e.target.value) || 0; return c; })} />
            <Input label="Poll Interval (seconds)" type="number" value={acct.pollIntervalSeconds || 900} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].pollIntervalSeconds = parseInt(e.target.value) || 900; return c; })} />
          </div>
          <div className="flex items-center gap-6">
            <Toggle label="TLS" checked={acct.tls} onChange={(v) => updateConfig((c) => { c.integrations.emailAccounts[i].tls = v; return c; })} />
            <Toggle label="Enabled" checked={acct.enabled} onChange={(v) => updateConfig((c) => { c.integrations.emailAccounts[i].enabled = v; return c; })} />
            {autoconfStatus && <span className="text-xs text-cyan font-mono ml-auto">{autoconfStatus}</span>}
          </div>
          {testResult && !testResult.ok && (
            <p className="text-xs text-error font-mono">{testResult.error}</p>
          )}
          {testResult?.ok && testResult.detail && (
            <p className="text-xs text-cyan font-mono">{testResult.detail}</p>
          )}
          <div className="flex gap-2 pt-2">
            <Button variant="secondary" size="sm" loading={testing} onClick={handleTest}>
              Test connection
            </Button>
            <Button variant="ghost" size="sm" className="ml-auto hover:text-error" onClick={() => updateConfig((c) => { c.integrations.emailAccounts.splice(i, 1); return c; })}>
              Remove
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export function EmailTab({
  config,
  updateConfig,
  saving,
  saveFullConfig,
  saveStatus,
  setError,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveStatus: string | null;
  setError: (e: string | null) => void;
}) {
  return (
    <div className="space-y-6">
      <SectionHeader
        title="Email Accounts"
        description="Manage connected email accounts for IMAP polling"
        action={
          <Button variant="secondary" size="sm" onClick={() => updateConfig((c) => {
            c.integrations.emailAccounts.push({
              label: "", address: "", imapHost: "", imapPort: 993,
              imapUsername: "", imapPassword: "",
              smtpHost: "", smtpPort: 587, tls: true, enabled: true,
              pollIntervalSeconds: 900,
            });
            return c;
          })}>
            Add Account
          </Button>
        }
      />
      {config.integrations.emailAccounts.map((acct, i) => (
        <EmailAccountCard key={i} acct={acct} index={i} updateConfig={updateConfig} setError={setError} />
      ))}
      {config.integrations.emailAccounts.length === 0 && (
        <p className="text-on-surface-variant text-sm">No email accounts configured.</p>
      )}
      <SaveBar onClick={saveFullConfig} loading={saving} label="Save Email Config" status={saveStatus} />
    </div>
  );
}
