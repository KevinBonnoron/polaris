import { useLiveQuery } from '@tanstack/react-db';
import { useNavigate } from '@tanstack/react-router';
import { BellRing, Bot, Check, CheckCheck, MessageCircleQuestion, X } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { notificationsCollection } from '@/collections/notifications.collection';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { formatAgo } from '@/lib/format-ago';
import { notificationSeverity, SEVERITY_TONE } from '@/lib/severity';
import { cn } from '@/lib/utils';
import { selectAgent } from '@/state/agent-selection';
import { useCurrentProject, useProjects } from '@/state/projects';
import type { Notification } from '@/types';
import { CancelAgent } from '@/wailsjs/go/main/App';

type Filter = 'all' | 'waiting' | 'completed';

function matchesFilter(n: Notification, filter: Filter): boolean {
  if (filter === 'all') {
    return true;
  }
  if (filter === 'waiting') {
    return n.type === 'agent' && n.payload?.event === 'waiting';
  }
  // 'completed' covers everything that isn't a pending action: any non-waiting
  // notification, whatever its severity (the icon tone says how it went).
  return !(n.type === 'agent' && n.payload?.event === 'waiting');
}

function iconFor(n: Notification): React.ReactNode {
  if (n.type === 'agent') {
    return n.payload?.event === 'waiting' ? <MessageCircleQuestion className="size-3.5" /> : n.severity === 'error' ? <X className="size-3.5" /> : <Check className="size-3.5" />;
  }
  if (n.type === 'automation') {
    return <Bot className="size-3.5" />;
  }
  return <BellRing className="size-3.5" />;
}

function toneFor(n: Notification): string {
  if (n.type === 'agent' && n.payload?.event === 'waiting') {
    return SEVERITY_TONE.warning;
  }
  return SEVERITY_TONE[notificationSeverity(n.severity)];
}

function agentIdOf(n: Notification): string | undefined {
  if (n.type === 'agent') {
    return n.payload.agentId;
  }
  if (n.type === 'automation') {
    return n.payload.agentId;
  }
  return undefined;
}

