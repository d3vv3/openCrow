"use client";

import { useState, type ReactNode } from "react";

interface TooltipProps {
  text: string;
  children?: ReactNode;
}

export function Tooltip({ text, children }: TooltipProps) {
  const [show, setShow] = useState(false);

  return (
    <span
      className="relative inline-flex items-center"
      onMouseEnter={() => setShow(true)}
      onMouseLeave={() => setShow(false)}
    >
      {children ?? (
        <span className="ml-1 cursor-help text-[10px] text-on-surface-variant border border-on-surface-variant rounded-full w-3.5 h-3.5 inline-flex items-center justify-center leading-none">
          ?
        </span>
      )}
      {show && (
        <span className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 text-xs rounded bg-surface-high text-on-surface shadow-lg whitespace-nowrap z-50 pointer-events-none">
          {text}
        </span>
      )}
    </span>
  );
}
