import type * as React from "react";

import { cn } from "@/lib/utils";

function Separator({
  className,
  orientation = "horizontal",
  ...props
}: React.ComponentProps<"hr"> & { orientation?: "horizontal" | "vertical" }) {
  return (
    <hr
      data-slot="separator"
      aria-orientation={orientation}
      className={cn(
        "shrink-0 border-0 bg-hairline",
        orientation === "horizontal" ? "h-px w-full" : "h-full w-px",
        className,
      )}
      {...(props as React.ComponentProps<"hr">)}
    />
  );
}

export { Separator };
