import { useState } from "react";
import { formatTime, isUuid, displayValue } from "./helpers";
import { MarkdownMessage } from "./MarkdownMessage";
import { ToolIcon } from "@/components/ui/icons";
import type { MessageDTO, ToolCallRecord } from "@/lib/api";

export function ToolCallItem({ tc, isLive }: { tc: ToolCallRecord; isLive: boolean }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const toggleExpand = () => setIsExpanded((v) => !v);

  const toolKind = tc.kind === "MCP" ? "MCP" : "TOOL";

  // Extract the primary "command" arg (command, query, content, prompt, action -- first string arg)
  const args = tc.arguments ?? {};
  const primaryArgKeys = [
    "command",
    "query",
    "content",
    "prompt",
    "action",
    "url",
    "messageId",
    "memoryId",
    "taskId",
  ];
  const primaryKey = primaryArgKeys.find((k) => k in args) ?? Object.keys(args)[0];
  const primaryVal = primaryKey ? String(args[primaryKey] ?? "") : "";

  // Parse stdout from output (output may be JSON with stdout/output/result fields, or raw string)
  let stdout = "";
  let stdoutIsJson = false;
  if (tc.output != null) {
    const raw = String(tc.output);
    try {
      const parsed = JSON.parse(raw);
      const extracted = parsed.stdout ?? parsed.output ?? parsed.result;
      if (extracted !== undefined) {
        // Extracted field may itself be an object (avoid [object Object])
        const rendered = displayValue(extracted);
        stdout = rendered.text;
        stdoutIsJson = rendered.isJson;
      } else {
        stdout = JSON.stringify(parsed, null, 2);
        stdoutIsJson = true;
      }
    } catch {
      stdout = raw;
    }
  }

  return (
    <div className="flex justify-center animate-in fade-in duration-300">
      <button
        onClick={toggleExpand}
        className="w-full max-w-[90%] text-left rounded-lg border border-[#2a2a3e] bg-[#0d0d1a] hover:border-[#3a3a5e] transition-colors font-mono text-xs overflow-hidden"
      >
        {/* Compact header -- always visible */}
        <div className="flex items-center gap-2 px-3 py-1.5">
          <ToolIcon className="text-[#6272a4] shrink-0 w-3.5 h-3.5" />
          <span
            className={`shrink-0 text-[10px] px-1.5 py-0.5 rounded border font-semibold ${toolKind === "MCP" ? "text-violet border-violet/40 bg-violet/10" : "text-cyan border-cyan/40 bg-cyan/10"}`}
          >
            [{toolKind}]
          </span>
          <span className="text-[#8be9fd] shrink-0">{tc.toolName}</span>
          {primaryVal && (
            <span className="text-[#f8f8f2]/60 truncate flex-1">{primaryVal.slice(0, 80)}</span>
          )}
          {!isLive &&
            (tc.error ? (
              <span className="shrink-0 ml-auto text-[10px] px-1.5 py-0.5 rounded-full font-semibold bg-[#ff5555]/15 text-[#ff5555] border border-[#ff5555]/30">
                failed
              </span>
            ) : (
              <span className="shrink-0 ml-auto text-[10px] px-1.5 py-0.5 rounded-full font-semibold bg-[#50fa7b]/15 text-[#50fa7b] border border-[#50fa7b]/30">
                ok
              </span>
            ))}
          <span className="text-[#6272a4] shrink-0">{formatTime(tc.createdAt)}</span>
          {isLive && <span className="text-[#6272a4] animate-pulse shrink-0 ml-auto">...</span>}
          <span className="text-[#6272a4] shrink-0 ml-1">{isExpanded ? "▲" : "▼"}</span>
        </div>

        {/* Expanded body */}
        {isExpanded && (
          <div className="border-t border-[#2a2a3e] px-3 py-2 space-y-2">
            {/* All args */}
            {Object.keys(args).length > 0 && (
              <div className="space-y-1">
                {Object.entries(args).map(([k, v]) => {
                  const isObj = typeof v === "object" && v !== null;
                  const prettyVal = isObj ? JSON.stringify(v, null, 2) : String(v);
                  const looksLikeJson =
                    !isObj &&
                    (() => {
                      try {
                        JSON.parse(String(v));
                        return true;
                      } catch {
                        return false;
                      }
                    })();
                  const displayVal = looksLikeJson
                    ? JSON.stringify(JSON.parse(String(v)), null, 2)
                    : prettyVal;
                  const multiline = displayVal.includes("\n");
                  return (
                    <div key={k}>
                      <span className="text-[#f1fa8c]">{k}</span>
                      <span className="text-[#6272a4]">=</span>
                      {multiline ? (
                        <pre className="text-[#50fa7b] whitespace-pre-wrap break-all mt-0.5 pl-2 border-l border-[#6272a4]/30">
                          {displayVal}
                        </pre>
                      ) : (
                        <span className="text-[#50fa7b] break-all ml-1">{displayVal}</span>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
            {/* Stdout / output */}
            {stdout && (
              <pre
                className={`whitespace-pre-wrap break-all leading-relaxed ${stdoutIsJson ? "text-[#f8f8f2]/80 bg-black/20 rounded p-2" : "text-[#50fa7b] opacity-80"}`}
              >
                {stdout}
              </pre>
            )}
            {/* Error */}
            {tc.error && (
              <pre className="text-[#ff5555] whitespace-pre-wrap break-all">{tc.error}</pre>
            )}
          </div>
        )}
      </button>
    </div>
  );
}

export function MessageItem({
  msg,
  streamingMsgId,
  regeneratingId,
  copiedId,
  onCopy,
  onRegenerate,
}: {
  msg: MessageDTO;
  streamingMsgId: string | null;
  regeneratingId: string | null;
  copiedId: string | null;
  onCopy: (id: string, content: string) => void;
  onRegenerate: (id: string) => void;
}) {
  if (msg.role === "system") {
    return (
      <div className="text-center animate-in fade-in slide-in-from-bottom-1 duration-300">
        <p className="text-xs text-on-surface-variant font-mono">{msg.content}</p>
      </div>
    );
  }

  const isUser = msg.role === "user";
  const canRegenerate = msg.role === "assistant" && isUuid(msg.id) && !msg.id.startsWith("stream-");

  return (
    <div
      className={`flex group ${isUser ? "justify-end" : "justify-start"} animate-in fade-in slide-in-from-bottom-2 duration-300`}
    >
      {!isUser && (
        <div className="shrink-0 mt-3 mr-2">
          <span className="block h-2 w-2 rounded-full bg-cyan" />
        </div>
      )}
      <div
        className={`max-w-[70%] rounded-lg p-4 border ${isUser ? "bg-violet/5 border-violet/20" : "bg-surface-high border-outline-ghost"}`}
      >
        <p className="flex items-center gap-1.5 text-xs uppercase tracking-wider text-on-surface-variant font-mono mb-1">
          {msg.role}
          <span className="inline-block h-1 w-1 rounded-full bg-on-surface-variant/50" />
          {formatTime(msg.createdAt)}
        </p>
        <div className="text-sm text-on-surface font-body break-words">
          {msg.role === "assistant" && msg.id === streamingMsgId && msg.content === "" ? (
            <div className="space-y-2 py-0.5">
              <div className="h-3 rounded bg-on-surface-variant/15 animate-pulse w-3/4" />
              <div className="h-3 rounded bg-on-surface-variant/10 animate-pulse w-1/2" />
              <div className="h-3 rounded bg-on-surface-variant/8 animate-pulse w-2/3" />
            </div>
          ) : msg.role === "assistant" ? (
            <MarkdownMessage content={msg.content} />
          ) : (
            <MarkdownMessage content={msg.content} compact />
          )}
        </div>
        {!isUser && (
          <div className="flex items-center justify-end gap-1 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
            <button
              onClick={() => onCopy(msg.id, msg.content)}
              className="text-xs text-on-surface-variant hover:text-on-surface px-1.5 py-0.5 rounded hover:bg-white/5 transition-colors font-mono"
              title="Copy"
            >
              {copiedId === msg.id ? "copied" : "copy"}
            </button>
            {canRegenerate && (
              <button
                onClick={() => onRegenerate(msg.id)}
                disabled={!!regeneratingId}
                className="text-xs text-on-surface-variant hover:text-cyan px-1.5 py-0.5 rounded hover:bg-white/5 transition-colors font-mono disabled:opacity-40"
                title="Regenerate"
              >
                {regeneratingId === msg.id ? "..." : "↺ regen"}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
