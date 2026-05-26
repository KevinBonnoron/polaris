import { cn } from '@/lib/utils';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { tagTint } from './issue-type-color';
import type { Issue } from './types';

interface Props {
  issue: Issue;
  selected?: boolean;
  dragging?: boolean;
  pending?: boolean;
  onClick?: () => void;
  onDoubleClick?: () => void;
  onDragStart?: () => void;
  onDragEnd?: () => void;
}

export function IssueCard({ issue, selected, dragging, pending, onClick, onDoubleClick, onDragStart, onDragEnd }: Props) {
  const handleClick = () => {
    if (onClick) {
      onClick();
    } else {
      BrowserOpenURL(issue.url);
    }
  };

  const priority = issue.priority?.startsWith('P') ? issue.priority : priorityShort(issue.priority);
  const assignee = issue.assignee ? `@${issue.assignee.split(' ')[0].toLowerCase()}` : 'Unassigned';
  const tag = issue.labels?.[0] ?? issue.issueType;

  return (
    <button
      type="button"
      draggable
      onClick={handleClick}
      onDoubleClick={onDoubleClick}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', issue.key);
        onDragStart?.();
      }}
      onDragEnd={() => onDragEnd?.()}
      aria-busy={pending || undefined}
      disabled={pending}
      className={cn('flex w-full cursor-grab flex-col gap-2 rounded-lg border bg-card/40 p-3 text-left transition-colors hover:bg-card/60 active:cursor-grabbing', selected ? 'border-blue-500 ring-1 ring-blue-500/40' : 'border-border/60', dragging && 'opacity-50', pending && 'opacity-60 grayscale')}
    >
      <span className="text-xs uppercase tracking-wide text-muted-foreground">{issue.key}</span>
      <span className="text-sm font-medium leading-snug text-foreground">{issue.summary}</span>
      {tag && <span className={cn('inline-flex w-fit rounded px-2 py-0.5 text-[10px] font-medium leading-none', tagTint(tag))}>{tag.toLowerCase()}</span>}
      <span className="text-xs text-muted-foreground">
        {priority} <span className="px-1">•</span> {assignee}
      </span>
    </button>
  );
}

function priorityShort(p: string | undefined): string {
  switch ((p ?? '').toLowerCase()) {
    case 'highest':
      return 'P0';
    case 'high':
      return 'P1';
    case 'medium':
      return 'P2';
    case 'low':
      return 'P3';
    case 'lowest':
      return 'P4';
    default:
      return p || '—';
  }
}
