import { FlagIcon } from "./FlagIcon";
import { automationLabel } from "./helpers";
import type { ConversationDTO, MessageDTO } from "@/lib/api";

export function AutomationBanner({
  conversation,
  messages,
}: {
  conversation: ConversationDTO;
  messages: MessageDTO[];
}) {
  const prompt =
    messages.find((m) => m.role === "user")?.content ||
    (conversation.title ?? "").replace(/^(Scheduled task|Heartbeat|Automatic):\s*/i, "") ||
    "No prompt";

  return (
    <div className="shrink-0 px-6 pt-5">
      <div className="mx-auto flex max-w-xl items-start gap-4 rounded-2xl border border-violet/20 bg-surface-lowest/80 px-5 py-4 backdrop-blur-2xl shadow-[var(--shadow-float)]">
        <div className="mt-0.5 inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-violet/15 text-violet">
          <FlagIcon className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-semibold text-on-surface">
              {automationLabel(conversation.automationKind)}
            </p>
            <span className="inline-flex items-center whitespace-nowrap rounded-full border border-violet/30 bg-violet/10 px-2 py-0.5 text-[10px] font-mono uppercase tracking-[0.18em] text-violet">
              Auto
            </span>
          </div>
          <p className="mt-1 text-sm text-on-surface-variant break-words w-full">{prompt}</p>
        </div>
      </div>
    </div>
  );
}
