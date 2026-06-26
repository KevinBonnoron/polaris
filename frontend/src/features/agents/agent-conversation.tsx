import { useLiveQuery } from '@tanstack/react-db';
import type { TFunction } from 'i18next';
import { Check, ChevronDown, ExternalLink, GitBranch, GitBranchPlus, GitPullRequest, Play, Square } from 'lucide-react';
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { projectsCollection } from '@/collections/projects.collection';
import { resolveProviderIcon } from '@/components/brand-icons';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { RunContext } from '@/features/integrations/create-run-context';
import { useCSharpRun } from '@/features/integrations/csharp/csharp-run-context';
import { useGodotRun } from '@/features/integrations/godot/godot-run-context';
import { useNodejsRun } from '@/features/integrations/nodejs/nodejs-run-context';
import { getLaunchTarget, withLaunchTarget } from '@/features/integrations/project-integrations';
import { usePythonRun } from '@/features/integrations/python/python-run-context';
import { useTaskfileRun } from '@/features/integrations/taskfile/taskfile-run-context';
import { isSeparator, isUsageBlock, sepGlyph, usageMode } from '@/features/settings/status-bar-settings';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { useStatusBarBlocks } from '@/providers/theme-accent';
import { useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { useCurrentProject } from '@/state/projects';
import { CreatePRForAgent, PromoteAgentToWorktree, ReadAgentLogFrom, SetAgentModel } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { BrowserOpenURL, EventsOn } from '@/wailsjs/runtime/runtime';
import { AgentDetailFilesTab } from './agent-detail-files-tab';
import { AgentDetailLogsTab } from './agent-detail-logs-tab';
import { AgentInputArea, NO_TOOLS_SENTINEL, TOOL_PRESETS } from './agent-input-area';
import { findAgentKind, OPENCODE_DESCRIPTOR } from './agent-kinds';
import { countToolsFromLog } from './agent-log-files';
import { useClaudeUsage } from './claude-usage-bar';
import { TokenStat } from './token-stat';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

const LAUNCH_KIND_LABEL: Record<string, string> = {
  nodejs: 'Node.js',
  python: 'Python',
  csharp: 'C#',
  taskfile: 'Taskfile',
  godot: 'Godot',
};

const PRESET_I18N: Record<string, string> = {
  readonly: 'toolsReadonly',
  'no-web': 'toolsNoWeb',
  'no-tools': 'toolsNone',
};

function resolveToolsInfo(tools: string[], t: TFunction): { label: string; tooltip: string } | null {
  if (tools.length === 0) {
    return null;
  }
  if (tools[0] === NO_TOOLS_SENTINEL) {
    return { label: t('agents.detail.toolsNone'), tooltip: t('agents.detail.toolsNoneDesc') };
  }
  const toolSet = new Set(tools);
  for (const [key, preset] of Object.entries(TOOL_PRESETS)) {
    if (key === 'all' || key === 'no-tools') {
      continue;
    }
    if (preset.length === tools.length && preset.every((p) => toolSet.has(p))) {
      const i18nKey = PRESET_I18N[key];
      return { label: t(`agents.detail.${i18nKey}`), tooltip: tools.join(', ') };
    }
  }
  return { label: t('agents.detail.toolsCount', { count: tools.length }), tooltip: tools.join(', ') };
}

function formatDuration(ms: number): string {
  const secs = Math.floor(ms / 1000);
  if (secs < 60) {
    return `${secs}s`;
  }
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  if (mins < 60) {
    return `${mins}m${rem > 0 ? ` ${rem}s` : ''}`;
  }
  const hrs = Math.floor(mins / 60);
  return `${hrs}h ${mins % 60}m`;
}

export function AgentConversation({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const { kinds: cliKinds, opencode } = useAgentClis();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const { blocks: visibleBlocks } = useStatusBarBlocks();
  const { project } = useCurrentProject();

  const { data = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const agent = data.find(({ id }) => id === agentId) ?? null;
  const status = agent?.status;
  const isDraft = status === 'draft';
  const isWorking = status === 'working';
  const [liveFileCount, setLiveFileCount] = useState<number | null>(null);
  useEffect(() => {
    setLiveFileCount(null);
  }, [agentId]);
  const filesModified = liveFileCount ?? agent?.filesModified ?? 0;

  const provider = providers.find((p) => p.id === agent?.providerId) ?? undefined;
  const cliCfg = agent && !agent.providerId ? (agent.kind === 'opencode' ? opencode : cliKinds.find((k) => k.id === agent.kind)) : undefined;
  const models: { value: string; label: string }[] = cliCfg ? cliCfg.models : provider ? provider.models.map((m) => ({ value: m, label: m })) : [];
  const defaultsId = provider?.id ?? cliCfg?.id;
  const agentDefaults = useAgentDefaults();
  const spawnModel = (defaultsId ? agentDefaults.get(defaultsId) : undefined) ?? models[0]?.value ?? '';
  const activeModelValue = (isDraft ? (agent?.model ?? spawnModel) : agent?.model) ?? '';
  const activeModelLabel = models.find((m) => m.value === activeModelValue)?.label ?? activeModelValue;
  const KindIcon = useMemo(() => {
    if (!agent) {
      return undefined;
    }
    if (agent.providerId) {
      const resolved = resolveProviderIcon(provider?.icon);
      if (resolved) {
        return resolved;
      }
    }
    const kindCfg = findAgentKind(agent.kind) ?? (agent.kind === 'opencode' ? OPENCODE_DESCRIPTOR : undefined);
    return kindCfg?.icon;
  }, [agent, provider]);

  const nodejs = useNodejsRun();
  const python = usePythonRun();
  const csharp = useCSharpRun();
  const taskfile = useTaskfileRun();
  const godot = useGodotRun();
  const runs = useMemo<RunContext[]>(() => [nodejs, python, csharp, taskfile, godot], [nodejs, python, csharp, taskfile, godot]);

  const launchCandidates = useMemo(
    () =>
      runs.flatMap((r) =>
        r.instances.map((inst, index) => ({ run: r, kind: r.kind, index, name: (inst as Record<string, unknown>)[r.startKey] as string | undefined, manifestPath: inst.manifestPath })).filter((c): c is { run: RunContext; kind: string; index: number; name: string; manifestPath: string | undefined } => Boolean(c.name)),
      ),
    [runs],
  );

  const launchTarget = getLaunchTarget(project);
  const runningRun = runs.find((r) => r.isRunning) ?? null;
  const selectedCandidate = useMemo(() => {
    const byTarget = launchTarget ? launchCandidates.find((c) => c.kind === launchTarget.kind && c.index === launchTarget.instanceIndex) : undefined;
    return byTarget ?? launchCandidates[0] ?? null;
  }, [launchTarget, launchCandidates]);

  const activeRun = runningRun ?? selectedCandidate?.run ?? null;
  const activeStartName = runningRun ? runningRun.startName : selectedCandidate?.name;

  useEffect(() => {
    if (!runningRun && selectedCandidate && selectedCandidate.run.instanceIndex !== selectedCandidate.index) {
      selectedCandidate.run.setInstanceIndex(selectedCandidate.index);
    }
  }, [runningRun, selectedCandidate]);

  const onSelectLaunchTarget = useCallback(
    (kind: string, index: number) => {
      if (!project) {
        return;
      }
      const next = withLaunchTarget(project, { kind, instanceIndex: index });
      projectsCollection.update(project.id, (d) => {
        d.integrations = next;
      });
    },
    [project],
  );

  const launchOptions = useMemo(() => {
    const counts = launchCandidates.reduce<Record<string, number>>((acc, c) => {
      acc[c.kind] = (acc[c.kind] ?? 0) + 1;
      return acc;
    }, {});
    return launchCandidates.map((c) => {
      const base = `${LAUNCH_KIND_LABEL[c.kind] ?? c.kind} · ${c.name}`;
      const manifestBase = c.manifestPath?.split(/[\\/]/).pop();
      const suffix = (counts[c.kind] ?? 0) > 1 && manifestBase ? ` (${manifestBase})` : '';
      return { ...c, label: base + suffix };
    });
  }, [launchCandidates]);

  const devManifestPath = useMemo(() => {
    const manifest = activeRun?.config?.manifestPath;
    if (!manifest) {
      return undefined;
    }
    const worktreePath = agent?.worktree?.path;
    if (!worktreePath) {
      return manifest;
    }
    const projectPath = project?.path;
    if (!projectPath || !manifest.startsWith(projectPath)) {
      return undefined;
    }
    return worktreePath + manifest.slice(projectPath.length);
  }, [activeRun?.config?.manifestPath, agent?.worktree?.path, project?.path]);

  const thisAgentRunning = activeRun?.isRunning === true && activeRun.run?.agentId === agentId;

  const onDevRun = useCallback(() => {
    if (!activeRun) {
      return;
    }

    if (activeRun.isRunning) {
      void activeRun.stop();
    } else if (activeStartName && devManifestPath) {
      void activeRun.startScript(activeStartName, devManifestPath, agentId);
    }
  }, [activeRun, activeStartName, devManifestPath, agentId]);

  const prUrl = agent?.worktree?.prUrl ?? '';
  const hasBranch = Boolean(agent?.worktree?.branch && agent?.worktree?.path);
  const prAvailable = !!prUrl || (hasBranch && !isWorking);
  const [creatingPr, setCreatingPr] = useState(false);

  const openOrCreatePr = useCallback(async () => {
    if (creatingPr) return;
    if (prUrl) {
      BrowserOpenURL(prUrl);
      return;
    }
    setCreatingPr(true);
    const toastId = toast.loading(t('agents.detail.createPrInProgress'));
    try {
      const url = await CreatePRForAgent(agentId);
      toast.success(t('agents.detail.createPrSuccess'), {
        id: toastId,
        action: url ? { label: t('agents.detail.openPr'), onClick: () => BrowserOpenURL(url) } : undefined,
      });
    } catch (err) {
      toast.dismiss(toastId);
      toastError({ title: t('agents.detail.createPrFailed'), err });
    } finally {
      setCreatingPr(false);
    }
  }, [agentId, creatingPr, prUrl, t]);

  const [modelPopoverOpen, setModelPopoverOpen] = useState(false);
  const [promoteOpen, setPromoteOpen] = useState(false);
  const [promoteBranch, setPromoteBranch] = useState('');
  const [promoteLoading, setPromoteLoading] = useState(false);
  const [allowedTools, setAllowedTools] = useState<string[]>([]);

  useEffect(() => {
    setPromoteOpen(false);
    setPromoteBranch('');
    setPromoteLoading(false);
  }, [agentId]);

  const [log, setLog] = useState<polaris.StreamEvent[]>([]);
  const logOffset = useRef(0);
  const [logLoading, setLogLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('logs');
  const logRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // The live log appends only the new tail (read incrementally by byte offset)
  // whenever the backend signals an append, instead of re-reading and
  // re-rendering the whole transcript on a timer. A fast-streaming agent could
  // otherwise saturate the renderer and freeze the UI as its log grew.
  useEffect(() => {
    if (isDraft) {
      setLog([]);
      logOffset.current = 0;
      setLogLoading(false);
      return;
    }

    let active = true;
    let reading = false;
    let pendingPull = false;
    logOffset.current = 0;
    setLog([]);
    setLogLoading(true);
    stickToBottom.current = true;

    const pull = (first = false) => {
      if (!active) {
        return;
      }
      if (reading) {
        // A signal arrived mid-read; coalesce it into a follow-up so the final
        // tail is never dropped.
        pendingPull = true;
        return;
      }
      reading = true;
      ReadAgentLogFrom(agentId, logOffset.current)
        .then((tail) => {
          if (!active) {
            return;
          }
          logOffset.current = tail.offset;
          const evts = tail.events ?? [];
          if (evts.length > 0) {
            setLog((prev) => [...prev, ...evts]);
          }
        })
        .catch(() => {})
        .finally(() => {
          reading = false;
          if (active && first) {
            setLogLoading(false);
          }
          if (active && pendingPull) {
            pendingPull = false;
            pull(false);
          }
        });
    };

    pull(true);
    const unsubscribe = EventsOn('agent:log:appended', (payload: { agentId: string }) => {
      if (payload.agentId === agentId) {
        pull(false);
      }
    });
    return () => {
      active = false;
      unsubscribe();
    };
  }, [agentId, isDraft]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: log is the trigger; body reads ref only
  useEffect(() => {
    const el = logRef.current;
    if (el && stickToBottom.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [log]);

  const onLogScroll = useCallback(() => {
    const el = logRef.current;
    if (!el) {
      return;
    }
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  }, []);

  const onLogRefresh = useCallback(() => {
    logOffset.current = 0;
    ReadAgentLogFrom(agentId, 0)
      .then((tail) => {
        logOffset.current = tail.offset;
        setLog(tail.events ?? []);
      })
      .catch(() => {});
  }, [agentId]);

  const onSetActiveTab = useCallback((tab: string) => setActiveTab(tab), []);

  const onClearLog = useCallback(() => {
    logOffset.current = 0;
    setLog([]);
  }, []);

  const onPromoteConfirm = useCallback(async () => {
    if (!agent?.id || !promoteBranch.trim() || promoteLoading) {
      return;
    }
    setPromoteLoading(true);
    try {
      await PromoteAgentToWorktree(agent.id, promoteBranch.trim());
      setPromoteOpen(false);
      setPromoteBranch('');
    } catch (err) {
      toastError({ title: t('agents.detail.promoteWorktreeError'), err });
    } finally {
      setPromoteLoading(false);
    }
  }, [agent?.id, promoteBranch, promoteLoading, t]);

  const toolsUsed = useMemo(() => countToolsFromLog(log), [log]);
  const liveTokens = useLiveTokens(agentId, tokenTotal(agent?.tokens));
  const displayTokens = liveTokens?.tokens ?? tokenTotal(agent?.tokens);
  const displayCost = liveTokens?.costUsd ?? agent?.costUsd ?? 0;
  const displayParts = liveTokens?.parts ??
    agent?.tokens ?? {
      input: 0,
      output: 0,
      cacheCreation: 0,
      cacheRead: 0,
    };

  const providerLabel = provider?.name ?? (cliCfg ? (findAgentKind(agent?.kind ?? '')?.label ?? cliCfg.id) : null);
  const duration = agent && !isDraft ? (agent.updatedAt ?? agent.startedAt) - agent.startedAt : 0;
  const { usage: claudeUsage } = useClaudeUsage();

  const blockRenderers: Record<string, () => ReactNode> = useMemo(
    () => ({
      model: () => {
        if (!activeModelLabel) {
          return null;
        }

        if (models.length > 1) {
          return (
            <Popover key="model" open={modelPopoverOpen} onOpenChange={setModelPopoverOpen}>
              <PopoverTrigger asChild>
                <Badge variant="outline" className="cursor-pointer text-muted-foreground hover:bg-accent">
                  {activeModelLabel}
                </Badge>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-auto min-w-48 p-1">
                {models.map((m) => (
                  <button
                    key={m.value}
                    type="button"
                    onClick={() => {
                      if (isDraft) {
                        agentsCollection.update(agentId, (d) => {
                          d.model = m.value;
                        });
                      } else {
                        void SetAgentModel(agentId, m.value).catch(() => {});
                      }
                      setModelPopoverOpen(false);
                    }}
                    className={cn('flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent', m.value === activeModelValue && 'bg-accent')}
                  >
                    <span className="flex-1">{m.label}</span>
                    {m.value === activeModelValue && <Check className="size-3.5 shrink-0 text-primary" />}
                  </button>
                ))}
              </PopoverContent>
            </Popover>
          );
        }
        return (
          <Badge key="model" variant="outline" className="text-muted-foreground">
            {activeModelLabel}
          </Badge>
        );
      },
      tools: () => {
        const tools = isDraft ? allowedTools : (agent?.allowedTools ?? []);
        if (cliCfg?.id !== 'claude-code') {
          return null;
        }

        const info = resolveToolsInfo(tools, t);
        if (!info) {
          return null;
        }

        return (
          <Tooltip key="tools">
            <TooltipTrigger asChild>
              <Badge variant="outline" className="cursor-default text-muted-foreground">
                {info.label}
              </Badge>
            </TooltipTrigger>
            <TooltipContent side="top">{info.tooltip}</TooltipContent>
          </Tooltip>
        );
      },
      tokens: () => (
        <Badge key="tokens" variant="outline" className="text-muted-foreground">
          <TokenStat tokens={displayTokens} parts={displayParts} className="tabular-nums" />
        </Badge>
      ),
      'tools-used': () => (
        <Badge key="tools-used" variant="outline" className="tabular-nums text-muted-foreground">
          {t('agents.detail.statTools', { count: toolsUsed })}
        </Badge>
      ),
      cost: () => {
        if (displayCost <= 0) {
          return null;
        }

        return (
          <Badge key="cost" variant="outline" className="tabular-nums text-muted-foreground">
            ${displayCost.toFixed(4)}
          </Badge>
        );
      },
      provider: () => {
        if (!providerLabel) {
          return null;
        }

        return (
          <Badge key="provider" variant="outline" className="text-muted-foreground">
            {providerLabel}
          </Badge>
        );
      },
      files: () => {
        if (filesModified <= 0) {
          return null;
        }

        return (
          <Badge key="files" variant="outline" className="tabular-nums text-muted-foreground">
            {t('agents.detail.files')} {filesModified}
          </Badge>
        );
      },
      duration: () => {
        if (duration <= 0) {
          return null;
        }

        return (
          <Badge key="duration" variant="outline" className="tabular-nums text-muted-foreground">
            {formatDuration(duration)}
          </Badge>
        );
      },
      usage: () => null,
    }),
    [activeModelLabel, activeModelValue, isDraft, models, modelPopoverOpen, agentId, allowedTools, agent, cliCfg, t, displayTokens, displayParts, toolsUsed, displayCost, providerLabel, filesModified, duration],
  );

  return (
    <div className="flex h-full min-h-0 flex-col" data-file-drop-target data-agent-id={agentId}>
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex shrink-0 items-center justify-between border-b border-border">
          <div className="flex items-center">
            {KindIcon && <KindIcon className="mr-2 size-4 shrink-0 text-muted-foreground" />}
            <TabsList variant="line" className="shrink-0">
              <TabsTrigger value="logs">{t('agents.detail.logs')}</TabsTrigger>
              <TabsTrigger value="files">
                {t('agents.detail.files')}
                {filesModified > 0 && (
                  <Badge variant="secondary" className="ml-0.5 h-4 min-w-4 px-1.5 text-[10px] tabular-nums">
                    {filesModified}
                  </Badge>
                )}
              </TabsTrigger>
            </TabsList>
          </div>
          <div className="flex items-center gap-1">
            {(agent?.worktree?.branch ?? project?.branch) && (
              <div className="flex items-center gap-1 text-xs text-muted-foreground">
                <GitBranch className="size-3 shrink-0" />
                <span className="font-mono">{agent?.worktree?.branch ?? project?.branch}</span>
                {prAvailable && (
                  <Button size="icon" variant="ghost" onClick={() => void openOrCreatePr()} disabled={creatingPr} className="ml-0.5 size-6" title={prUrl ? t('agents.detail.openPr') : t('agents.detail.createPr')}>
                    {prUrl ? <ExternalLink className="size-3" /> : <GitPullRequest className="size-3" />}
                  </Button>
                )}
              </div>
            )}
            {activeRun && (thisAgentRunning || (!activeRun.isRunning && activeStartName && devManifestPath)) && (
              <div className="ml-1 flex items-center">
                <Button size="sm" variant={thisAgentRunning ? 'destructive' : 'secondary'} onClick={onDevRun} className={cn('h-6 px-2 text-xs', !activeRun.isRunning && launchOptions.length > 1 && 'rounded-r-none')}>
                  {thisAgentRunning ? <Square className="mr-1 size-3 fill-current" /> : <Play className="mr-1 size-3 fill-current" />}
                  {thisAgentRunning ? t('agents.detail.stopServer') : t('agents.detail.runServer')}
                </Button>
                {!activeRun.isRunning && launchOptions.length > 1 && (
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button size="icon" variant="secondary" className="h-6 w-5 rounded-l-none border-l border-border/50" title={t('agents.detail.selectLaunchTarget')}>
                        <ChevronDown className="size-3" />
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent align="end" className="w-56 p-1">
                      {launchOptions.map((o) => {
                        const isActive = o.kind === selectedCandidate?.kind && o.index === selectedCandidate?.index;
                        return (
                          <button type="button" key={`${o.kind}:${o.index}`} onClick={() => onSelectLaunchTarget(o.kind, o.index)} className={cn('flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-accent', isActive && 'font-medium')}>
                            <Check className={cn('size-3 shrink-0', !isActive && 'opacity-0')} />
                            <span className="truncate">{o.label}</span>
                          </button>
                        );
                      })}
                    </PopoverContent>
                  </Popover>
                )}
              </div>
            )}
            {!agent?.worktree?.branch && agent && project?.hasGit && !isDraft && filesModified > 0 && (
              <Button size="sm" variant="ghost" onClick={() => setPromoteOpen(true)} className="h-6 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground">
                <GitBranchPlus className="size-3 shrink-0" />
                {t('agents.detail.promoteWorktree')}
              </Button>
            )}
          </div>
        </div>

        <TabsContent forceMount value="logs" className={cn('m-0 flex min-h-0 flex-col gap-3', activeTab !== 'logs' && 'hidden')}>
          <AgentDetailLogsTab log={log} isWorking={isWorking} isLoading={logLoading} logRef={logRef} onLogScroll={onLogScroll} />
        </TabsContent>

        <TabsContent forceMount value="files" className={cn('m-0 flex min-h-0 flex-1 flex-col', activeTab !== 'files' && 'hidden')}>
          <AgentDetailFilesTab agent={agent} onCountChange={setLiveFileCount} />
        </TabsContent>
      </Tabs>

      <AgentInputArea agentId={agentId} agent={agent} inputRef={inputRef} onLogRefresh={onLogRefresh} onSetActiveTab={onSetActiveTab} onClearLog={onClearLog} allowedTools={allowedTools} onAllowedToolsChange={setAllowedTools} />

      <Dialog open={promoteOpen} onOpenChange={setPromoteOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('agents.detail.promoteWorktreeTitle')}</DialogTitle>
          </DialogHeader>
          <Input
            placeholder={t('agents.detail.promoteWorktreePlaceholder')}
            value={promoteBranch}
            onChange={(e) => setPromoteBranch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                void onPromoteConfirm();
              }
            }}
            autoFocus
          />
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPromoteOpen(false)} disabled={promoteLoading}>
              {t('common.cancel')}
            </Button>
            <Button onClick={() => void onPromoteConfirm()} disabled={!promoteBranch.trim() || promoteLoading}>
              {t('agents.detail.promoteWorktreeConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {agent && (
        <div className="mt-2 flex shrink-0 flex-wrap items-center gap-1">
          {visibleBlocks.map((blockId, idx) => {
            if (isSeparator(blockId)) {
              return (
                <span key={`${blockId}-${idx}`} className="text-xs text-muted-foreground/60">
                  {sepGlyph(blockId)}
                </span>
              );
            }
            if (isUsageBlock(blockId)) {
              if (!claudeUsage || cliCfg?.id !== 'claude-code') {
                return null;
              }

              const pct = claudeUsage.sessionPercentUsed;
              const mode = usageMode(blockId);
              const display = mode === 'remaining' ? 100 - pct : pct;
              const label = mode === 'remaining' ? t('agents.usage.left') : t('agents.usage.used');
              return (
                <Tooltip key={`usage-${idx}`}>
                  <TooltipTrigger asChild>
                    <Badge variant="outline" className="cursor-default tabular-nums text-muted-foreground">
                      <span className={cn('mr-1 inline-block size-1.5 rounded-full', pct >= 90 ? 'bg-red-400' : pct >= 70 ? 'bg-amber-400' : 'bg-emerald-400')} />
                      <span className={pct >= 90 ? 'text-red-400' : pct >= 70 ? 'text-amber-400' : 'text-emerald-400'}>{display}%</span>
                      <span className="ml-1">{label}</span>
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent side="top">
                    {t('agents.usage.session')}: {display}% {label}
                  </TooltipContent>
                </Tooltip>
              );
            }
            return blockRenderers[blockId]?.();
          })}
        </div>
      )}
    </div>
  );
}
