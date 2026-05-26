import { useLiveQuery } from '@tanstack/react-db';
import { Link, useRouterState } from '@tanstack/react-router';
import { ArrowDownAZ, ArrowUpDown, Bell, Bot, Check, ChevronsUpDown, Clock, Files, Plus, Settings, Workflow } from 'lucide-react';
import { type DragEvent, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import { Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuBadge, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar';
import { agentsCollection, notificationsCollection } from '@/db';
import { AddIntegrationModal } from '@/features/integrations/add-integration-modal';
import { INTEGRATIONS } from '@/features/integrations/integration-catalog';
import { getIntegrations } from '@/features/integrations/project-integrations';
import { AddProjectModal } from '@/features/projects/add-project-modal';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { ProjectSettingsModal } from '@/features/projects/project-settings-modal';
import { SettingsModal } from '@/features/settings/settings-modal';
import { cn } from '@/lib/utils';
import { useCurrentProject, useProjects } from '@/state/projects';
import { projectHasAutomatable } from '../automations/eligibility';
import { ConsoleMenu } from './console-menu';
import { NotificationPopoverContent } from './notification-popover';

const NAV_ROUTES: Record<string, string> = {
  jira: '/jira',
  repository: '/repository',
  nodejs: '/nodejs',
  docker: '/docker',
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

type NavItem = { id: string; to: string; label: string; icon: typeof Bot };

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
  const hasAutomatable = projects.some(projectHasAutomatable);
  const hasGit = currentProject?.hasGit === true;
  const baseNavItems: NavItem[] = useMemo(() => {
    const integrationNav: NavItem[] = INTEGRATIONS.filter((i) => {
      if (!NAV_ROUTES[i.id]) return false;
      if (i.id === 'repository') return hasGit;
      return Object.hasOwn(connected, i.id);
    }).map((i) => ({
      id: `integration:${i.id}`,
      to: NAV_ROUTES[i.id],
      label: i.name,
      icon: i.icon,
    }));
    return [{ id: 'agents', to: '/', label: t('sidebar.agents'), icon: Bot }, { id: 'files', to: '/files', label: t('sidebar.files'), icon: Files }, ...integrationNav, ...(hasAutomatable ? [{ id: 'automations', to: '/automations', label: t('sidebar.automations'), icon: Workflow }] : [])];
  }, [t, connected, hasAutomatable, hasGit]);

  const [order, setOrder] = useState<string[]>(() => readStoredOrder());
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [overId, setOverId] = useState<string | null>(null);

  const navItems = useMemo(() => applyOrder(baseNavItems, order), [baseNavItems, order]);

  useEffect(() => {
    const known = new Set(baseNavItems.map((i) => i.id));
    const filtered = order.filter((id) => known.has(id));
    const missing = baseNavItems.filter((i) => !order.includes(i.id)).map((i) => i.id);
    if (missing.length === 0 && filtered.length === order.length) {
      return;
    }
    const next = [...filtered, ...missing];
    setOrder(next);
    writeStoredOrder(next);
  }, [baseNavItems, order]);

  const handleDrop = useCallback(
    (targetId: string, e: DragEvent<HTMLLIElement>) => {
      e.preventDefault();
      const sourceId = e.dataTransfer.getData('text/plain');
      setDraggingId(null);
      setOverId(null);
      if (!sourceId || sourceId === targetId) {
        return;
      }
      const ids = navItems.map((i) => i.id);
      const from = ids.indexOf(sourceId);
      const to = ids.indexOf(targetId);
      if (from < 0 || to < 0) {
        return;
      }
      const next = [...ids];
      next.splice(from, 1);
      next.splice(to, 0, sourceId);
      setOrder(next);
      writeStoredOrder(next);
    },
    [navItems],
  );

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
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
                    <ProjectSettingsModal projectId={p.id}>
                      <button type="button" className="rounded p-1 hover:bg-accent" onClick={(e) => e.stopPropagation()}>
                        <Settings className="size-3.5" />
                      </button>
                    </ProjectSettingsModal>
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
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>{t('sidebar.workspace')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => (
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
                    setOverId(null);
                  }}
                  onDragOver={(e) => {
                    if (!draggingId || draggingId === item.id) {
                      return;
                    }
                    e.preventDefault();
                    e.dataTransfer.dropEffect = 'move';
                    if (overId !== item.id) {
                      setOverId(item.id);
                    }
                  }}
                  onDragLeave={() => setOverId((prev) => (prev === item.id ? null : prev))}
                  onDrop={(e) => handleDrop(item.id, e)}
                  className={cn('cursor-grab transition-colors', draggingId === item.id && 'opacity-50', overId === item.id && draggingId && draggingId !== item.id && 'ring-1 ring-sidebar-ring rounded-md')}
                >
                  <SidebarMenuButton asChild isActive={route === item.to} tooltip={item.label}>
                    <Link to={item.to} preload="intent" draggable={false}>
                      <item.icon />
                      <span>{item.label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
              <Separator className="my-1" />
              <SidebarMenuItem>
                <AddIntegrationModal>
                  <SidebarMenuButton tooltip={t('sidebar.addIntegration')}>
                    <Plus />
                    <span>{t('sidebar.add')}</span>
                  </SidebarMenuButton>
                </AddIntegrationModal>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <ConsoleMenu />
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <Popover>
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
                <NotificationPopoverContent />
              </PopoverContent>
            </Popover>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SettingsModal>
              <SidebarMenuButton tooltip={t('sidebar.settings')}>
                <Settings />
                <span>{t('sidebar.settings')}</span>
              </SidebarMenuButton>
            </SettingsModal>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
