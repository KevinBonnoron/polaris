import { useNavigate } from '@tanstack/react-router';
import { Bot, Download, Files, FolderPlus, Hammer, Play, Plus, Settings, Square, TerminalSquare, TestTube, Workflow } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from '@/components/ui/command';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { startDraftAgent } from '@/features/agents/start-draft-agent';
import { projectHasAutomatable } from '@/features/automations/eligibility';
import { AddIntegrationModal } from '@/features/integrations/add-integration-modal';
import { INTEGRATIONS } from '@/features/integrations/integration-catalog';
import { useNodejsRun } from '@/features/integrations/nodejs/nodejs-run-context';
import { getIntegrations } from '@/features/integrations/project-integrations';
import { usePythonRun } from '@/features/integrations/python/python-run-context';
import { installArgs } from '@/features/integrations/python/types';
import { AddProjectModal } from '@/features/projects/add-project-modal';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { SettingsModal } from '@/features/settings/settings-modal';
import { shortcutDisplayKeys } from '@/lib/shortcuts';
import { useShortcuts } from '@/providers/shortcuts';
import { useAgentClis } from '@/state/agent-clis';
import { useCurrentProject, useProjects } from '@/state/projects';
import { DetectTerminals, OpenTerminal } from '@/wailsjs/go/main/App';
import type { terminal } from '@/wailsjs/go/models';

const INTEGRATION_ROUTES: Record<string, string> = {
  jira: '/jira',
  repository: '/repository',
  nodejs: '/nodejs',
  python: '/python',
  docker: '/docker',
  sentry: '/sentry',
};

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type Launch = { kind: 'addProject' } | { kind: 'addIntegration' } | { kind: 'settings' };

