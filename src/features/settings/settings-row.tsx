import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

interface Props {
  label: ReactNode;
  description?: ReactNode;
  control: ReactNode;
  className?: string;
}

export function SettingsRow({ label, description, control, className }: Props) {
  return (
    <div className={cn('flex items-center justify-between gap-4 border-b border-border py-3 last:border-b-0', className)}>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{label}</div>
        {description && <div className="mt-0.5 text-xs text-muted-foreground">{description}</div>}
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}
