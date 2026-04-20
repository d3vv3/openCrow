"use client";

import { type ReactNode } from "react";

interface SectionHeaderProps {
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}

export function SectionHeader({ title, description, action }: SectionHeaderProps) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div>
        <h2 className="font-display text-2xl font-semibold text-on-surface">{title}</h2>
        {description && <p className="text-on-surface-variant text-base mt-1">{description}</p>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}
