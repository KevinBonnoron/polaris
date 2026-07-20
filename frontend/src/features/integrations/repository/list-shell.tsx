import type { ReactNode } from 'react';
import { ScrollArea } from '@/components/ui/scroll-area';
import { ListLoading } from './list-loading';

interface Props {
  title: string;
  initial: boolean;
  error: string | null;
  empty?: boolean;
  emptyText: string;
  filters?: ReactNode;
  children: ReactNode;
}

export function ListShell({ title: _title, initial, error, empty, emptyText, filters, children }: Props) {
  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {error && <p className="px-6 pb-2 text-sm text-destructive">{error}</p>}
      {filters && <div className="pb-3">{filters}</div>}
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col gap-2 pb-6 pr-3">{initial ? <ListLoading /> : empty && !error ? <p className="py-6 text-center text-sm text-muted-foreground">{emptyText}</p> : children}</div>
      </ScrollArea>
    </div>
  );
}
