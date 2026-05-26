import { ExternalLink } from 'lucide-react';
import type { ReactNode } from 'react';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { LabelChip } from './label-chip';
import type { Label } from './types';

interface Props {
  icon: ReactNode;
  title: string;
  subtitle: string;
  labels?: Label[];
  url: string;
  meta?: ReactNode;
}

export function Row({ icon, title, subtitle, labels, url, meta }: Props) {
  return (
    <button type="button" onClick={() => BrowserOpenURL(url)} className="flex items-start gap-3 rounded-md px-2 py-2 text-left transition-colors hover:bg-accent">
      <div className="mt-0.5">{icon}</div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="truncate text-sm font-medium">{title}</span>
          {meta}
          {labels?.map((label) => (
            <LabelChip key={label.name} label={label} />
          ))}
        </div>
        <div className="truncate text-xs text-muted-foreground">{subtitle}</div>
      </div>
      <ExternalLink className="mt-1 size-3.5 shrink-0 text-muted-foreground" />
    </button>
  );
}
