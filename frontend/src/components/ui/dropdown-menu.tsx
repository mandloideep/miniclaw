import { Check, ChevronRight } from "lucide-react";
import { DropdownMenu as DropdownPrimitive } from "radix-ui";
import type * as React from "react";

import { cn } from "@/lib/utils";

const DropdownMenu = DropdownPrimitive.Root;
const DropdownMenuTrigger = DropdownPrimitive.Trigger;
const DropdownMenuGroup = DropdownPrimitive.Group;
const DropdownMenuPortal = DropdownPrimitive.Portal;
const DropdownMenuSub = DropdownPrimitive.Sub;
const DropdownMenuRadioGroup = DropdownPrimitive.RadioGroup;

function DropdownMenuContent({
  className,
  sideOffset = 6,
  ...props
}: React.ComponentProps<typeof DropdownPrimitive.Content>) {
  return (
    <DropdownPrimitive.Portal>
      <DropdownPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          "z-50 min-w-[180px] overflow-hidden rounded-md border border-hairline bg-surface-2 p-1 shadow-lg",
          "data-[state=open]:animate-in data-[state=closed]:animate-out",
          "data-[state=open]:fade-in-0 data-[state=closed]:fade-out-0",
          "data-[side=bottom]:slide-in-from-top-1 data-[side=top]:slide-in-from-bottom-1",
          className,
        )}
        {...props}
      />
    </DropdownPrimitive.Portal>
  );
}

function DropdownMenuItem({
  className,
  inset,
  ...props
}: React.ComponentProps<typeof DropdownPrimitive.Item> & { inset?: boolean }) {
  return (
    <DropdownPrimitive.Item
      className={cn(
        "relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-[13px] text-ink-muted outline-none",
        "focus:bg-surface-3 focus:text-ink",
        "data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        inset && "pl-8",
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuLabel({
  className,
  inset,
  ...props
}: React.ComponentProps<typeof DropdownPrimitive.Label> & { inset?: boolean }) {
  return (
    <DropdownPrimitive.Label
      className={cn(
        "px-2 py-1.5 text-[10px] uppercase tracking-[0.08em] text-ink-tertiary",
        inset && "pl-8",
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuSeparator({
  className,
  ...props
}: React.ComponentProps<typeof DropdownPrimitive.Separator>) {
  return (
    <DropdownPrimitive.Separator
      className={cn("-mx-1 my-1 h-px bg-hairline", className)}
      {...props}
    />
  );
}

function DropdownMenuCheckboxItem({
  className,
  children,
  checked,
  ...props
}: React.ComponentProps<typeof DropdownPrimitive.CheckboxItem>) {
  return (
    <DropdownPrimitive.CheckboxItem
      className={cn(
        "relative flex cursor-default select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-[13px] outline-none",
        "focus:bg-surface-3 focus:text-ink data-[disabled]:opacity-50",
        className,
      )}
      checked={checked}
      {...props}
    >
      <span className="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
        <DropdownPrimitive.ItemIndicator>
          <Check className="h-3 w-3" />
        </DropdownPrimitive.ItemIndicator>
      </span>
      {children}
    </DropdownPrimitive.CheckboxItem>
  );
}

function DropdownMenuShortcut({ className, ...props }: React.ComponentProps<"span">) {
  return <span className={cn("ml-auto text-[10px] text-ink-tertiary", className)} {...props} />;
}

export {
  ChevronRight as DropdownMenuChevron,
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuRadioGroup,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuTrigger,
};
