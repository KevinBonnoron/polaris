import { useLiveQuery } from '@tanstack/react-db';
import type { TFunction } from 'i18next';
import { GitBranch } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { resolveProviderIcon } from '@/components/brand-icons';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { ReadAgentLog } from '@/wailsjs/go/main/App';
import { AgentDetailFilesTab } from './agent-detail-files-tab';
import { AgentDetailLogsTab } from './agent-detail-logs-tab';
import { findAgentKind, OPENCODE_DESCRIPTOR } from './agent-kinds';
import { countToolsFromLog } from './agent-log-files';
import { AgentInputArea, NO_TOOLS_SENTINEL, TOOL_PRESETS } from './agent-input-area';
import { TokenStat } from './token-stat';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

const PRESET_I18N: Record<string, string> = {
  readonly: 'toolsReadonly',
  'no-web': 'toolsNoWeb',
  'no-tools': 'toolsNone',
};

function resolveToolsInfo(tools: string[], t: TFunction): { label: string; tooltip: string } | null {
  if (tools.length === 0) return null;
  if (tools[0] === NO_TOOLS_SENTINEL) return { label: t('agents.detail.toolsNone'), tooltip: t('agents.detail.toolsNoneDesc') };
  const toolSet = new Set(tools);
  for (const [key, preset] of Object.entries(TOOL_PRESETS)) {
    if (key === 'all' || key === 'no-tools') continue;
    if (preset.length === tools.length && preset.every((p) => toolSet.has(p))) {
      const i18nKey = PRESET_I18N[key];
      return { label: t(`agents.detail.${i18nKey}`), tooltip: tools.join(', ') };
    }
  }
  return { label: t('agents.detail.toolsCount', { count: tools.length }), tooltip: tools.join(', ') };
}

export function AgentConversation({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const { kinds: cliKinds, opencode } = useAgentClis();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));

  const { data = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const agent = data.find(({ id }) => id === agentId) ?? null;
  const status = agent?.status;
  const isDraft = status === 'draft';
  const isWorking = status === 'working';
  const filesModified = agent?.filesModified ?? 0;

  const provider = providers.find((p) => p.id === agent?.providerId) ?? undefined;
  const cliCfg = agent && !agent.providerId ? (agent.kind === 'opencode' ? opencode : cliKinds.find((k) => k.id === agent.kind)) : undefined;
  const models: { value: string; label: string }[] = cliCfg ? cliCfg.models : provider ? provider.models.map((m) => ({ value: m, label: m })) : [];
  const defaultsId = provider?.id ?? cliCfg?.id;
  const agentDefaults = useAgentDefaults();
  const spawnModel = (defaultsId ? agentDefaults.get(defaultsId) : undefined) ?? models[0]?.value ?? '';
  const activeModelValue = (isDraft ? spawnModel : agent?.model) ?? '';
  const activeModelLabel = models.find((m) => m.value === activeModelValue)?.label ?? activeModelValue;
  const KindIcon = useMemo(() => {
    if (!agent) return undefined;
    if (agent.providerId) {
      const resolved = resolveProviderIcon(provider?.icon);
      if (resolved) return resolved;
    }
    const kindCfg = findAgentKind(agent.kind) ?? (agent.kind === 'opencode' ? OPENCODE_DESCRIPTOR : undefined);
    return kindCfg?.icon;
  }, [agent, provider]);

  const [allowedTools, setAllowedTools] = useState<string[]>([]);
  const [log, setLog] = useState('');
  const [activeTab, setActiveTab] = useState('logs');
  const logRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (isDraft) {
      return;
    }

    let active = true;
    const tick = () => {
      ReadAgentLog(agentId)
        .then((s: string) => {
          if (active) {
            setLog(s);
          }
        })
        .catch(() => {});
    };
    stickToBottom.current = true;
    tick();
    if (status !== 'working') {
      return;
    }

    const id = window.setInterval(tick, 1000);
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
    ReadAgentLog(agentId).then(setLog).catch(() => {});
  }, [agentId]);

  const onSetActiveTab = useCallback((tab: string) => setActiveTab(tab), []);

  const onClearLog = useCallback(() => setLog(''), []);

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
              <span className="max-w-40 truncate font-mono">{agent.worktree.branch}</span>
            </div>
          )}
        </div>

        <TabsContent forceMount value="logs" className={cn('m-0 flex min-h-0 flex-col gap-3', activeTab !== 'logs' && 'hidden')}>
          <AgentDetailLogsTab log={log} isWorking={isWorking} logRef={logRef} onLogScroll={onLogScroll} />
        </TabsContent>

        <TabsContent forceMount value="files" className={cn('m-0 flex min-h-0 flex-1 flex-col', activeTab !== 'files' && 'hidden')}>
          <AgentDetailFilesTab agent={agent} />
        </TabsContent>
      </Tabs>

      <AgentInputArea
        agentId={agentId}
        agent={agent}
        inputRef={inputRef}
        onLogRefresh={onLogRefresh}
        onSetActiveTab={onSetActiveTab}
        onClearLog={onClearLog}
        allowedTools={allowedTools}
        onAllowedToolsChange={setAllowedTools}
      />

      {agent && (
        <div className="mt-2 flex shrink-0 flex-wrap items-center gap-1">
          {activeModelLabel && (
            <Badge variant="outline" className="text-muted-foreground">{activeModelLabel}</Badge>
          )}
          {(() => {
            const tools = isDraft ? allowedTools : (agent.allowedTools ?? []);
            if (cliCfg?.id !== 'claude-code') return null;
            const info = resolveToolsInfo(tools, t);
            if (!info) return null;
            return (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="outline" className="cursor-default text-muted-foreground">{info.label}</Badge>
                </TooltipTrigger>
                <TooltipContent side="top">{info.tooltip}</TooltipContent>
              </Tooltip>
            );
          })()}
          <Badge variant="outline" className="text-muted-foreground">
            <TokenStat tokens={displayTokens} parts={displayParts} className="tabular-nums" />
          </Badge>
          <Badge variant="outline" className="tabular-nums text-muted-foreground">{t('agents.detail.statTools', { count: toolsUsed })}</Badge>
          {displayCost > 0 && (
            <Badge variant="outline" className="tabular-nums text-muted-foreground">${displayCost.toFixed(4)}</Badge>
          )}
        </div>
      )}
    </div>
  );
}
