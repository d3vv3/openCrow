import { formatTime, isUuid } from "./helpers";
import { MarkdownMessage } from "./MarkdownMessage";
import { formatAttachmentSize } from "./attachments";
import { FileIcon, CopyIcon, RegenIcon, SparkleIcon, UserIcon } from "@/components/ui/icons";
import type { MessageDTO } from "@/lib/api";

type MessageItemProps = {
  msg: MessageDTO;
  streamingMsgId: string | null;
  regeneratingId: string | null;
  copiedId: string | null;
  onCopy: (id: string, content: string) => void;
  onRegenerate: (id: string) => void;
};

export function MessageItem({
  msg,
  streamingMsgId,
  regeneratingId,
  copiedId,
  onCopy,
  onRegenerate,
}: MessageItemProps) {
  if (msg.role === "system") {
    const isError =
      msg.content.startsWith("Failed") ||
      msg.content.toLowerCase().includes("error") ||
      msg.id.startsWith("err-");
    return (
      <div className="text-center animate-in fade-in slide-in-from-bottom-1 duration-300">
        <p className={`text-xs font-mono ${isError ? "text-error" : "text-on-surface-variant"}`}>
          {msg.content}
        </p>
      </div>
    );
  }

  const isUser = msg.role === "user";
  const canRegenerate = msg.role === "assistant" && isUuid(msg.id) && !msg.id.startsWith("stream-");

  return (
    <div
      className={`flex group items-start gap-3 ${isUser ? "flex-row-reverse" : "flex-row"} animate-in fade-in slide-in-from-bottom-2 duration-300`}
    >
      {/* Avatar */}
      {isUser ? (
        <div className="hidden sm:flex shrink-0 mt-1 h-11 w-11 rounded-full bg-violet items-center justify-center shadow-[0_0_18px_rgba(124,58,237,0.75)]">
          <UserIcon className="text-white w-5 h-5" />
        </div>
      ) : (
        <div className="hidden sm:flex shrink-0 mt-1 h-11 w-11 rounded-full bg-[#0d0d1a] border border-white/10 items-center justify-center shadow-[0_0_12px_rgba(0,0,0,0.5)]">
          <SparkleIcon className="text-violet-light w-5 h-5" />
        </div>
      )}

      {/* Bubble */}
      <div
        className={`max-w-full sm:max-w-[78%] rounded-2xl px-4 py-3 ${
          isUser
            ? "bg-violet text-white shadow-[0_6px_28px_rgba(124,58,237,0.38)]"
            : "bg-surface-high border border-white/8 shadow-sm"
        }`}
      >
        <div
          className={`text-base font-body break-words ${isUser ? "text-white" : "text-on-surface"}`}
        >
          {msg.role === "assistant" && msg.id === streamingMsgId && msg.content === "" ? (
            <div className="flex items-center gap-2 py-0.5">
              {[3, 1.5, 2].map((w, i) => (
                <div
                  key={i}
                  className="h-[1em] rounded bg-on-surface-variant/20 animate-pulse"
                  style={{ width: `${w}rem`, animationDelay: `${i * 150}ms` }}
                />
              ))}
            </div>
          ) : msg.role === "assistant" ? (
            <MarkdownMessage content={msg.content} />
          ) : (
            <MarkdownMessage content={msg.content} compact />
          )}

          {!!msg.attachments?.length && (
            <div className="mt-3 grid gap-2">
              {msg.attachments.map((att, index) => {
                const isImage = att.mimeType?.startsWith("image/");
                return (
                  <a
                    key={att.id || `${att.fileName}-${index}`}
                    href={att.dataUrl}
                    download={att.fileName}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`rounded-lg border px-3 py-2 text-xs hover:opacity-80 transition-opacity overflow-hidden ${isUser ? "border-white/20 bg-white/10" : "border-white/10 bg-surface-mid"}`}
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      {isImage ? (
                        <span className="font-mono opacity-70">IMG</span>
                      ) : (
                        <FileIcon className="h-4 w-4 opacity-70" aria-hidden="true" />
                      )}
                      <span className="min-w-0 flex-1 truncate font-medium">{att.fileName}</span>
                      <span className="ml-auto shrink-0 max-w-[45%] truncate text-right opacity-60 font-mono">
                        {[att.mimeType, formatAttachmentSize(att.sizeBytes)]
                          .filter(Boolean)
                          .join(" · ")}
                      </span>
                    </div>
                  </a>
                );
              })}
            </div>
          )}
        </div>

        {/* Timestamp + actions */}
        <div
          className={`flex items-center gap-0.5 mt-2 ${isUser ? "justify-start" : "justify-between"}`}
        >
          <span
            className={`text-[10px] font-mono ${isUser ? "text-white/50" : "text-on-surface-variant/60"}`}
          >
            {formatTime(msg.createdAt)}
          </span>
          {!isUser && (
            <div className="flex items-center gap-0.5 ml-auto">
              <button
                onClick={() => msg.content && onCopy(msg.id, msg.content)}
                disabled={!msg.content}
                className="flex items-center gap-1 text-xs text-on-surface-variant hover:text-on-surface px-1.5 py-0.5 rounded hover:cursor-pointer transition-colors font-mono disabled:opacity-30 disabled:cursor-default"
                title="Copy"
              >
                {copiedId === msg.id ? <span className="text-[10px]">copied</span> : <CopyIcon />}
              </button>
              {canRegenerate && (
                <button
                  onClick={() => onRegenerate(msg.id)}
                  disabled={!!regeneratingId || !msg.content}
                  className="flex items-center gap-1 text-xs text-on-surface-variant hover:text-cyan px-1.5 py-0.5 rounded hover:cursor-pointer transition-colors font-mono disabled:opacity-30 disabled:cursor-default"
                  title="Regenerate"
                >
                  <RegenIcon className={regeneratingId === msg.id ? "animate-spin" : ""} />
                </button>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