export function CommandPalette({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const { projects } = useProjects();
  const { project: currentProject, setProjectId } = useCurrentProject();
  const { shortcuts, isMac } = useShortcuts();
  const { kinds } = useAgentClis();
  const navigate = useNavigate();
  const modifier = isMac ? '⌘' : 'Ctrl';
  const installedKinds = kinds.filter((k) => k.installed);

  const [launch, setLaunch] = useState<Launch | null>(null);

  const run = (next: Launch | (() => void)) => () => {
    onOpenChange(false);
    if (typeof next === 'function') {
      next();
    } else {
      setLaunch(next);
    }
  };

  const [terminals, setTerminals] = useState<terminal.Terminal[]>([]);
  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    DetectTerminals()
      .then((res) => {
        if (!cancelled) {
          setTerminals(res ?? []);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [open]);
  const installedTerminals = terminals
    .filter((tt) => tt.installed)
    .slice()
    .sort((a, b) => Number(Boolean(b.default)) - Number(Boolean(a.default)));
  const projectPath = currentProject?.path ?? '';
  const launchTerminal = (id: string) => {
    if (!projectPath) {
      return;
    }
    OpenTerminal(id, projectPath).catch((err) => console.error(err));
  };

  const otherProjects = projects.filter((p) => p.id !== currentProject?.id);

  const connected = getIntegrations(currentProject);
  const integrationNav = INTEGRATIONS.filter((i) => {
    if (!INTEGRATION_ROUTES[i.id]) return false;
    if (i.id === 'repository') return currentProject?.hasGit === true;
    return Object.hasOwn(connected, i.id);
  });
  const hasAutomatable = projects.some(projectHasAutomatable);

  const { config: nodejsConfig, isRunning: nodejsRunning, startScript: nodejsStart, runCommand: nodejsRunCommand, stop: nodejsStop } = useNodejsRun();
  const nodejsActions = [
    { key: 'start', label: t('integrations.nodejs.startAction'), script: nodejsConfig?.startScript, icon: Play },
    { key: 'test', label: t('integrations.nodejs.testAction'), script: nodejsConfig?.testScript, icon: TestTube },
    { key: 'build', label: t('integrations.nodejs.buildAction'), script: nodejsConfig?.buildScript, icon: Hammer },
  ].filter((a) => a.script);

  const { config: pythonConfig, isRunning: pythonRunning, startScript: pythonStart, runCommand: pythonRunCommand, stop: pythonStop } = usePythonRun();
  const pythonActions = [
    { key: 'start', label: t('integrations.python.startAction'), script: pythonConfig?.startScript, icon: Play },
    { key: 'test', label: t('integrations.python.testAction'), script: pythonConfig?.testScript, icon: TestTube },
    { key: 'build', label: t('integrations.python.buildAction'), script: pythonConfig?.buildScript, icon: Hammer },
  ].filter((a) => a.script);

  const goTo = (to: string) => navigate({ to });
  const dismissLaunch = (o: boolean) => {
    if (!o) {
      setLaunch(null);
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent showCloseButton={false} className="overflow-hidden p-0 sm:max-w-[640px]">
          <DialogTitle className="sr-only">{t('commandPalette.title')}</DialogTitle>
          <DialogDescription className="sr-only">{t('commandPalette.description')}</DialogDescription>
          <Command>
            <CommandInput placeholder={t('commandPalette.placeholder')} />
            <CommandList>
              <CommandEmpty>{t('commandPalette.empty')}</CommandEmpty>

              <CommandGroup heading={t('commandPalette.groups.actions')}>
                {installedKinds.map((k, idx) => {
                  const Icon = k.icon;
                  return (
                    <CommandItem
                      key={k.id}
                      value={`new agent ${k.label}`}
                      onSelect={run(() => {
                        if (currentProject) {
                          void startDraftAgent(currentProject.id, { kindId: k.id });
                          navigate({ to: '/' });
                        }
                      })}
                    >
                      <Icon />
                      <span>
                        {t('commandPalette.actions.newAgent')} · {k.label}
                      </span>
                      {idx === 0 && <CommandShortcut keys={shortcutDisplayKeys(shortcuts.newAgent, modifier)} />}
                    </CommandItem>
                  );
                })}
                <CommandItem onSelect={run({ kind: 'addProject' })}>
                  <FolderPlus />
                  <span>{t('commandPalette.actions.addProject')}</span>
                  <CommandShortcut keys={shortcutDisplayKeys(shortcuts.addProject, modifier)} />
                </CommandItem>
                <CommandItem onSelect={run({ kind: 'addIntegration' })}>
                  <Plus />
                  <span>{t('commandPalette.actions.addIntegration')}</span>
                </CommandItem>
                <CommandItem onSelect={run({ kind: 'settings' })}>
                  <Settings />
                  <span>{t('commandPalette.actions.openSettings')}</span>
                </CommandItem>
              </CommandGroup>

              <CommandSeparator />
              <CommandGroup heading={t('commandPalette.groups.goTo')}>
                <CommandItem value="go agents" onSelect={run(() => goTo('/'))}>
                  <Bot />
                  <span>{t('sidebar.agents')}</span>
                </CommandItem>
                <CommandItem value="go files" onSelect={run(() => goTo('/files'))}>
                  <Files />
                  <span>{t('sidebar.files')}</span>
                </CommandItem>
                {integrationNav.map((i) => (
                  <CommandItem key={i.id} value={`go ${i.name}`} onSelect={run(() => goTo(INTEGRATION_ROUTES[i.id]))}>
                    <i.icon />
                    <span>{i.name}</span>
                  </CommandItem>
                ))}
                {hasAutomatable && (
                  <CommandItem value="go automations" onSelect={run(() => goTo('/automations'))}>
                    <Workflow />
                    <span>{t('sidebar.automations')}</span>
                  </CommandItem>
                )}
              </CommandGroup>

              {nodejsConfig?.manifestPath && nodejsActions.length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading="Node.js">
                    {nodejsActions.map((action) => (
                      <CommandItem key={action.key} value={`nodejs ${action.key} ${action.script}`} onSelect={run(() => nodejsStart(action.script!))} disabled={nodejsRunning}>
                        <action.icon />
                        <span>{action.label}</span>
                        <span className="ml-auto font-mono text-xs text-muted-foreground">{action.script}</span>
                      </CommandItem>
                    ))}
                    {nodejsRunning && (
                      <CommandItem value="nodejs stop" onSelect={run(() => nodejsStop())}>
                        <Square />
                        <span>{t('integrations.nodejs.stop')}</span>
                      </CommandItem>
                    )}
                    <CommandItem value="nodejs install packages" onSelect={run(() => nodejsRunCommand(['install'], t('integrations.nodejs.installAll')))} disabled={nodejsRunning}>
                      <Download />
                      <span>{t('integrations.nodejs.installAll')}</span>
                    </CommandItem>
                  </CommandGroup>
                </>
              )}

              {pythonConfig?.manifestPath && pythonActions.length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading="Python">
                    {pythonActions.map((action) => (
                      <CommandItem key={action.key} value={`python ${action.key} ${action.script}`} onSelect={run(() => pythonStart(action.script!))} disabled={pythonRunning}>
                        <action.icon />
                        <span>{action.label}</span>
                        <span className="ml-auto font-mono text-xs text-muted-foreground">{action.script}</span>
                      </CommandItem>
                    ))}
                    {pythonRunning && (
                      <CommandItem value="python stop" onSelect={run(() => pythonStop())}>
                        <Square />
                        <span>{t('integrations.python.stop')}</span>
                      </CommandItem>
                    )}
                    <CommandItem value="python install packages" onSelect={run(() => pythonRunCommand(installArgs(pythonConfig?.packageManager ?? 'pip'), t('integrations.python.installAll')))} disabled={pythonRunning}>
                      <Download />
                      <span>{t('integrations.python.installAll')}</span>
                    </CommandItem>
                  </CommandGroup>
                </>
              )}

              {projectPath && installedTerminals.length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading={t('commandPalette.groups.openTerminal')}>
                    {installedTerminals.map((tt) => (
                      <CommandItem key={tt.id} value={`terminal ${tt.name}`} onSelect={run(() => launchTerminal(tt.id))}>
                        <TerminalSquare />
                        <span>{tt.name}</span>
                        {tt.default && <span className="ml-auto rounded bg-accent px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">{t('sidebar.consoleDefault')}</span>}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </>
              )}

              {otherProjects.length > 0 && (
                <>
                  <CommandSeparator />
                  <CommandGroup heading={t('commandPalette.groups.switchProject')}>
                    {otherProjects.map((p) => (
                      <CommandItem key={p.id} value={`switch ${p.name}`} onSelect={run(() => setProjectId(p.id))}>
                        <ProjectAvatar project={p} className="size-5 rounded" textClassName="text-[9px]" />
                        <span>{p.name}</span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </DialogContent>
      </Dialog>

      {launch?.kind === 'addProject' && <AddProjectModal open={true} onOpenChange={dismissLaunch} />}
      {launch?.kind === 'addIntegration' && <AddIntegrationModal open={true} onOpenChange={dismissLaunch} />}
      {launch?.kind === 'settings' && <SettingsModal open={true} onOpenChange={dismissLaunch} />}
    </>
  );
}

function CommandShortcut({ keys }: { keys: string[] }) {
  return (
    <span className="ml-auto flex items-center gap-1 text-xs text-muted-foreground">
      {keys.map((k) => (
        <kbd key={k} className="inline-flex h-5 min-w-5 items-center justify-center rounded border bg-muted px-1 font-sans text-[10px] leading-none">
          {k}
        </kbd>
      ))}
    </span>
  );
}
