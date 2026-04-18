"use client";

import { forwardRef, type InputHTMLAttributes } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ label, error, className = "", ...rest }, ref) => {
    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label className="text-xs uppercase tracking-wider text-on-surface-variant font-mono">
            {label}
          </label>
        )}
        <input
          ref={ref}
          className={`bg-surface-high text-on-surface placeholder:text-on-surface-variant px-3 py-2 rounded text-sm font-body border-b-2 border-transparent focus:border-cyan focus:outline-none transition-colors duration-[120ms] ${error ? "border-error" : ""} ${className}`}
          {...rest}
        />
        {error && <span className="text-error text-xs">{error}</span>}
      </div>
    );
  }
);

Input.displayName = "Input";
