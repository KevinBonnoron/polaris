import { Copy, ExternalLink, GitPullRequest, Square, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { agentsCollection } from '@/db';
import { formatTokens } from '@/lib/format';
import { formatRelative } from '@/lib/time';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import type { Agent } from '@/types';
import { CancelAgent, CreatePRForAgent, GetAgentGitState, ReadAgentLog } from '@/wailsjs/go/main/App';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { AgentDetailModal } from './agent-detail-modal';
import { findAgentKind } from './agent-kinds';
import { useLiveTokens } from './use-live-tokens';

interface Props {
  agent: Agent;
}

const STATUS_DOT: Record<string, string> = {
  working: 'bg-blue-500 animate-pulse',
  waiting: 'bg-amber-500',
  error: 'bg-red-500',
  completed: 'bg-emerald-500',
  idle: 'bg-muted-foreground',
};

export function AgentCard({ agent }: Props) {
  const { t } = useTranslation();
  const kind = findAgentKind(agent.kind);
  const label = kind?.label ?? agent.kind;
  const isWorking = agent.status === 'working';
  const liveTokens = useLiveTokens(agent.id, agent.tokens);
  const displayTokens = liveTokens?.tokens ?? agent.tokens;
  const statusKey = (STATUS_DOT[agent.status] ? agent.status : 'idle') as keyof typeof STATUS_DOT;
  const statusLabel = t(`agents.status.${statusKey}`);

  const prUrl = agent.prUrl ?? '';
  const hasBranch = Boolean(agent.branch && agent.worktreePath);
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
    if (agent.branch && agent.worktreePath) {
      try {
        const state = await GetAgentGitState(agent.id);
        const hasUnpushed = (state?.aheadCount ?? 0) > 0;
        const hasStaged = (state?.stagedCount ?? 0) > 0;
        if (hasUnpushed || hasStaged) {
          const ok = window.confirm(t('agents.card.deleteWithUnpushedConfirm', { branch: agent.branch }));
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

  const stopClick = (e: React.MouseEvent) => {
    e.stopPropagation();
  };

  return (
    <AgentDetailModal agentId={agent.id}>
      <Card className="flex cursor-pointer flex-col gap-2 overflow-hidden py-3 transition-colors hover:bg-accent/30">
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0 px-3">
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <span className={cn('size-2 shrink-0 rounded-full', STATUS_DOT[statusKey])} aria-hidden />
            <span className="truncate text-xs font-semibold">{label}</span>
            <span className="shrink-0 text-[11px] text-muted-foreground">· {statusLabel}</span>
          </div>
          {/** biome-ignore lint/a11y/useKeyWithClickEvents: action cluster wraps individual buttons; stopPropagation only */}
          {/** biome-ignore lint/a11y/noStaticElementInteractions: same */}
          <div className="flex shrink-0 items-center gap-1" onClick={stopClick}>
            <span className="text-[11px] text-muted-foreground tabular-nums">{t('agents.card.tokens', { count: formatTokens(displayTokens) })}</span>
            {prAvailable && (
              <Button variant="ghost" size="icon" onClick={() => void openOrCreatePr()} disabled={creatingPr} className="size-7" title={prUrl ? t('agents.detail.openPr') : t('agents.detail.createPr')}>
                {prUrl ? <ExternalLink className="size-3.5" /> : <GitPullRequest className="size-3.5" />}
              </Button>
            )}
            <Button variant="ghost" size="icon" onClick={() => void copyLog()} className="size-7" title={t('agents.detail.copyLog')}>
              <Copy className="size-3.5" />
            </Button>
            {isWorking ? (
              <Button variant="ghost" size="icon" onClick={() => void cancel()} className="size-7" title={t('agents.card.stop')}>
                <Square className="size-3.5" />
              </Button>
            ) : (
              <Button variant="ghost" size="icon" onClick={() => void remove()} className="size-7 text-muted-foreground hover:text-destructive" title={t('agents.card.delete')}>
                <Trash2 className="size-3.5" />
              </Button>
            )}
          </div>
        </CardHeader>

        <CardContent className="px-3">
          {agent.summary && (
            <p className="line-clamp-2 break-words text-xs" title={agent.summary}>
              {agent.summary}
            </p>
          )}
          <p className="mt-1 text-[10px] text-muted-foreground">{t('agents.card.started', { when: formatRelative(agent.startedAt) })}</p>
        </CardContent>
      </Card>
    </AgentDetailModal>
  );
}
