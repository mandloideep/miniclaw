import type * as React from "react";

import { cn } from "@/lib/utils";

/* Native scroll surface tuned for our hairline-on-surface aesthetic. We
   skip the radix-ui scroll-area primitive here because the in-app
   scrollbar styles already match — adding the radix wrapper buys nothing
   beyond an extra dom layer. Keeps the API matching shadcn so the rest
   of the app reads the same. */
function ScrollArea({ className, children, ...props }: React.ComponentProps<"div">) {
  return (
    <div data-slot="scroll-area" className={cn("relative overflow-auto", className)} {...props}>
      {children}
    </div>
  );
}

export { ScrollArea };
