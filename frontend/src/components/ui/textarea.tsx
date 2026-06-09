import type * as React from "react";

import { cn } from "@/lib/utils";

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex min-h-24 w-full rounded-md border border-hairline bg-surface-1 px-3 py-2 text-sm text-ink shadow-none outline-none placeholder:text-ink-tertiary",
        "focus-visible:border-brand-focus focus-visible:ring-2 focus-visible:ring-brand-focus/40",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };
