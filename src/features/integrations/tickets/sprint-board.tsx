import { type DragEvent, useState } from 'react';
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { IssueCard } from './issue-card';
import type { BoardColumn } from './types';

interface Props {
  columns: BoardColumn[];
  columnTargetStatusIds: Record<string, string[]>;
  pendingKeys?: Set<string>;
  onDropIssue: (issueKey: string, targetStatusIds: string[]) => void;
  onOpenIssue?: (issueKey: string) => void;
}

export function SprintBoard({ columns, columnTargetStatusIds, pendingKeys, onDropIssue, onOpenIssue }: Props) {
  const [selected, setSelected] = useState<string | null>(null);
  const [draggingKey, setDraggingKey] = useState<string | null>(null);
  const [hoverColumn, setHoverColumn] = useState<string | null>(null);

  const handleDrop = (columnName: string, e: DragEvent<HTMLElement>) => {
    e.preventDefault();
    setHoverColumn(null);
    setDraggingKey(null);
    const key = e.dataTransfer.getData('text/plain');
    const targetIds = columnTargetStatusIds[columnName];
    if (!key || !targetIds || targetIds.length === 0) {
      return;
    }
    onDropIssue(key, targetIds);
  };

  return (
    <ScrollArea className="h-full">
      <div className="flex min-w-full gap-4 pb-4">
        {columns.map((col) => {
          const droppable = (columnTargetStatusIds[col.name]?.length ?? 0) > 0;
          return (
            <section
              key={col.name}
              aria-label={col.name}
              className={cn('flex w-72 shrink-0 flex-col gap-2 rounded-md p-1 transition-colors', hoverColumn === col.name && droppable && 'bg-accent/40 ring-1 ring-blue-500/40')}
              onDragOver={(e) => {
                if (!droppable) {
                  return;
                }
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                if (hoverColumn !== col.name) {
                  setHoverColumn(col.name);
                }
              }}
              onDragLeave={() => setHoverColumn((prev) => (prev === col.name ? null : prev))}
              onDrop={(e) => handleDrop(col.name, e)}
            >
              <div className="flex items-center justify-between border-b border-border/60 px-1 pb-2">
                <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{col.name}</span>
                <span className="text-xs text-muted-foreground">{col.issues.length}</span>
              </div>
              <div className="flex flex-col gap-2">
                {col.issues.map((issue) => (
                  <IssueCard
                    key={issue.key}
                    issue={issue}
                    selected={selected === issue.key}
                    dragging={draggingKey === issue.key}
                    pending={pendingKeys?.has(issue.key)}
                    onClick={() => setSelected(issue.key === selected ? null : issue.key)}
                    onDoubleClick={onOpenIssue ? () => onOpenIssue(issue.key) : undefined}
                    onDragStart={() => setDraggingKey(issue.key)}
                    onDragEnd={() => setDraggingKey(null)}
                  />
                ))}
              </div>
            </section>
          );
        })}
      </div>
      <ScrollBar orientation="horizontal" />
    </ScrollArea>
  );
}
