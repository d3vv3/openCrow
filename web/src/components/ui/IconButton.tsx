"use client";

import { type ButtonHTMLAttributes } from "react";

type IconButtonProps = ButtonHTMLAttributes<HTMLButtonElement>;

export function IconButton({ children, className = "", ...rest }: IconButtonProps) {
  return (
    <button
      className={`bg-transparent hover:bg-surface-high rounded p-2 text-on-surface-variant hover:text-on-surface transition-colors duration-[120ms] disabled:opacity-50 disabled:pointer-events-none ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
}
