import { Hammer, ListChecks, Play, Plug, Rocket, Settings, Square } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { useCurrentProject } from '@/state/projects';
import { ListTaskfileTasks } from '@/wailsjs/go/main/App';
import { ConfigureIntegrationModal } from '../configure-integration-modal';
import { findIntegration } from '../integration-catalog';
import { InstanceSelector } from '../instance-selector';
import { useTaskfileRun } from './taskfile-run-context';
import type { TaskfileConfig } from './types';

interface Task {
  name: string;
  command?: string;
}

const QUICK_ACTIONS: { key: keyof TaskfileConfig; Icon: typeof Play }[] = [
  { key: 'startTask', Icon: Play },
  { key: 'testTask', Icon: ListChecks },
  { key: 'buildTask', Icon: Hammer },
];

export function TaskfilePage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { run, isRunning, config, instances, instanceIndex, setInstanceIndex, startScript, stop } = useTaskfileRun();
  const integration = findIntegration('taskfile');

  const manifestPath = config?.manifestPath ?? '';
  const typedConfig = config as TaskfileConfig | null;

  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const refreshSeq = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++refreshSeq.current;
    if (!manifestPath) {
      setTasks([]);
      return;
    }
    setLoading(true);
    try {
      const list = await ListTaskfileTasks(manifestPath);
      if (seq !== refreshSeq.current) {
        return;
      }
      setTasks((list ?? []).map((s) => ({ name: s.name, command: s.command })));
    } catch (err) {
      if (seq !== refreshSeq.current) {
        return;
      }
      toastError({ title: t('integrations.taskfile.couldNotList'), err });
    } finally {
      if (seq === refreshSeq.current) {
        setLoading(false);
      }
    }
  }, [manifestPath, t]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const toggle = (name: string) => {
    const active = isRunning && run?.scriptName === name;
    return active ? stop() : startScript(name);
  };

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.taskfile.selectProject')}</p>
      </div>
    );
  }

  if (!manifestPath) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.taskfile.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.taskfile.noManifest', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.taskfile.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.taskfile.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="taskfile">
              <Button>{t('integrations.taskfile.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  const pinned = QUICK_ACTIONS.map(({ key, Icon }) => ({ Icon, name: typeof typedConfig?.[key] === 'string' ? (typedConfig[key] as string) : '' })).filter((a) => a.name);

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-6 p-4">
        <PageHeader
          icon={<Rocket className="size-5 text-muted-foreground" />}
          title={t('integrations.taskfile.title')}
          badges={<Badge variant="secondary">task</Badge>}
          subtitle={<span title={manifestPath}>{manifestPath}</span>}
          actions={
            <div className="flex items-center gap-2">
              {integration && <InstanceSelector integration={integration} instances={instances} selectedIndex={instanceIndex} onSelect={setInstanceIndex} projectPath={project.path ?? ''} />}
              <RefreshAction onRefresh={() => refresh()} loading={loading} />
              <ConfigureIntegrationModal projectId={project.id} integrationId="taskfile" instanceIndex={instanceIndex}>
                <Button variant="outline" size="sm">
                  <Settings className="size-3.5" />
                  {t('common.configure')}
                </Button>
              </ConfigureIntegrationModal>
            </div>
          }
        />

        {pinned.length > 0 && (
          <section className="flex flex-col gap-2">
            <h2 className="text-sm font-medium text-muted-foreground">{t('integrations.taskfile.quickActions')}</h2>
            <div className="flex flex-wrap gap-2">
              {pinned.map(({ Icon, name }) => {
                const active = isRunning && run?.scriptName === name;
                return (
                  <Button key={name} variant={active ? 'default' : 'outline'} size="sm" className={cn('gap-1.5 font-mono text-xs', active && 'animate-pulse')} disabled={isRunning && !active} onClick={() => void toggle(name)}>
                    {active ? <Square className="size-3" /> : <Icon className="size-3" />}
                    {name}
                  </Button>
                );
              })}
            </div>
          </section>
        )}

        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t('integrations.taskfile.tabTasks')}</h2>
          {tasks.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('integrations.taskfile.noTasks')}</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {tasks.map((task) => {
                const active = isRunning && run?.scriptName === task.name;
                const button = (
                  <Button variant={active ? 'default' : 'outline'} size="sm" className={cn('gap-1.5 font-mono text-xs', active && 'animate-pulse')} disabled={isRunning && !active} onClick={() => void toggle(task.name)}>
                    {active ? <Square className="size-3" /> : <Play className="size-3" />}
                    {task.name}
                  </Button>
                );
                return task.command ? (
                  <Tooltip key={task.name}>
                    <TooltipTrigger asChild>{button}</TooltipTrigger>
                    <TooltipContent side="bottom" className="font-mono text-xs">
                      {task.command}
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  <span key={task.name}>{button}</span>
                );
              })}
            </div>
          )}
        </section>
      </div>
    </ScrollArea>
  );
}
