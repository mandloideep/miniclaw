import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium leading-none tracking-[0.02em] whitespace-nowrap",
  {
    variants: {
      variant: {
        default: "border-hairline bg-surface-2 text-ink-muted",
        accent: "border-transparent bg-brand/15 text-brand",
        success: "border-transparent bg-success/15 text-success",
        outline: "border-hairline-strong text-ink-subtle",
        muted: "border-transparent bg-surface-2 text-ink-subtle",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

function Badge({
  className,
  variant = "default",
  ...props
}: React.ComponentProps<"span"> & VariantProps<typeof badgeVariants>) {
  return (
    <span data-slot="badge" className={cn(badgeVariants({ variant, className }))} {...props} />
  );
}

export { Badge, badgeVariants };
