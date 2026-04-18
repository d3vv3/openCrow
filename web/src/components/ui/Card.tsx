"use client";

import { type ReactNode } from "react";

interface CardProps {
  title?: string;
  subtitle?: string;
  children: ReactNode;
  className?: string;
}

export function Card({ title, subtitle, children, className = "" }: CardProps) {
  return (
    <div className={`bg-surface-mid rounded p-6 ${className}`}>
      {title && <h3 className="font-display text-lg font-semibold text-on-surface">{title}</h3>}
      {subtitle && <p className="text-on-surface-variant text-sm mt-1">{subtitle}</p>}
      {(title || subtitle) && <div className="mt-4" />}
      {children}
    </div>
  );
}
