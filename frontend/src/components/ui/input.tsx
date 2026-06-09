import type * as React from "react";

import { cn } from "@/lib/utils";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "flex h-9 w-full min-w-0 rounded-md border border-hairline bg-surface-1 px-3 py-1 text-sm text-ink shadow-none transition-colors outline-none placeholder:text-ink-tertiary",
        "selection:bg-brand/40 selection:text-ink",
        "focus-visible:border-brand-focus focus-visible:ring-2 focus-visible:ring-brand-focus/40",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "aria-invalid:border-danger aria-invalid:ring-danger/20",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
