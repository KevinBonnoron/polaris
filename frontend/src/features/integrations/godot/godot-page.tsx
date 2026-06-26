import { Gamepad2, Pencil, Play, Plug, Settings, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { useCurrentProject } from '@/state/projects';
import { ConfigureIntegrationModal } from '../configure-integration-modal';
import { InstanceSelector } from '../instance-selector';
import { findIntegration } from '../integration-catalog';
import { useGodotRun } from './godot-run-context';
import type { GodotConfig } from './types';

export function GodotPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const { run, isRunning, config, instances, instanceIndex, setInstanceIndex, startScript, stop } = useGodotRun();
  const integration = findIntegration('godot');

  const typedConfig = config as GodotConfig | null;
  const manifestPath = typedConfig?.manifestPath ?? '';
  const playCommand = (typedConfig?.playCommand || 'play').trim();

  const actions: { label: string; command: string; Icon: typeof Play }[] = [
    { label: playCommand, command: playCommand, Icon: Play },
    { label: `${playCommand} -e`, command: `${playCommand} -e`, Icon: Pencil },
  ];

  const toggle = (command: string) => {
    const active = isRunning && run?.scriptName === command;
    return active ? stop() : startScript(command);
  };

  if (!project) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('integrations.godot.selectProject')}</p>
      </div>
    );
  }

  if (!manifestPath) {
    return (
      <div className="flex h-full flex-col gap-6 p-4">
        <header className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('integrations.godot.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('integrations.godot.noManifest', { project: project.name })}</p>
        </header>
        <Card className="border-dashed">
          <CardHeader className="items-center text-center">
            <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
              <Plug className="size-5 text-muted-foreground" />
            </div>
            <CardTitle className="text-base">{t('integrations.godot.connectTitle')}</CardTitle>
            <CardDescription>{t('integrations.godot.connectDesc')}</CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <ConfigureIntegrationModal projectId={project.id} integrationId="godot">
              <Button>{t('integrations.godot.configureCta')}</Button>
            </ConfigureIntegrationModal>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col gap-6 p-4">
        <PageHeader
          icon={<Gamepad2 className="size-5 text-muted-foreground" />}
          title={t('integrations.godot.title')}
          badges={<Badge variant="secondary">{playCommand}</Badge>}
          subtitle={<span title={manifestPath}>{manifestPath}</span>}
          actions={
            <div className="flex items-center gap-2">
              {integration && <InstanceSelector integration={integration} instances={instances} selectedIndex={instanceIndex} onSelect={setInstanceIndex} projectPath={project.path ?? ''} />}
              <ConfigureIntegrationModal projectId={project.id} integrationId="godot" instanceIndex={instanceIndex}>
                <Button variant="outline" size="sm">
                  <Settings className="size-3.5" />
                  {t('common.configure')}
                </Button>
              </ConfigureIntegrationModal>
            </div>
          }
        />

        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">{t('integrations.godot.quickActions')}</h2>
          <div className="flex flex-wrap gap-2">
            {actions.map(({ label, command, Icon }) => {
              const active = isRunning && run?.scriptName === command;
              return (
                <Button key={command} variant={active ? 'default' : 'outline'} size="sm" className={cn('gap-1.5 font-mono text-xs', active && 'animate-pulse')} disabled={isRunning && !active} onClick={() => void toggle(command)}>
                  {active ? <Square className="size-3" /> : <Icon className="size-3" />}
                  {label}
                </Button>
              );
            })}
          </div>
        </section>
      </div>
    </ScrollArea>
  );
}
