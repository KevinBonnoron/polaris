import type { ReactNode } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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

export function ListShell({ title, initial, error, empty, emptyText, filters, children }: Props) {
  return (
    <Card className="flex h-full min-h-0 flex-col">
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        {error && <CardDescription className="text-destructive">{error}</CardDescription>}
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col p-0">
        {filters}
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-2 px-6 pb-6">{initial ? <ListLoading /> : empty && !error ? <p className="py-6 text-center text-sm text-muted-foreground">{emptyText}</p> : children}</div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
