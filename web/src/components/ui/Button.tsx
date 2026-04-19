"use client";

import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { Spinner } from "./Spinner";

const variants = {
  primary: "bg-violet text-white hover:shadow-[0_4px_20px_color-mix(in_srgb,var(--color-violet)_50%,transparent),0_2px_8px_color-mix(in_srgb,var(--color-violet)_30%,transparent)]",
  secondary: "bg-surface-highest text-violet-light hover:bg-surface-high",
  ghost: "bg-transparent hover:bg-surface-high text-on-surface",
  danger: "bg-red-500/10 hover:bg-red-500/20 text-red-400",
} as const;

const sizes = {
  sm: "px-3 py-1 text-xs",
  md: "px-4 py-2 text-sm",
  lg: "px-6 py-3 text-base",
} as const;

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants;
  size?: keyof typeof sizes;
  loading?: boolean;
  children: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "primary", size = "md", loading = false, disabled, children, className = "", ...rest },
  ref
) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center gap-2 rounded font-body font-medium transition-all duration-[120ms] cursor-pointer disabled:opacity-50 disabled:pointer-events-none ${variants[variant]} ${sizes[size]} ${className}`}
      {...rest}
    >
      {loading && <Spinner size="sm" />}
      {children}
    </button>
  );
});
