import { Skeleton } from '@/components/ui/skeleton';

export function BoardSkeleton({ columns = 4, cardsPerColumn = 3 }: { columns?: number; cardsPerColumn?: number }) {
  return (
    <div className="flex h-full flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Skeleton className="h-8 w-[200px]" />
        <Skeleton className="h-9 min-w-0 flex-1" />
        <Skeleton className="h-9 w-48" />
        <Skeleton className="h-8 w-24" />
      </div>
      <div className="flex min-w-full gap-4 pb-4">
        {Array.from({ length: columns }, (_, i) => `col-${i}`).map((colKey) => (
          <div key={colKey} className="flex w-72 shrink-0 flex-col gap-2 p-1">
            <div className="flex items-center justify-between border-b border-border/60 px-1 pb-2">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-3 w-4" />
            </div>
            <div className="flex flex-col gap-2">
              {Array.from({ length: cardsPerColumn }, (_, i) => `${colKey}-card-${i}`).map((cardKey) => (
                <div key={cardKey} className="flex flex-col gap-2 rounded-lg border border-border/60 bg-card/40 p-3">
                  <Skeleton className="h-3 w-16" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-4 w-12" />
                  <Skeleton className="h-3 w-24" />
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
