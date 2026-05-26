import { useLiveQuery } from '@tanstack/react-db';
import { Bot, Paperclip, Send, Square, TriangleAlert, X } from 'lucide-react';
import { type PropsWithChildren, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { agentsCollection } from '@/db';
import { formatRelative } from '@/lib/time';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { cn } from '@/lib/utils';
import { useAgentClis } from '@/state/agent-clis';
import { useCurrentProject } from '@/state/projects';
import type { AgentKind } from '@/types';
import { CancelAgent, GetProjectFileStatuses, PickFiles, ReadAgentLog, ReadFileBase64, RespondToAgentQuestion, SendToAgent, SpawnAgent } from '@/wailsjs/go/main/App';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';
import { AgentDetailFilesTab } from './agent-detail-files-tab';
import { AgentDetailLogsTab } from './agent-detail-logs-tab';
import { findAgentKind } from './agent-kinds';
import { countFilesFromLog, countToolsFromLog } from './agent-log-files';
import { AskUserQuestionPanel, type AskUserQuestionPayload } from './ask-user-question-panel';
import { MentionTextarea } from './mention-textarea';
import { TokenStat } from './token-stat';
import { useLiveTokens } from './use-live-tokens';

interface Props extends DialogModeProps {
  agentId?: string;
  pending?: { kindId: AgentKind };
}

const STATUS_DOT: Record<string, string> = {
  working: 'bg-blue-500 animate-pulse',
  waiting: 'bg-amber-500',
  error: 'bg-red-500',
  completed: 'bg-emerald-500',
  idle: 'bg-muted-foreground',
};

export function AgentDetailModal({ agentId: agentIdProp, pending: pendingProp, children, ...modeProps }: PropsWithChildren<Props>) {
  const { open, setOpen } = useDialogMode(modeProps);
  const [override, setOverride] = useState<{ agentId: string } | null>(null);
  const statusRef = useRef<string | undefined>(undefined);
  const pendingQuestionRef = useRef<string | null>(null);

  useEffect(() => {
    if (!open) {
      setOverride(null);
    }
  }, [open]);

  const agentId = override?.agentId ?? agentIdProp ?? null;
  const pendingKindId = override ? null : (pendingProp?.kindId ?? null);

  const onEscapeKeyDown = (e: KeyboardEvent) => {
    // Pending question takes precedence: Esc dismisses the question rather
    // than closing the modal, matching how claude-code's own UI handles it.
    if (agentId && pendingQuestionRef.current) {
      e.preventDefault();
      RespondToAgentQuestion(agentId, pendingQuestionRef.current, 'User dismissed the question without answering.').catch(() => {});
      pendingQuestionRef.current = null;
      return;
    }
    if (agentId && statusRef.current === 'working') {
      e.preventDefault();
      CancelAgent(agentId).catch(() => {});
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent onEscapeKeyDown={onEscapeKeyDown} className="flex h-[85vh] w-[min(96vw,1600px)] max-w-[1600px] flex-col gap-0 p-6 sm:max-w-[1600px]">
        <Body agentId={agentId} pendingKindId={pendingKindId} onSpawned={(id) => setOverride({ agentId: id })} statusRef={statusRef} pendingQuestionRef={pendingQuestionRef} dialogOpen={open} />
      </DialogContent>
    </Dialog>
  );
}

function Body({ agentId, pendingKindId, onSpawned, statusRef, pendingQuestionRef, dialogOpen }: { agentId: string | null; pendingKindId: AgentKind | null; onSpawned: (agentId: string) => void; statusRef: React.RefObject<string | undefined>; pendingQuestionRef: React.RefObject<string | null>; dialogOpen: boolean }) {
  const { t } = useTranslation();
  const isPending = !agentId && !!pendingKindId;
  const { project, projectId } = useCurrentProject();
  const { kinds: cliKinds } = useAgentClis();

  const { data = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const agent = data.find(({ id }) => id === agentId) ?? null;
  const status = agent?.status;
  statusRef.current = status;
  const pendingKindCfg = isPending ? cliKinds.find(({ id }) => id === pendingKindId) : undefined;
  const kindCfg = pendingKindCfg ?? (agent?.kind ? findAgentKind(agent.kind) : undefined);
  const kind = kindCfg ?? { label: agent?.kind ?? pendingKindId ?? 'Agent', icon: Bot, iconClass: 'bg-muted text-foreground' };
  const Icon = kind.icon;

  const [log, setLog] = useState('');
  const [activeTab, setActiveTab] = useState('logs');
  const [message, setMessage] = useState('');
  const [attachments, setAttachments] = useState<{ path: string; preview: string }[]>([]);
  const [sending, setSending] = useState(false);
  const [pendingQuestion, setPendingQuestion] = useState<{ toolUseId: string; payload: AskUserQuestionPayload } | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);

  useEffect(() => {
    if (!agentId || !dialogOpen) {
      return;
    }
    const eventName = 'agent:ask-user-question';
    const handler = (payload: { agentId: string; toolUseId: string; input: AskUserQuestionPayload }) => {
      if (payload.agentId !== agentId) {
        return;
      }
      setPendingQuestion({ toolUseId: payload.toolUseId, payload: payload.input });
    };
    EventsOn(eventName, handler);
    return () => EventsOff(eventName);
  }, [agentId, dialogOpen]);

  // Hydrate from the agent record so the panel reappears when the modal
  // reopens (or after an app restart) — without it the question would be
  // visible only to the listener active at emit time.
  const persistedQuestionId = agent?.pendingQuestionId ?? '';
  const persistedQuestionInput = agent?.pendingQuestionInput ?? '';
  useEffect(() => {
    if (!persistedQuestionId || !persistedQuestionInput) {
      return;
    }
    setPendingQuestion((prev) => {
      if (prev?.toolUseId === persistedQuestionId) {
        return prev;
      }
      try {
        const parsed = JSON.parse(persistedQuestionInput) as AskUserQuestionPayload;
        return { toolUseId: persistedQuestionId, payload: parsed };
      } catch {
        return prev;
      }
    });
  }, [persistedQuestionId, persistedQuestionInput]);

  useEffect(() => {
    if (!persistedQuestionId) {
      setPendingQuestion(null);
    }
  }, [persistedQuestionId]);

  useEffect(() => {
    if (!dialogOpen) {
      setPendingQuestion(null);
    }
  }, [dialogOpen]);

  // Mirror the toolUseId on a ref so the Dialog's onEscapeKeyDown (declared
  // in the parent component) can read it without prop drilling state.
  useEffect(() => {
    pendingQuestionRef.current = pendingQuestion?.toolUseId ?? null;
  }, [pendingQuestion, pendingQuestionRef]);

  const filesModified = useMemo(() => countFilesFromLog(log), [log]);
  const toolsUsed = useMemo(() => countToolsFromLog(log), [log]);
  const liveTokens = useLiveTokens(agentId, agent?.tokens ?? 0);
  const displayTokens = liveTokens?.tokens ?? agent?.tokens ?? 0;
  const displayCost = liveTokens?.costUsd ?? agent?.costUsd ?? 0;
  const displayParts = liveTokens?.parts ?? {
    input: agent?.tokensInput ?? 0,
    output: agent?.tokensOutput ?? 0,
    cacheCreation: agent?.tokensCacheCreate ?? 0,
    cacheRead: agent?.tokensCacheRead ?? 0,
  };
  const isWorking = status === 'working';
  const canSend = isPending || (agent?.kind === 'claude-code' && !!agent?.sessionId) || agent?.status === 'waiting';
  const sendDisabledReason = isPending || !agent ? null : !canSend ? t('agents.detail.sendNotSupported') : null;
  const isolated = project?.isolatedDefault ?? false;
  const [dirtyWorktree, setDirtyWorktree] = useState(false);
  const [warnDismissed, setWarnDismissed] = useState(false);
  useEffect(() => {
    if (!isPending || isolated || !projectId) {
      setDirtyWorktree(false);
      return;
    }
    GetProjectFileStatuses(projectId)
      .then((files) => setDirtyWorktree(files.length > 0))
      .catch(() => setDirtyWorktree(false));
  }, [isPending, isolated, projectId]);

  useEffect(() => {
    if (!agentId) {
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
  }, [agentId, status]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: log is the trigger; body reads ref only
  useEffect(() => {
    const el = logRef.current;
    if (el && stickToBottom.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [log]);

  const onLogScroll = () => {
    const el = logRef.current;
    if (!el) {
      return;
    }

    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };

  const pickFiles = async () => {
    const files = await PickFiles(project?.path ?? '').catch(() => null);
    if (!files?.length) {
      return;
    }
    const base = project?.path ? project.path.replace(/\/?$/, '/') : '';
    const refs = files.map((f) => (base && f.startsWith(base) ? f.slice(base.length) : f));
    setMessage((prev) => (prev ? `${prev}\n${refs.join('\n')}` : refs.join('\n')));
  };

  const addAttachment = async (path: string) => {
    try {
      const b64 = await ReadFileBase64(path);
      setAttachments((prev) => [...prev, { path, preview: `data:image/png;base64,${b64}` }]);
    } catch {
      setAttachments((prev) => [...prev, { path, preview: '' }]);
    }
  };

  const cancel = async () => {
    if (!agentId) {
      return;
    }
    try {
      await CancelAgent(agentId);
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotStop'), err });
    }
  };

  const send = async () => {
    const parts = [message.trim(), ...attachments.map((a) => a.path)];
    const text = parts.filter(Boolean).join('\n');
    if (!text || sending) {
      return;
    }
    if (isPending) {
      if (!projectId || !pendingKindCfg) {
        return;
      }
      setSending(true);
      try {
        const spawned = await SpawnAgent({
          projectId,
          kind: pendingKindCfg.id,
          task: text,
          model: pendingKindCfg.models[0]?.value ?? '',
          binary: pendingKindCfg.binary,
          isolated,
        });
        if (spawned) {
          setMessage('');
          setAttachments([]);
          onSpawned(spawned.id);
        }
      } catch (err) {
        toastError({ title: t('agents.new.couldNotStart'), err });
      } finally {
        setSending(false);
      }
      return;
    }
    if (!agentId || sendDisabledReason) {
      return;
    }
    setSending(true);
    try {
      await SendToAgent(agentId, text);
      setMessage('');
      setAttachments([]);
      ReadAgentLog(agentId)
        .then((s: string) => setLog(s))
        .catch(() => {});
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotSend'), err });
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <span className={cn('size-2.5 shrink-0 rounded-full', STATUS_DOT[status ?? 'idle'] ?? STATUS_DOT.idle)} />
          <span className={cn('flex size-7 shrink-0 items-center justify-center rounded-md', kind.iconClass)}>
            <Icon className="size-3.5" />
          </span>
          <div className="min-w-0">
            <DialogTitle className="truncate text-base">
              {kind.label}
              {agent && <span className="ml-2 text-xs font-normal text-muted-foreground">· {t('agents.detail.started', { when: formatRelative(agent.startedAt) })}</span>}
              {agent?.branch && (
                <Badge variant="outline" className="ml-2 gap-1 font-mono text-[10px]" title={agent.worktreePath ?? ''}>
                  {t('agents.detail.isolatedBadge')}
                  <span>·</span>
                  <span>{agent.branch}</span>
                </Badge>
              )}
            </DialogTitle>
            {agent?.summary && (
              <p className="mt-0.5 truncate text-xs text-muted-foreground" title={agent.summary}>
                {agent.summary}
              </p>
            )}
          </div>
        </div>
      </DialogHeader>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="mt-4 flex min-h-0 flex-1 flex-col gap-3">
        <TabsList variant="line" className="shrink-0 border-b border-border">
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

        <TabsContent forceMount value="logs" className={cn('m-0 flex min-h-0 flex-col gap-3', activeTab !== 'logs' && 'hidden')}>
          <AgentDetailLogsTab log={log} isWorking={isWorking} logRef={logRef} onLogScroll={onLogScroll} />
        </TabsContent>

        <TabsContent forceMount value="files" className={cn('m-0 flex min-h-0 flex-col', activeTab !== 'files' && 'hidden')}>
          <AgentDetailFilesTab agent={agent} />
        </TabsContent>
      </Tabs>

      <div className="mt-3 flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
        <TokenStat tokens={displayTokens} parts={displayParts} className="tabular-nums" />
        <span aria-hidden>·</span>
        <span className="tabular-nums">{t('agents.detail.statTools', { count: toolsUsed })}</span>
        {displayCost ? (
          <>
            <span aria-hidden>·</span>
            <span className="tabular-nums">${displayCost.toFixed(4)}</span>
          </>
        ) : null}
      </div>

      {dirtyWorktree && !warnDismissed && (
        <div className="mt-2 flex shrink-0 items-center gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
          <TriangleAlert className="size-3.5 shrink-0" />
          <span className="flex-1">{t('agents.new.sharedBranchWarn')}</span>
          <button type="button" onClick={() => setWarnDismissed(true)} className="shrink-0 rounded-sm opacity-70 hover:opacity-100">
            <X className="size-3.5" />
          </button>
        </div>
      )}
      <div className="mt-2 shrink-0">
        {pendingQuestion && agentId ? (
          <div className="flex items-start gap-2">
            <AskUserQuestionPanel
              payload={pendingQuestion.payload}
              onCancel={() => {
                void RespondToAgentQuestion(agentId, pendingQuestion.toolUseId, 'User dismissed the question without answering.').catch(() => {});
                setPendingQuestion(null);
              }}
              onSubmit={(answers) => {
                const content = JSON.stringify(answers);
                void RespondToAgentQuestion(agentId, pendingQuestion.toolUseId, content).catch((err) => toastError({ title: t('agents.askUserQuestion.failed', { defaultValue: 'Could not send answer' }), err }));
                setPendingQuestion(null);
              }}
            />
          </div>
        ) : (
          <>
            {attachments.length > 0 && (
              <div className="mb-2 flex flex-wrap gap-2">
                {attachments.map((att, i) => (
                  <div key={att.path} className="group relative">
                    {att.preview ? <img src={att.preview} alt="" className="h-20 rounded-md border object-cover" /> : <div className="flex h-20 w-24 items-center justify-center rounded-md border bg-muted text-xs text-muted-foreground">{att.path.split('/').pop()}</div>}
                    <button type="button" onClick={() => setAttachments((prev) => prev.filter((_, j) => j !== i))} className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100">
                      <X className="size-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-start gap-2">
              <Button variant="outline" size="icon" title={t('agents.detail.attachFiles')} onClick={() => void pickFiles()} className="size-10 shrink-0">
                <Paperclip className="size-4" />
              </Button>
              <MentionTextarea
                autoFocus
                placeholder={isPending ? t('agents.new.taskPlaceholder') : (sendDisabledReason ?? t('agents.detail.sendPlaceholder'))}
                value={message}
                onChange={setMessage}
                onAttach={(path) => void addAttachment(path)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    void send();
                  }
                }}
                disabled={(!isPending && !!sendDisabledReason) || sending}
                projectPath={project?.path}
                className="max-h-48 min-h-10 resize-none field-sizing-content"
              />
              {message.trim() || attachments.length > 0 || !isWorking ? (
                <Button size="icon" title={t('agents.detail.send')} disabled={(!isPending && !!sendDisabledReason) || sending || (!message.trim() && attachments.length === 0) || (isPending && !projectId)} onClick={() => void send()} className="size-10">
                  <Send className="size-4" />
                </Button>
              ) : (
                <Button variant="destructive" size="icon" title={t('agents.detail.stopAgent')} onClick={() => void cancel()} className="size-10">
                  <Square className="size-3.5" />
                </Button>
              )}
            </div>
          </>
        )}
      </div>
    </>
  );
}
