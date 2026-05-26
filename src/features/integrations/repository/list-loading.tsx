import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

const SKELETON_ROWS: { id: string; title: string; subtitle: string }[] = [
  { id: 'r1', title: 'w-3/5', subtitle: 'w-1/4' },
  { id: 'r2', title: 'w-4/5', subtitle: 'w-1/3' },
  { id: 'r3', title: 'w-2/5', subtitle: 'w-1/5' },
  { id: 'r4', title: 'w-3/4', subtitle: 'w-1/4' },
  { id: 'r5', title: 'w-1/2', subtitle: 'w-1/3' },
];

export function ListLoading() {
  return (
    <>
      {SKELETON_ROWS.map((row) => (
        <div key={row.id} className="flex items-start gap-3 rounded-md px-2 py-2">
          <Skeleton className="mt-0.5 size-4 rounded-sm" />
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            <Skeleton className={cn('h-4', row.title)} />
            <Skeleton className={cn('h-3', row.subtitle)} />
          </div>
          <Skeleton className="mt-1 size-3.5 shrink-0 rounded-sm" />
        </div>
      ))}
    </>
  );
}
