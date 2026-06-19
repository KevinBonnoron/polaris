import { Loader2Icon } from 'lucide-react';
import { cn } from '@/lib/utils';

interface Props {
  className?: string;
}

export function PageLoader({ className }: Props) {
  return (
    <div className={cn('flex h-full min-h-[200px] w-full items-center justify-center', className)}>
      <Loader2Icon className="size-8 animate-spin text-muted-foreground" />
    </div>
  );
}
