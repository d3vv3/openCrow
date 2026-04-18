"use client";

import { type ReactNode } from "react";

const variants = {
  success: "bg-success/20 text-success",
  warning: "bg-warning/20 text-warning",
  error: "bg-error/20 text-error",
  info: "bg-cyan/20 text-cyan",
  neutral: "bg-surface-highest text-on-surface-variant",
} as const;

interface BadgeProps {
  variant?: keyof typeof variants;
  children: ReactNode;
  dot?: boolean;
}

export function Badge({ variant = "neutral", children, dot = false }: BadgeProps) {
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2 py-0.5 rounded-sm ${variants[variant]}`}>
      {dot && <span className="h-1.5 w-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}
