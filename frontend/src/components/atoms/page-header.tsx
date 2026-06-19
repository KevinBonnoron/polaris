import type { ReactNode } from 'react';

interface Props {
  title: ReactNode;
  icon?: ReactNode;
  badges?: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
}

export function PageHeader({ title, icon, badges, subtitle, actions }: Props) {
  return (
    <header className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex items-center gap-2">
          {icon}
          <h1 className="truncate text-2xl font-semibold tracking-tight">{title}</h1>
          {badges}
        </div>
        {subtitle && <div className="truncate text-sm text-muted-foreground">{subtitle}</div>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  );
}
