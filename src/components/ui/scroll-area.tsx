'use client';

import { ScrollArea as ScrollAreaPrimitive } from 'radix-ui';
import type * as React from 'react';
import { cn } from '@/lib/utils';

function ScrollArea({ className, children, viewportProps, ...props }: React.ComponentProps<typeof ScrollAreaPrimitive.Root> & { viewportProps?: React.ComponentProps<typeof ScrollAreaPrimitive.Viewport> }) {
  const { className: viewportClassName, ...restViewportProps } = viewportProps ?? {};
  return (
    <ScrollAreaPrimitive.Root data-slot="scroll-area" className={cn('relative', className)} {...props}>
      <ScrollAreaPrimitive.Viewport data-slot="scroll-area-viewport" className={cn('size-full rounded-[inherit] transition-[color,box-shadow] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1', viewportClassName)} {...restViewportProps}>
        {children}
      </ScrollAreaPrimitive.Viewport>
      <ScrollBar />
      <ScrollAreaPrimitive.Corner />
    </ScrollAreaPrimitive.Root>
  );
}

function ScrollBar({ className, orientation = 'vertical', ...props }: React.ComponentProps<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>) {
  return (
    <ScrollAreaPrimitive.ScrollAreaScrollbar
      data-slot="scroll-area-scrollbar"
      orientation={orientation}
      className={cn('group/scrollbar flex touch-none p-0.5 select-none', orientation === 'vertical' && 'h-full w-2 hover:w-2.5 transition-[width] duration-150 ease-out', orientation === 'horizontal' && 'h-2 hover:h-2.5 flex-col transition-[height] duration-150 ease-out', className)}
      {...props}
    >
      <ScrollAreaPrimitive.ScrollAreaThumb data-slot="scroll-area-thumb" className="relative flex-1 rounded-full bg-border/60 transition-colors duration-150 group-hover/scrollbar:bg-border hover:bg-muted-foreground/60" />
    </ScrollAreaPrimitive.ScrollAreaScrollbar>
  );
}

export { ScrollArea, ScrollBar };
