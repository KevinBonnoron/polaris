import { useLiveQuery } from '@tanstack/react-db';
import type { TFunction } from 'i18next';
import { Check, GitBranch, Play, Square } from 'lucide-react';
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { resolveProviderIcon } from '@/components/brand-icons';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useNodejsRun } from '@/features/integrations/nodejs/nodejs-run-context';
import { usePythonRun } from '@/features/integrations/python/python-run-context';
import { isSeparator, isUsageBlock, sepGlyph, usageMode } from '@/features/settings/status-bar-settings';
import { cn } from '@/lib/utils';
import { useStatusBarBlocks } from '@/providers/theme-accent';
import { useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { useCurrentProject } from '@/state/projects';
import { ReadAgentLog, SetAgentModel } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { AgentDetailFilesTab } from './agent-detail-files-tab';
import { AgentDetailLogsTab } from './agent-detail-logs-tab';
import { AgentInputArea, NO_TOOLS_SENTINEL, TOOL_PRESETS } from './agent-input-area';
import { findAgentKind, OPENCODE_DESCRIPTOR } from './agent-kinds';
import { countToolsFromLog } from './agent-log-files';
import { useClaudeUsage } from './claude-usage-bar';
import { TokenStat } from './token-stat';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

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
  const activeRun = nodejs.isRunning ? nodejs : python.isRunning ? python : nodejs.config?.startScript ? nodejs : python.config?.startScript ? python : null;

  const devManifestPath = useMemo(() => {
    const manifest = activeRun?.config?.manifestPath;
    const worktreePath = agent?.worktree?.path;
    const projectPath = project?.path;
    if (!manifest || !worktreePath || !projectPath) {
      return undefined;
    }

    if (!manifest.startsWith(projectPath)) {
      return undefined;
    }

    return worktreePath + manifest.slice(projectPath.length);
  }, [activeRun?.config?.manifestPath, agent?.worktree?.path, project?.path]);

  const onDevRun = useCallback(() => {
    if (!activeRun) {
      return;
    }

    if (activeRun.isRunning) {
      void activeRun.stop();
    } else if (activeRun.config?.startScript && devManifestPath) {
      void activeRun.startScript(activeRun.config.startScript, devManifestPath);
    }
  }, [activeRun, devManifestPath]);

  const [modelPopoverOpen, setModelPopoverOpen] = useState(false);
  const [allowedTools, setAllowedTools] = useState<string[]>([]);
  const [log, setLog] = useState<polaris.StreamEvent[]>([]);
  const [logLoading, setLogLoading] = useState(true);
  const [activeTab, setActiveTab] = useState('logs');
  const logRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (isDraft) {
      setLogLoading(false);
      return;
    }

    let active = true;
    const tick = (first = false) => {
      ReadAgentLog(agentId)
        .then((evts) => {
          if (active) {
            setLog(evts ?? []);
            if (first) {
              setLogLoading(false);
            }
          }
        })
        .catch(() => {
          if (active && first) {
            setLogLoading(false);
          }
        });
    };
    setLogLoading(true);
    stickToBottom.current = true;
    tick(true);
    if (status !== 'working') {
      return;
    }

    const id = window.setInterval(() => tick(false), 1000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, [agentId, status, isDraft]);

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
    ReadAgentLog(agentId)
      .then(setLog)
      .catch(() => {});
  }, [agentId]);

  const onSetActiveTab = useCallback((tab: string) => setActiveTab(tab), []);

  const onClearLog = useCallback(() => setLog([]), []);

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
  const duration = agent && !isDraft ? (agent.updatedAt ?? Date.now()) - agent.startedAt : 0;
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
    <div className="flex h-full min-h-0 flex-col">
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
          {agent?.worktree?.branch && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <GitBranch className="size-3 shrink-0" />
              <span className="font-mono">{agent.worktree.branch}</span>
              {activeRun && (activeRun.isRunning || devManifestPath) && (
                <Button size="sm" variant={activeRun.isRunning ? 'destructive' : 'secondary'} onClick={onDevRun} className="ml-1 h-6 px-2 text-xs">
                  {activeRun.isRunning ? <Square className="mr-1 size-3 fill-current" /> : <Play className="mr-1 size-3 fill-current" />}
                  {activeRun.isRunning ? t('agents.detail.stopServer') : t('agents.detail.runServer')}
                </Button>
              )}
            </div>
          )}
        </div>

        <TabsContent forceMount value="logs" className={cn('m-0 flex min-h-0 flex-col gap-3', activeTab !== 'logs' && 'hidden')}>
          <AgentDetailLogsTab log={log} isWorking={isWorking} isLoading={logLoading} logRef={logRef} onLogScroll={onLogScroll} />
        </TabsContent>

        <TabsContent forceMount value="files" className={cn('m-0 flex min-h-0 flex-1 flex-col', activeTab !== 'files' && 'hidden')}>
          <AgentDetailFilesTab agent={agent} onCountChange={setLiveFileCount} />
        </TabsContent>
      </Tabs>

      <AgentInputArea agentId={agentId} agent={agent} inputRef={inputRef} onLogRefresh={onLogRefresh} onSetActiveTab={onSetActiveTab} onClearLog={onClearLog} allowedTools={allowedTools} onAllowedToolsChange={setAllowedTools} />

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
