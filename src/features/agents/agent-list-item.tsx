import { Copy, ExternalLink, GitPullRequest, Square, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { agentsCollection } from '@/collections/agents.collection';
import { Button } from '@/components/ui/button';
import { formatTokens } from '@/lib/format';
import { agentStatusSeverity, SEVERITY_DOT } from '@/lib/severity';
import { formatRelative } from '@/lib/time';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import type { Agent } from '@/types';
import { CancelAgent, CreatePRForAgent, GetAgentGitState, ReadAgentLog } from '@/wailsjs/go/main/App';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { findAgentKind } from './agent-kinds';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

const KNOWN_STATUSES = new Set(['working', 'waiting', 'error', 'completed', 'idle']);

interface Props {
  agent: Agent;
  selected: boolean;
  onSelect: () => void;
}

export function AgentListItem({ agent, selected, onSelect }: Props) {
  const { t } = useTranslation();
  const kind = findAgentKind(agent.kind);
  const label = kind?.label ?? agent.kind;
  const isWorking = agent.status === 'working';
  const liveTokens = useLiveTokens(agent.id, tokenTotal(agent.tokens));
  const displayTokens = liveTokens?.tokens ?? tokenTotal(agent.tokens);
  const statusKey = KNOWN_STATUSES.has(agent.status) ? agent.status : 'idle';
  const statusLabel = t(`agents.status.${statusKey}`);

  const prUrl = agent.worktree?.prUrl ?? '';
  const hasBranch = Boolean(agent.worktree?.branch && agent.worktree?.path);
  const prDisabledReason = prUrl ? null : !hasBranch ? t('agents.detail.createPrDisabledNoBranch') : isWorking ? t('agents.detail.createPrDisabledStillWorking') : null;
  const prAvailable = !!prUrl || !prDisabledReason;
  const [creatingPr, setCreatingPr] = useState(false);

  const cancel = async () => {
    try {
      await CancelAgent(agent.id);
    } catch (err) {
      toastError({ title: t('agents.card.couldNotStop'), err });
    }
  };

  const remove = async () => {
    if (agent.worktree?.branch && agent.worktree?.path) {
      try {
        const state = await GetAgentGitState(agent.id);
        const hasUnpushed = (state?.aheadCount ?? 0) > 0;
        const hasStaged = (state?.stagedCount ?? 0) > 0;
        if (hasUnpushed || hasStaged) {
          const ok = window.confirm(t('agents.card.deleteWithUnpushedConfirm', { branch: agent.worktree.branch }));
          if (!ok) {
            return;
          }
        }
      } catch {
        // If we can't read git state, fall through to delete — better than blocking
      }
    }
    try {
      await agentsCollection.delete(agent.id);
    } catch (err) {
      toastError({ title: t('agents.card.couldNotDelete'), err });
    }
  };

  const copyLog = async () => {
    try {
      const text = await ReadAgentLog(agent.id);
      await navigator.clipboard.writeText(text);
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotCopy'), err });
    }
  };

  const openOrCreatePr = async () => {
    if (creatingPr) {
      return;
    }
    if (prUrl) {
      BrowserOpenURL(prUrl);
      return;
    }
    setCreatingPr(true);
    const toastId = toast.loading(t('agents.detail.createPrInProgress'));
    try {
      const url = await CreatePRForAgent(agent.id);
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
  };

  return (
    <div className="group relative">
      <button type="button" onClick={onSelect} className={cn('flex w-full flex-col gap-1 rounded-md px-3 py-2.5 text-left transition-colors', selected ? 'bg-accent' : 'hover:bg-accent/50')}>
        <div className="flex items-center gap-2">
          <span className={cn('size-2 shrink-0 rounded-full', SEVERITY_DOT[agentStatusSeverity(statusKey)], statusKey === 'working' && 'animate-pulse')} aria-hidden />
          <span className="truncate text-xs font-semibold">{label}</span>
          <span className="shrink-0 text-[11px] text-muted-foreground">· {statusLabel}</span>
        </div>
        {agent.summary && (
          <p className="truncate text-xs text-muted-foreground" title={agent.summary}>
            {agent.summary}
          </p>
        )}
        <div className="flex items-center justify-between text-[10px] text-muted-foreground">
          <span>{t('agents.card.started', { when: formatRelative(agent.startedAt) })}</span>
          <span className="tabular-nums">{t('agents.card.tokens', { count: formatTokens(displayTokens) })}</span>
        </div>
      </button>

      <div className="absolute right-2 top-2 hidden items-center gap-0.5 rounded-md bg-background/80 backdrop-blur-sm group-hover:flex">
        {prAvailable && (
          <Button variant="ghost" size="icon" onClick={() => void openOrCreatePr()} disabled={creatingPr} className="size-6" title={prUrl ? t('agents.detail.openPr') : t('agents.detail.createPr')}>
            {prUrl ? <ExternalLink className="size-3" /> : <GitPullRequest className="size-3" />}
          </Button>
        )}
        <Button variant="ghost" size="icon" onClick={() => void copyLog()} className="size-6" title={t('agents.detail.copyLog')}>
          <Copy className="size-3" />
        </Button>
        {isWorking ? (
          <Button variant="ghost" size="icon" onClick={() => void cancel()} className="size-6" title={t('agents.card.stop')}>
            <Square className="size-3" />
          </Button>
        ) : (
          <Button variant="ghost" size="icon" onClick={() => void remove()} className="size-6 text-muted-foreground hover:text-destructive" title={t('agents.card.delete')}>
            <Trash2 className="size-3" />
          </Button>
        )}
      </div>
    </div>
  );
}
