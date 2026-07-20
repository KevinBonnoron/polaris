import { useCallback, useEffect, useRef, useState } from 'react';
import type { KeyboardEvent, MouseEvent, ReactNode } from 'react';
import { cn } from '@/lib/utils';

interface Props {
  onConfirm: () => void;
  icon: ReactNode;
  confirmIcon: ReactNode;
  timeout?: number;
  className?: string;
  'aria-label'?: string;
  confirmAriaLabel?: string;
}

export function ClickToConfirm({ onConfirm, icon, confirmIcon, timeout = 2000, className, 'aria-label': ariaLabel, confirmAriaLabel }: Props) {
  const [armed, setArmed] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    [],
  );

  const handleClick = useCallback(
    (e: MouseEvent | KeyboardEvent) => {
      e.stopPropagation();
      if (armed) {
        if (timerRef.current) clearTimeout(timerRef.current);
        setArmed(false);
        onConfirm();
      } else {
        setArmed(true);
        timerRef.current = setTimeout(() => setArmed(false), timeout);
      }
    },
    [armed, onConfirm, timeout],
  );

  return (
    <span
      role="button"
      tabIndex={0}
      aria-label={armed ? (confirmAriaLabel ?? ariaLabel) : ariaLabel}
      className={cn('rounded p-0.5 transition-colors', armed ? 'bg-destructive/15 text-destructive ring-1 ring-inset ring-destructive/30' : 'text-muted-foreground hover:text-destructive', className)}
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') handleClick(e);
      }}
    >
      {armed ? confirmIcon : icon}
    </span>
  );
}
