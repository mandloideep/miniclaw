import { Avatar as AvatarPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/utils";

function Avatar({ className, ...props }: React.ComponentProps<typeof AvatarPrimitive.Root>) {
  return (
    <AvatarPrimitive.Root
      className={cn(
        "relative flex h-7 w-7 shrink-0 overflow-hidden rounded-full bg-surface-2",
        className,
      )}
      {...props}
    />
  );
}

function AvatarImage({ className, ...props }: React.ComponentProps<typeof AvatarPrimitive.Image>) {
  return (
    <AvatarPrimitive.Image className={cn("aspect-square h-full w-full", className)} {...props} />
  );
}

function AvatarFallback({
  className,
  ...props
}: React.ComponentProps<typeof AvatarPrimitive.Fallback>) {
  return (
    <AvatarPrimitive.Fallback
      className={cn(
        "flex h-full w-full items-center justify-center bg-surface-2 text-[10px] font-medium text-ink-muted uppercase",
        className,
      )}
      {...props}
    />
  );
}

export { Avatar, AvatarFallback, AvatarImage };
