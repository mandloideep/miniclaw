import { Tabs as TabsPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/utils";

const Tabs = TabsPrimitive.Root;

function TabsList({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      className={cn(
        "inline-flex h-8 items-center gap-1 rounded-md bg-surface-1 p-0.5 border border-hairline",
        className,
      )}
      {...props}
    />
  );
}

function TabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      className={cn(
        "inline-flex items-center justify-center whitespace-nowrap rounded-sm px-2.5 py-1 text-[12px] text-ink-subtle transition-colors",
        "data-[state=active]:bg-surface-3 data-[state=active]:text-ink",
        "disabled:pointer-events-none disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

function TabsContent({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      className={cn("mt-3 focus-visible:outline-none", className)}
      {...props}
    />
  );
}

export { Tabs, TabsContent, TabsList, TabsTrigger };
