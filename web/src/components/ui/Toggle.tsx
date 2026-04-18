"use client";

interface ToggleProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label?: string;
  disabled?: boolean;
}

export function Toggle({ checked, onChange, label, disabled = false }: ToggleProps) {
  return (
    <label className={`inline-flex items-center gap-2 ${disabled ? "opacity-50 pointer-events-none" : "cursor-pointer"}`}>
      {label && <span className="text-sm text-on-surface">{label}</span>}
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={`relative w-10 h-[22px] rounded-full transition-colors duration-[120ms] ${checked ? "bg-cyan" : "bg-surface-highest"}`}
      >
        <span
          className={`absolute top-[3px] left-[3px] h-4 w-4 rounded-full bg-white transition-transform duration-[120ms] ${checked ? "translate-x-[18px]" : ""}`}
        />
      </button>
    </label>
  );
}
