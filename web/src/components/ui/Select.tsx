"use client";

import { forwardRef, type SelectHTMLAttributes } from "react";

interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  options: SelectOption[];
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ label, options, className = "", ...rest }, ref) => {
    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label className="text-xs uppercase tracking-wider text-on-surface-variant font-mono">
            {label}
          </label>
        )}
        <select
          ref={ref}
          className={`bg-surface-high text-on-surface px-3 py-2 rounded text-sm font-body border-b-2 border-transparent focus:border-cyan focus:outline-none transition-colors duration-[120ms] appearance-none ${className}`}
          {...rest}
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
    );
  }
);

Select.displayName = "Select";
