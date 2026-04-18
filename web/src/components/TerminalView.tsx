"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { endpoints, type UserConfig, getAccessToken } from "@/lib/api";
import { Button } from "@/components/ui/Button";

const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ||
  process.env.NEXT_PUBLIC_API_BASE_URL ||
  "http://localhost:8080";

function getWsBase(): string {
  return API_BASE.replace(/^http/, "ws");
}

export default function TerminalView() {
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const termContainerRef = useRef<HTMLDivElement>(null);
  // Store terminal + ws in refs to avoid stale closures
  const xtermRef = useRef<import("@xterm/xterm").Terminal | null>(null);
  const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  // Generation counter: incremented on cleanup so stale async connects abort
  const genRef = useRef(0);

  const connectTerminal = useCallback(async () => {
    if (!termContainerRef.current) return;
    setError(null);

    // Capture generation; if cleanup runs while we await, we bail out
    const gen = ++genRef.current;

    // Dynamically import xterm (avoids SSR issues)
    const { Terminal } = await import("@xterm/xterm");
    const { FitAddon } = await import("@xterm/addon-fit");
    const { WebLinksAddon } = await import("@xterm/addon-web-links");

    // If unmounted/re-mounted while awaiting imports, bail
    if (gen !== genRef.current || !termContainerRef.current) return;

    // Destroy existing terminal if reconnecting
    if (xtermRef.current) {
      xtermRef.current.dispose();
      xtermRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    if (termContainerRef.current) {
      termContainerRef.current.innerHTML = "";
    }

    const term = new Terminal({
      theme: {
        background: "#0d0d1a",
        foreground: "#f8f8f2",
        cursor: "#f8f8f2",
        selectionBackground: "#44475a",
        black: "#000000",
        brightBlack: "#555555",
        red: "#ff5555",
        brightRed: "#ff5555",
        green: "#50fa7b",
        brightGreen: "#50fa7b",
        yellow: "#f1fa8c",
        brightYellow: "#f1fa8c",
        blue: "#6272a4",
        brightBlue: "#6272a4",
        magenta: "#ff79c6",
        brightMagenta: "#ff79c6",
        cyan: "#8be9fd",
        brightCyan: "#8be9fd",
        white: "#bfbfbf",
        brightWhite: "#ffffff",
      },
      fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", monospace',
      fontSize: 13,
      lineHeight: 1.4,
      cursorBlink: true,
      scrollback: 10000,
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());
    xtermRef.current = term;
    fitAddonRef.current = fitAddon;

    term.open(termContainerRef.current);
    fitAddon.fit();

    // Connect WebSocket
    const token = getAccessToken();
    const wsUrl = `${getWsBase()}/v1/terminal/ws${token ? `?token=${encodeURIComponent(token)}` : ""}`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setError(null);
      // Send initial terminal size
      const dims = fitAddon.proposeDimensions();
      if (dims) {
        ws.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
      }
    };

    ws.onmessage = (ev) => {
      if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data));
      } else {
        term.write(ev.data as string);
      }
    };

    ws.onerror = () => {
      setError("WebSocket connection error");
      setConnected(false);
    };

    ws.onclose = () => {
      setConnected(false);
      term.write("\r\n\x1b[31m[connection closed]\x1b[0m\r\n");
    };

    // Keystrokes -> PTY
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Resize -> PTY
    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols, rows }));
      }
    });
  }, []);

  // Initial mount: open terminal
  useEffect(() => {
    connectTerminal();

    return () => {
      genRef.current++;  // invalidate any in-flight connectTerminal
      wsRef.current?.close();
      xtermRef.current?.dispose();
      wsRef.current = null;
      xtermRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Handle container resize via ResizeObserver
  useEffect(() => {
    if (!termContainerRef.current) return;
    const obs = new ResizeObserver(() => {
      if (fitAddonRef.current && xtermRef.current) {
        try {
          fitAddonRef.current.fit();
        } catch { /* ignore */ }
      }
    });
    obs.observe(termContainerRef.current);
    return () => obs.disconnect();
  }, []);

  return (
    <div className="flex flex-col gap-4 h-[calc(100vh-64px)]">
      {/* Terminal window */}
      <div className="flex-1 flex flex-col rounded-lg overflow-hidden border border-[#2a2a3e] min-h-0">
        {/* Title bar */}
        <div className="shrink-0 flex items-center gap-3 bg-[#1a1a2e] px-4 py-2">
          <div className="flex gap-1.5">
            <span className="block h-3 w-3 rounded-full bg-[#ff5f57]" />
            <span className="block h-3 w-3 rounded-full bg-[#ffbd2e]" />
            <span className="block h-3 w-3 rounded-full bg-[#28c840]" />
          </div>
          <span className="text-xs font-mono text-[#8888aa]">
            openCrow@sandbox &mdash; /bin/bash
          </span>
          <div className="ml-auto flex items-center gap-3">
            {connected ? (
              <span className="flex items-center gap-1.5 text-xs font-mono text-[#50fa7b]">
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-[#50fa7b] opacity-60" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-[#50fa7b]" />
                </span>
                connected
              </span>
            ) : (
              <span className="text-xs font-mono text-[#ff5555]">disconnected</span>
            )}
            <button
              onClick={connectTerminal}
              className="text-xs font-mono text-[#8888aa] hover:text-[#f8f8f2] transition-colors px-2 py-0.5 rounded border border-[#2a2a3e] hover:border-[#6272a4]"
            >
              reconnect
            </button>
          </div>
        </div>

        {/* xterm container */}
        <div className="flex-1 bg-[#0d0d1a] min-h-0 overflow-hidden p-1">
          {error && (
            <div className="text-[#ff5555] font-mono text-xs px-3 py-2 border-b border-[#2a2a3e]">
              {error}
            </div>
          )}
          <div
            ref={termContainerRef}
            className="h-full w-full"
            style={{ height: "100%", width: "100%" }}
          />
        </div>
      </div>
    </div>
  );
}
