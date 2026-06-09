import { Tooltip as TooltipPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/utils";

const TooltipProvider = TooltipPrimitive.Provider;
const Tooltip = TooltipPrimitive.Root;
const TooltipTrigger = TooltipPrimitive.Trigger;

function TooltipContent({
  className,
  sideOffset = 6,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Content
      sideOffset={sideOffset}
      className={cn(
        "z-50 overflow-hidden rounded-md border border-hairline bg-surface-2 px-2 py-1 text-[11px] text-ink-muted shadow-md",
        "data-[state=delayed-open]:animate-in data-[state=closed]:animate-out",
        "data-[state=delayed-open]:fade-in-0 data-[state=closed]:fade-out-0",
        className,
      )}
      {...props}
    />
  );
}

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
