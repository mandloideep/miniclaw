import { Label as LabelPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/utils";

function Label({ className, ...props }: React.ComponentProps<typeof LabelPrimitive.Root>) {
  return (
    <LabelPrimitive.Root
      className={cn(
        "text-[12px] font-medium leading-none text-ink-muted",
        "peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
        className,
      )}
      {...props}
    />
  );
}

export { Label };
