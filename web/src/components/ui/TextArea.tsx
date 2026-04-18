"use client";

import { forwardRef, type TextareaHTMLAttributes } from "react";

interface TextAreaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
}

export const TextArea = forwardRef<HTMLTextAreaElement, TextAreaProps>(
  ({ label, error, rows = 4, className = "", ...rest }, ref) => {
    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label className="text-xs uppercase tracking-wider text-on-surface-variant font-mono">
            {label}
          </label>
        )}
        <textarea
          ref={ref}
          rows={rows}
          className={`bg-surface-high text-on-surface placeholder:text-on-surface-variant px-3 py-2 rounded text-sm font-body resize-y border-b-2 border-transparent focus:border-cyan focus:outline-none transition-colors duration-[120ms] ${error ? "border-error" : ""} ${className}`}
          {...rest}
        />
        {error && <span className="text-error text-xs">{error}</span>}
      </div>
    );
  }
);

TextArea.displayName = "TextArea";
