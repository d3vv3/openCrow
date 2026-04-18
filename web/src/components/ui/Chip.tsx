"use client";

import { type ReactNode } from "react";

interface ChipProps {
  children: ReactNode;
  onRemove?: () => void;
  className?: string;
}

export function Chip({ children, onRemove, className = "" }: ChipProps) {
  return (
    <span className={`inline-flex items-center gap-1 bg-surface-highest text-on-surface-variant text-xs px-2 py-0.5 rounded-sm ${className}`}>
      {children}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          className="hover:text-on-surface transition-colors duration-[120ms] ml-0.5"
          aria-label="Remove"
        >
          *
        </button>
      )}
    </span>
  );
}
