import { useLiveQuery } from '@tanstack/react-db';
import { Link, useRouterState } from '@tanstack/react-router';
import { ArrowDownAZ, ArrowUpDown, Bell, Bot, Check, ChevronsUpDown, Clock, Files, LayoutDashboard, Plus, Settings, SquareTerminal, Workflow } from 'lucide-react';
import { type ComponentType, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { notificationsCollection } from '@/collections/notifications.collection';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuBadge, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar';
import { AddIntegrationModal } from '@/features/integrations/add-integration-modal';
import { INTEGRATIONS } from '@/features/integrations/integration-catalog';
import { useNodejsRun } from '@/features/integrations/nodejs/nodejs-run-context';
import { getIntegrations } from '@/features/integrations/project-integrations';
import { usePythonRun } from '@/features/integrations/python/python-run-context';
import { useShellRun } from '@/features/integrations/shell/shell-context';
import { AddProjectModal } from '@/features/projects/add-project-modal';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { ProjectSettingsModal } from '@/features/projects/project-settings-modal';
import { cn } from '@/lib/utils';
import { useCurrentProject, useProjects } from '@/state/projects';
import { NotificationPopoverContent } from './notification-popover';

const NAV_ROUTES: Record<string, string> = {
  tickets: '/tickets',
  repository: '/repository',
  nodejs: '/nodejs',
  python: '/python',
  docker: '/docker',
  resend: '/resend',
  sentry: '/sentry',
  dokploy: '/dokploy',
  slack: '/slack',
  discord: '/discord',
  telegram: '/telegram',
};

const SIDEBAR_ORDER_KEY = 'polaris:sidebar-nav-order';
const PROJECT_SORT_KEY = 'polaris:project-sort';

type ProjectSort = 'recent' | 'alphabetical';

function readStoredProjectSort(): ProjectSort {
  if (typeof window === 'undefined') {
    return 'recent';
  }
  try {
    const raw = window.localStorage.getItem(PROJECT_SORT_KEY);
    return raw === 'alphabetical' ? 'alphabetical' : 'recent';
  } catch {
    return 'recent';
  }
}

type NavItem = { id: string; to: string; label: string; icon: ComponentType<{ className?: string }> };

function readStoredOrder(): string[] {
  if (typeof window === 'undefined') {
    return [];
  }
  try {
    const raw = window.localStorage.getItem(SIDEBAR_ORDER_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

function writeStoredOrder(ids: string[]) {
  try {
    window.localStorage.setItem(SIDEBAR_ORDER_KEY, JSON.stringify(ids));
  } catch {
    // ignore quota errors
  }
}

function applyOrder(items: NavItem[], order: string[]): NavItem[] {
  const byId = new Map(items.map((i) => [i.id, i]));
  const ordered: NavItem[] = [];
  for (const id of order) {
    const item = byId.get(id);
    if (item) {
      ordered.push(item);
      byId.delete(id);
    }
  }
  for (const item of items) {
    if (byId.has(item.id)) {
      ordered.push(item);
    }
  }
  return ordered;
}

export function AppSidebar() {
  const { t } = useTranslation();
  const { projects } = useProjects();
  const { project: currentProject, setProjectId } = useCurrentProject();
  const route = useRouterState({ select: (s) => s.location.pathname });

  const { data: notifs = [] } = useLiveQuery((q) => q.from({ n: notificationsCollection }));
  const waitingCount = notifs.filter((n) => !n.read && ((n.type === 'agent' && n.payload?.event === 'waiting') || n.severity === 'error')).length;

  const { data: agents = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const currentAgentsCount = currentProject ? agents.filter((a) => a.projectId === currentProject.id).length : 0;
  const [notifOpen, setNotifOpen] = useState(false);

  const { sessions, paneOpen, setPaneOpen, startSession } = useShellRun();
  const { run: nodejsRun } = useNodejsRun();
  const { run: pythonRun } = usePythonRun();
  const hasTerminalError = sessions.some((s) => s.exited && s.exited.code !== 0) || (nodejsRun?.exited?.code !== undefined && nodejsRun.exited.code !== 0) || (pythonRun?.exited?.code !== undefined && pythonRun.exited.code !== 0);
  const hasAnything = sessions.length > 0 || !!nodejsRun || !!pythonRun;

  const handleTerminalToggle = async () => {
    if (paneOpen && hasAnything) {
      setPaneOpen(false);
    } else if (hasAnything) {
      setPaneOpen(true);
    } else {
      await startSession();
    }
  };

  const [projectSort, setProjectSort] = useState<ProjectSort>(() => readStoredProjectSort());
  const updateProjectSort = useCallback((mode: ProjectSort) => {
    setProjectSort(mode);
    try {
      window.localStorage.setItem(PROJECT_SORT_KEY, mode);
    } catch {
      // ignore quota errors
    }
  }, []);

  const sortedProjects = useMemo(() => {
    const collator = new Intl.Collator(undefined, { sensitivity: 'base' });
    if (projectSort === 'alphabetical') {
      return [...projects].sort((a, b) => collator.compare(a.name, b.name));
    }
    const lastActivity = new Map<string, number>();
    for (const a of agents) {
      const prev = lastActivity.get(a.projectId) ?? 0;
      if (a.startedAt > prev) {
        lastActivity.set(a.projectId, a.startedAt);
      }
    }
    return [...projects].sort((a, b) => {
      const diff = (lastActivity.get(b.id) ?? 0) - (lastActivity.get(a.id) ?? 0);
      return diff !== 0 ? diff : collator.compare(a.name, b.name);
    });
  }, [projects, agents, projectSort]);

  const connected = getIntegrations(currentProject);
  const hasGit = currentProject?.hasGit === true;

  const stableNavItems: NavItem[] = useMemo(
    () => [
      { id: 'agents', to: '/', label: t('sidebar.agents'), icon: Bot },
      { id: 'files', to: '/files', label: t('sidebar.files'), icon: Files },
      { id: 'automations', to: '/automations', label: t('sidebar.automations'), icon: Workflow },
    ],
    [t],
  );

  const baseIntegrationItems: NavItem[] = useMemo(() => {
    return INTEGRATIONS.filter((i) => {
      if (!NAV_ROUTES[i.id]) {
        return false;
      }
      if (i.id === 'repository') {
        return hasGit;
      }
      return Object.hasOwn(connected, i.id);
    }).map((i) => ({
      id: `integration:${i.id}`,
      to: NAV_ROUTES[i.id],
      label: i.name,
      icon: i.icon,
    }));
  }, [connected, hasGit]);

  const [order, setOrder] = useState<string[]>(() => readStoredOrder());
  const [draggingId, setDraggingId] = useState<string | null>(null);

  const integrationItems = useMemo(() => applyOrder(baseIntegrationItems, order), [baseIntegrationItems, order]);

  useEffect(() => {
    const known = new Set(baseIntegrationItems.map((i) => i.id));
    const filtered = order.filter((id) => known.has(id));
    const missing = baseIntegrationItems.filter((i) => !order.includes(i.id)).map((i) => i.id);
    if (missing.length === 0 && filtered.length === order.length) {
      return;
    }
    const next = [...filtered, ...missing];
    setOrder(next);
    writeStoredOrder(next);
  }, [baseIntegrationItems, order]);

  const moveOver = useCallback(
    (targetId: string) => {
      setOrder((prev) => {
        if (!draggingId || draggingId === targetId) {
          return prev;
        }
        const from = prev.indexOf(draggingId);
        const to = prev.indexOf(targetId);
        if (from < 0 || to < 0 || from === to) {
          return prev;
        }
        const next = [...prev];
        next.splice(from, 1);
        next.splice(to, 0, draggingId);
        return next;
      });
    },
    [draggingId],
  );

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem className="relative">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton size="lg" className="data-[state=open]:bg-sidebar-accent">
                  <ProjectAvatar project={currentProject} className="aspect-square size-8" textClassName="text-xs" />
                  <div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate font-semibold">{currentProject?.name ?? t('sidebar.noProject')}</span>
                    <span className="truncate text-xs text-muted-foreground">{currentAgentsCount ? t('sidebar.agentCount', { count: currentAgentsCount }) : t('sidebar.idle')}</span>
                  </div>
                  <ChevronsUpDown className="ml-auto size-4 group-data-[collapsible=icon]:hidden" />
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" side="right" sideOffset={4} className="w-64">
                {sortedProjects.map((p) => (
                  <DropdownMenuItem key={p.id} onClick={() => setProjectId(p.id)} className="gap-2">
                    <ProjectAvatar project={p} className="size-5 rounded" textClassName="text-[9px]" />
                    <span className="flex-1">{p.name}</span>
                  </DropdownMenuItem>
                ))}
                <DropdownMenuSeparator />
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger className="gap-2">
                    <ArrowUpDown className="size-3.5" />
                    <span className="flex-1">{t('sidebar.sortProjects')}</span>
                  </DropdownMenuSubTrigger>
                  <DropdownMenuSubContent className="w-52">
                    <DropdownMenuItem onClick={() => updateProjectSort('recent')} className="gap-2">
                      <Clock className="size-3.5" />
                      <span className="flex-1">{t('sidebar.sortRecent')}</span>
                      {projectSort === 'recent' && <Check className="size-3.5" />}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => updateProjectSort('alphabetical')} className="gap-2">
                      <ArrowDownAZ className="size-3.5" />
                      <span className="flex-1">{t('sidebar.sortAlphabetical')}</span>
                      {projectSort === 'alphabetical' && <Check className="size-3.5" />}
                    </DropdownMenuItem>
                  </DropdownMenuSubContent>
                </DropdownMenuSub>
                <AddProjectModal>
                  <DropdownMenuItem onSelect={(e) => e.preventDefault()} className="gap-2">
                    <Plus className="size-3.5" />
                    {t('sidebar.addProject')}
                  </DropdownMenuItem>
                </AddProjectModal>
              </DropdownMenuContent>
            </DropdownMenu>
            {currentProject && (
              <ProjectSettingsModal projectId={currentProject.id}>
                <button
                  type="button"
                  aria-label={t('sidebar.projectSettings')}
                  onClick={(e) => e.stopPropagation()}
                  className="absolute top-8 left-7 z-10 flex size-5 cursor-pointer items-center justify-center rounded-full border border-sidebar-border bg-sidebar text-muted-foreground shadow-sm transition-colors hover:text-sidebar-accent-foreground group-data-[collapsible=icon]:top-7"
                >
                  <Settings className="size-3" />
                </button>
              </ProjectSettingsModal>
            )}
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent className="overflow-hidden">
        <ScrollArea className="flex-1">
          <SidebarGroup>
            <SidebarGroupLabel>{t('sidebar.workspace')}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {stableNavItems.map((item) => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton asChild isActive={route === item.to} tooltip={item.label}>
                      <Link to={item.to} preload="intent">
                        <item.icon />
                        <span>{item.label}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          <div className="px-2">
            <Separator className="bg-sidebar-border" />
          </div>

          <SidebarGroup>
            <SidebarGroupLabel>{t('sidebar.integrations')}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {integrationItems.map((item) => (
                  <SidebarMenuItem
                    key={item.id}
                    draggable
                    onDragStart={(e) => {
                      e.dataTransfer.setData('text/plain', item.id);
                      e.dataTransfer.effectAllowed = 'move';
                      setDraggingId(item.id);
                    }}
                    onDragEnd={() => {
                      setDraggingId(null);
                      writeStoredOrder(order);
                    }}
                    onDragOver={(e) => {
                      if (!draggingId) {
                        return;
                      }
                      e.preventDefault();
                      e.dataTransfer.dropEffect = 'move';
                      moveOver(item.id);
                    }}
                    onDrop={(e) => e.preventDefault()}
                    className={cn('cursor-grab transition-colors', draggingId === item.id && 'opacity-40')}
                  >
                    <SidebarMenuButton asChild isActive={route === item.to} tooltip={item.label}>
                      <Link to={item.to} preload="intent" draggable={false}>
                        <item.icon />
                        <span>{item.label}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
                <SidebarMenuItem>
                  <AddIntegrationModal>
                    <SidebarMenuButton tooltip={t('sidebar.addIntegration')} className="border border-dashed border-sidebar-border text-muted-foreground hover:text-sidebar-accent-foreground">
                      <Plus />
                      <span>{t('sidebar.add')}</span>
                    </SidebarMenuButton>
                  </AddIntegrationModal>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </ScrollArea>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton tooltip={t('sidebar.terminal')} onClick={() => void handleTerminalToggle()} isActive={paneOpen}>
              <div className="relative inline-flex shrink-0">
                <SquareTerminal />
                {hasTerminalError && <span className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-destructive" />}
              </div>
              <span>{t('sidebar.terminal')}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <Popover open={notifOpen} onOpenChange={setNotifOpen}>
              <PopoverTrigger asChild>
                <SidebarMenuButton tooltip={t('sidebar.inbox')}>
                  <div className="relative inline-flex shrink-0">
                    <Bell />
                    {waitingCount > 0 && <span className="absolute -top-0.5 -right-0.5 hidden size-2 rounded-full bg-destructive group-data-[collapsible=icon]:block" />}
                  </div>
                  <span>{t('sidebar.inbox')}</span>
                  {waitingCount > 0 && <SidebarMenuBadge>{waitingCount}</SidebarMenuBadge>}
                </SidebarMenuButton>
              </PopoverTrigger>
              <PopoverContent side="right" align="end" sideOffset={8} className="w-[420px] p-3">
                <NotificationPopoverContent onClose={() => setNotifOpen(false)} />
              </PopoverContent>
            </Popover>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton asChild isActive={route === '/dashboard'} tooltip={t('sidebar.dashboard')}>
              <Link to="/dashboard" preload="intent">
                <LayoutDashboard />
                <span>{t('sidebar.dashboard')}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton asChild isActive={route === '/settings'} tooltip={t('sidebar.settings')}>
              <Link to="/settings" search={{ section: 'general' }} preload="intent">
                <Settings />
                <span>{t('sidebar.settings')}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