export function NotificationPopoverContent({ onClose }: { onClose?: () => void }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { projects } = useProjects();
  const { setProjectId } = useCurrentProject();
  const [filter, setFilter] = useState<Filter>('all');

  const { data: notifications = [] } = useLiveQuery((q) => q.from({ n: notificationsCollection }));
  const unreadNotifications = notifications.filter(({ read }) => !read);

  const projectsById = useMemo(() => {
    const map = new Map<string, (typeof projects)[number]>();
    for (const p of projects) {
      map.set(p.id, p);
    }
    return map;
  }, [projects]);

  const waitingCount = unreadNotifications.filter((n) => n.type === 'agent' && n.payload?.event === 'waiting').length;

  const filtered = useMemo(() => unreadNotifications.filter((n) => matchesFilter(n, filter)), [filter, unreadNotifications]);

  const grouped = useMemo(() => {
    const now = Date.now() / 1000;
    const day = 24 * 60 * 60;
    const today: Notification[] = [];
    const yesterday: Notification[] = [];
    const earlier: Notification[] = [];
    for (const n of filtered) {
      const ageDays = (now - n.createdAt) / day;
      if (ageDays < 1) {
        today.push(n);
      } else if (ageDays < 2) {
        yesterday.push(n);
      } else {
        earlier.push(n);
      }
    }
    return { today, yesterday, earlier };
  }, [filtered]);

  const markAll = () => {
    for (const n of unreadNotifications) {
      if (!n.read) {
        notificationsCollection.update(n.id, (draft) => {
          draft.read = true;
        });
      }
    }
  };

  const openNotification = (n: Notification) => {
    if (!n.read) {
      notificationsCollection.update(n.id, (draft) => {
        draft.read = true;
      });
    }
    setProjectId(n.projectId);
    const agentId = agentIdOf(n);
    if (agentId) {
      selectAgent(agentId);
      void navigate({ to: '/' });
      onClose?.();
    }
  };

  const renderRow = (n: Notification) => {
    const project = projectsById.get(n.projectId);
    const projectName = project?.name ?? n.projectId;
    const tone = toneFor(n);
    const agentId = agentIdOf(n);
    const canStop = n.type === 'automation' && Boolean(agentId);
    return (
      // biome-ignore lint/a11y/useSemanticElements: row wraps a nested Stop <Button>; a parent <button> would create invalid markup
      <div
        key={n.id}
        role="button"
        tabIndex={0}
        onClick={() => openNotification(n)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            openNotification(n);
          }
        }}
        className="group flex cursor-pointer items-start gap-3 rounded-md px-2 py-2 text-left transition-colors hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
      >
        <div className={cn('mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full', tone)}>{iconFor(n)}</div>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex items-center gap-1.5">
            <Badge variant="outline" className="gap-1.5 py-0 pl-0.5 pr-2 font-normal">
              <ProjectAvatar project={project} className="size-4 rounded-sm" textClassName="text-[8px]" />
              <span className="truncate text-[11px]">{projectName}</span>
            </Badge>
            <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">{formatAgo(n.createdAt, i18n.language)}</span>
          </div>
          <p className="line-clamp-2 break-words text-xs font-medium text-foreground">{n.title}</p>
        </div>
        {canStop && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-7 shrink-0 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                aria-label={t('shell.notifications.actionStop')}
                onClick={async (e) => {
                  e.stopPropagation();
                  if (agentId) {
                    try {
                      await CancelAgent(agentId);
                    } catch {}
                  }
                }}
              >
                <X className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('shell.notifications.actionStop')}</TooltipContent>
          </Tooltip>
        )}
      </div>
    );
  };

  const empty = grouped.today.length + grouped.yesterday.length + grouped.earlier.length === 0;

  const renderGroup = (label: string, items: Notification[], showSeparator: boolean) => {
    if (items.length === 0) {
      return null;
    }
    return (
      <div className="flex flex-col">
        {showSeparator && <Separator className="my-1" />}
        <div className="px-2 pb-1 pt-2 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{label}</div>
        <div className="flex flex-col gap-0.5">{items.map(renderRow)}</div>
      </div>
    );
  };

  return (
    <div className="flex flex-col gap-3" role="dialog" aria-label="Notifications">
      <Tabs value={filter} onValueChange={(v) => setFilter(v as Filter)}>
        <TabsList>
          <TabsTrigger value="all">{t('shell.notifications.all')}</TabsTrigger>
          <TabsTrigger value="waiting" className="gap-1.5">
            {t('shell.notifications.waiting')}
            {waitingCount > 0 && (
              <Badge variant="secondary" className="h-4 min-w-4 px-1 text-[10px]">
                {waitingCount}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="completed">{t('shell.notifications.done')}</TabsTrigger>
        </TabsList>
      </Tabs>
      <ScrollArea className="h-[480px] -mx-1 pr-2">
        {empty ? (
          <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
            <div className="flex size-10 items-center justify-center rounded-full bg-muted">
              <Check className="size-5 text-muted-foreground" />
            </div>
            <div className="text-sm font-medium">{t('shell.notifications.allCaughtUp')}</div>
            <div className="text-xs text-muted-foreground">{t('shell.notifications.allCaughtUpDesc')}</div>
          </div>
        ) : (
          <div className="flex flex-col px-1">
            {renderGroup(t('shell.notifications.today'), grouped.today, false)}
            {renderGroup(t('shell.notifications.yesterday'), grouped.yesterday, grouped.today.length > 0)}
            {renderGroup(t('shell.notifications.earlier'), grouped.earlier, grouped.today.length + grouped.yesterday.length > 0)}
          </div>
        )}
      </ScrollArea>
      {!empty && (
        <>
          <Separator />
          <div className="flex justify-end">
            <Button variant="ghost" size="sm" className="gap-1.5" onClick={markAll}>
              <CheckCheck className="size-3.5" />
              {t('shell.notifications.markAllRead')}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
