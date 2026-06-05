import { Archive, Copy, ExternalLink, GitBranch, GitPullRequest, Pencil, Square, Trash2 } from 'lucide-react';
import type * as React from 'react';
import { useRef, useState } from 'react';
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
import { useCardAnimationStyle } from '@/providers/appearance';
import { findAgentKind, OPENCODE_DESCRIPTOR } from './agent-kinds';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

const KNOWN_STATUSES = new Set(['working', 'waiting', 'error', 'completed', 'idle', 'archived']);

interface Props {
  agent: Agent;
  selected: boolean;
  onSelect: () => void;
  // biome-ignore lint/suspicious/noExplicitAny: icon props vary
  providerIcon?: React.ComponentType<any>;
}

export function AgentListItem({ agent, selected, onSelect, providerIcon }: Props) {
  const { t } = useTranslation();
  const kindCfg = findAgentKind(agent.kind) ?? (agent.kind === 'opencode' ? OPENCODE_DESCRIPTOR : undefined);
  const KindIcon = providerIcon ?? kindCfg?.icon;
  const isWorking = agent.status === 'working';
  const { style: cardAnimation } = useCardAnimationStyle();
  const liveTokens = useLiveTokens(agent.id, tokenTotal(agent.tokens));
  const displayTokens = liveTokens?.tokens ?? tokenTotal(agent.tokens);
  const statusKey = KNOWN_STATUSES.has(agent.status) ? agent.status : 'idle';
  const statusLabel = t(`agents.status.${statusKey}`);

  const prUrl = agent.worktree?.prUrl ?? '';
  const hasBranch = Boolean(agent.worktree?.branch && agent.worktree?.path);
  const prDisabledReason = prUrl ? null : !hasBranch ? t('agents.detail.createPrDisabledNoBranch') : isWorking ? t('agents.detail.createPrDisabledStillWorking') : null;
  const prAvailable = !!prUrl || !prDisabledReason;
  const [creatingPr, setCreatingPr] = useState(false);

  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

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
      const events = await ReadAgentLog(agent.id);
      const text = (events ?? [])
        .filter((e) => e.content)
        .map((e) => {
          if (e.type === 'tool_call') {
            return `→ ${e.name ?? 'Tool'}${e.content ? ' · ' + e.content : ''}`;
          }
          if (e.type === 'tool_result') {
            return `← ${e.content}`;
          }
          return e.content ?? '';
        })
        .join('\n');
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

  const startEditing = (e: React.MouseEvent) => {
    e.stopPropagation();
    setEditValue(agent.summary ?? '');
    setIsEditing(true);
    setTimeout(() => {
      inputRef.current?.select();
    }, 0);
  };

  const commitEdit = async () => {
    setIsEditing(false);
    const trimmed = editValue.trim();
    if (trimmed === (agent.summary ?? '')) {
      return;
    }

    try {
      agentsCollection.update(agent.id, (draft) => {
        draft.summary = trimmed || undefined;
      });
    } catch (err) {
      toastError({ title: t('agents.card.couldNotRename'), err });
    }
  };

  const cancelEdit = () => {
    setIsEditing(false);
  };

  return (
    <div className="group relative">
      {isWorking && cardAnimation === 'pulse' && <span className="pointer-events-none absolute inset-0 animate-pulse rounded-md ring-1 ring-inset ring-primary/50" aria-hidden />}
      {isWorking && cardAnimation === 'glow' && <span className="pointer-events-none absolute inset-0 animate-pulse rounded-md ring-2 ring-inset ring-primary/30 shadow-[0_0_12px_2px] shadow-primary/20" aria-hidden />}
      <div
        role="button"
        tabIndex={0}
        onClick={isEditing ? undefined : onSelect}
        onKeyDown={(e) => {
          if (!isEditing && (e.key === 'Enter' || e.key === ' ')) {
            onSelect();
          }
        }}
        className={cn(
          'relative overflow-hidden flex w-full cursor-pointer flex-col gap-1 rounded-md border px-3 py-2.5 text-left transition-colors',
          selected ? 'border-border/60 bg-accent' : 'border-border/30 hover:border-border/50 hover:bg-accent/50',
          isEditing && 'cursor-default',
          statusKey === 'archived' && 'opacity-50',
        )}
      >
        {isWorking && cardAnimation === 'shimmer' && <span className="pointer-events-none absolute inset-y-0 w-1/3 bg-gradient-to-r from-transparent via-foreground/8 to-transparent" style={{ animation: 'card-shimmer 2.4s ease-in-out infinite' }} aria-hidden />}
        {isWorking && cardAnimation === 'progress' && <span className="pointer-events-none absolute top-0 left-0 h-0.5 w-1/3 rounded-full bg-primary/70" style={{ animation: 'card-progress 1.8s ease-in-out infinite' }} aria-hidden />}
        {KindIcon && <KindIcon className="pointer-events-none absolute inset-y-0 right-1 my-auto h-[90%] w-auto opacity-[0.06] text-foreground" aria-hidden />}
        <div className="flex items-center gap-2">
          {statusKey === 'archived' ? <Archive className="size-3 shrink-0 text-muted-foreground/60" aria-hidden /> : <span className={cn('size-2 shrink-0 rounded-full', SEVERITY_DOT[agentStatusSeverity(statusKey)], statusKey === 'working' && 'animate-pulse')} aria-hidden />}
          <span className="shrink-0 text-[11px] text-muted-foreground">{statusLabel}</span>
          {!isEditing && (
            <div className="ml-auto flex items-center gap-0.5 opacity-0 transition-opacity pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()}>
              <Button variant="ghost" size="icon" onClick={startEditing} className="size-6" title={t('agents.card.rename')}>
                <Pencil className="size-3" />
              </Button>
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
                <Button variant="ghost" size="icon" onClick={() => void remove()} className="size-6 text-destructive/60 hover:text-destructive" title={t('agents.card.delete')}>
                  <Trash2 className="size-3" />
                </Button>
              )}
            </div>
          )}
        </div>
        {isEditing ? (
          <input
            ref={inputRef}
            value={editValue}
            onChange={(e) => setEditValue(e.target.value)}
            onBlur={() => void commitEdit()}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void commitEdit();
              }
              if (e.key === 'Escape') {
                e.preventDefault();
                cancelEdit();
              }
              e.stopPropagation();
            }}
            onClick={(e) => e.stopPropagation()}
            placeholder={t('agents.card.renamePlaceholder')}
            className="w-full truncate border-b border-border bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground/50"
            autoFocus
          />
        ) : (
          agent.summary && (
            <p className={cn('truncate text-xs text-muted-foreground', statusKey === 'archived' && 'line-through decoration-muted-foreground/40')} title={agent.summary}>
              {agent.summary}
            </p>
          )
        )}
        {agent.worktree?.branch && (
          <div className="flex items-center gap-0.5 truncate text-[10px] text-muted-foreground">
            <GitBranch className="size-2.5 shrink-0" />
            <span className="truncate font-mono">{agent.worktree.branch}</span>
          </div>
        )}
        <div className="flex items-center justify-between text-[10px] text-muted-foreground">
          <span>{t('agents.card.started', { when: formatRelative(agent.startedAt) })}</span>
          <span className="tabular-nums">{t('agents.card.tokens', { count: formatTokens(displayTokens) })}</span>
        </div>
      </div>
    </div>
  );
}
