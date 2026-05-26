import { useLiveQuery } from '@tanstack/react-db';
import { Paperclip, Send, Square, TriangleAlert, X } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { selectAgent } from '@/state/agent-selection';
import { useCurrentProject } from '@/state/projects';
import type { Agent } from '@/types';
import { CancelAgent, ClearAgentLog, GetProjectFileStatuses, PickFiles, ReadAgentLog, ReadFileBase64, RespondToAgentQuestion, SendToAgent, SpawnAgent } from '@/wailsjs/go/main/App';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';
import { AgentDetailFilesTab } from './agent-detail-files-tab';
import { AgentDetailLogsTab } from './agent-detail-logs-tab';
import { countToolsFromLog } from './agent-log-files';
import { AskUserQuestionPanel, type AskUserQuestionPayload } from './ask-user-question-panel';
import { stripFileMentions } from './file-mentions';
import { MentionTextarea, type SlashCommand } from './mention-textarea';
import { TokenStat } from './token-stat';
import { tokenTotal, useLiveTokens } from './use-live-tokens';

// Tool-use ids the user has already answered/dismissed, keyed `${agentId}:${toolUseId}`.
// Module-level so it survives the panel remounting on agent/tab switches: a question
// the user acted on must never re-display, even if the backend's persisted-question
// clear is slow to reach the frontend collection. A genuinely new question (new
// toolUseId) is unaffected and still shows.
const answeredQuestions = new Set<string>();
const questionKey = (agentId: string, toolUseId: string) => `${agentId}:${toolUseId}`;

export function AgentConversation({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const { project, projectId } = useCurrentProject();
  const { kinds: cliKinds, opencode } = useAgentClis();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));

  const { data = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const agent = data.find(({ id }) => id === agentId) ?? null;
  const status = agent?.status;
  const isDraft = status === 'draft';

  const provider = providers.find((p) => p.id === agent?.providerId) ?? undefined;
  const cliCfg = agent && !agent.providerId ? (agent.kind === 'opencode' ? opencode : cliKinds.find((k) => k.id === agent.kind)) : undefined;
  const models: { value: string; label: string }[] = cliCfg ? cliCfg.models : provider ? provider.models.map((m) => ({ value: m, label: m })) : [];
  const defaultsId = provider?.id ?? cliCfg?.id;
  const agentDefaults = useAgentDefaults();
  const spawnModel = (defaultsId ? agentDefaults.get(defaultsId) : undefined) ?? models[0]?.value ?? '';

  const [log, setLog] = useState('');
  const [activeTab, setActiveTab] = useState('logs');
  const [message, setMessage] = useState('');
  const [attachments, setAttachments] = useState<{ path: string; preview: string }[]>([]);
  const [sending, setSending] = useState(false);
  const [pendingQuestion, setPendingQuestion] = useState<{ toolUseId: string; payload: AskUserQuestionPayload } | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);

  // Focus the input whenever a conversation opens (the panel remounts per
  // agentId). Re-assert on the next frame: a closing dropdown / Radix focus
  // scope can steal it back to its trigger right after mount.
  useEffect(() => {
    inputRef.current?.focus();
    const raf = requestAnimationFrame(() => inputRef.current?.focus());
    return () => cancelAnimationFrame(raf);
  }, []);

  useEffect(() => {
    const eventName = 'agent:ask-user-question';
    const handler = (payload: { agentId: string; toolUseId: string; input: AskUserQuestionPayload }) => {
      if (payload.agentId !== agentId) {
        return;
      }
      if (answeredQuestions.has(questionKey(agentId, payload.toolUseId))) {
        return;
      }
      setPendingQuestion({ toolUseId: payload.toolUseId, payload: payload.input });
    };
    EventsOn(eventName, handler);
    return () => EventsOff(eventName);
  }, [agentId]);

  // Hydrate from the agent record so the panel reappears after an app restart
  // — without it the question would be visible only to the listener active at
  // emit time.
  const persistedQuestionId = agent?.pendingQuestion?.toolUseId ?? '';
  const persistedQuestionInput = agent?.pendingQuestion?.input;
  useEffect(() => {
    if (!persistedQuestionId || !persistedQuestionInput) {
      return;
    }
    if (answeredQuestions.has(questionKey(agentId, persistedQuestionId))) {
      return;
    }
    setPendingQuestion((prev) => {
      if (prev?.toolUseId === persistedQuestionId) {
        return prev;
      }
      return { toolUseId: persistedQuestionId, payload: persistedQuestionInput as AskUserQuestionPayload };
    });
  }, [agentId, persistedQuestionId, persistedQuestionInput]);

  useEffect(() => {
    if (!persistedQuestionId) {
      setPendingQuestion(null);
    }
  }, [persistedQuestionId]);

  // Use the backend count: it intersects the log's edited files with the actual
  // git changes, so scratch files the agent wrote outside the repo (e.g. a plan
  // .md) don't inflate it and it matches the Files tab. The raw log count would
  // count every Write/Edit target, real change or not.
  const filesModified = agent?.filesModified ?? 0;
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
  const isWorking = status === 'working';
  const canSend = isDraft || ((agent?.kind === 'claude-code' || agent?.kind === 'opencode' || agent?.kind === 'mistral') && !!agent?.sessionId) || agent?.status === 'waiting';
  const sendDisabledReason = isDraft || !agent ? null : !canSend ? t('agents.detail.sendNotSupported') : null;
  const isolated = project?.isolatedDefault ?? false;
  const [dirtyWorktree, setDirtyWorktree] = useState(false);
  const [warnDismissed, setWarnDismissed] = useState(false);
  useEffect(() => {
    if (!isDraft || isolated || !projectId) {
      setDirtyWorktree(false);
      return;
    }
    GetProjectFileStatuses(projectId)
      .then((files) => setDirtyWorktree(files.length > 0))
      .catch(() => setDirtyWorktree(false));
  }, [isDraft, isolated, projectId]);

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

  const onLogScroll = () => {
    const el = logRef.current;
    if (!el) {
      return;
    }

    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };

  const pickFiles = async () => {
    const files = await PickFiles(project?.path ?? '').catch((err: unknown) => {
      toastError({ title: t('agents.detail.pickerUnavailable'), err });
      return null;
    });
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
    try {
      await CancelAgent(agentId);
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotStop'), err });
    }
  };

  const send = async () => {
    const parts = [stripFileMentions(message).trim(), ...attachments.map((a) => a.path)];
    const text = parts.filter(Boolean).join('\n');
    if (!text || sending) {
      return;
    }
    if (isDraft && agent) {
      const pid = agent.projectId;
      setSending(true);
      try {
        const input = provider
          ? { projectId: pid, kind: 'opencode', providerId: provider.id, task: text, model: spawnModel, isolated }
          : cliCfg?.id === 'opencode'
            ? { projectId: pid, kind: 'opencode', task: text, model: spawnModel, isolated }
            : cliCfg
              ? { projectId: pid, kind: cliCfg.id, task: text, model: spawnModel, binary: cliCfg.binary, isolated }
              : null;
        const spawned = input ? await SpawnAgent(input) : null;
        if (spawned) {
          setMessage('');
          setAttachments([]);
          await agentsCollection.insert(spawned as unknown as Agent);
          try {
            await agentsCollection.delete(agent.id);
          } catch {
            // draft cleanup is best effort; the spawned agent is what matters
          }
          selectAgent(spawned.id);
        }
      } catch (err) {
        toastError({ title: t('agents.new.couldNotStart'), err });
      } finally {
        setSending(false);
      }
      return;
    }
    if (sendDisabledReason) {
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

  const slashCommands = useMemo<SlashCommand[]>(() => {
    const list: SlashCommand[] = [];
    if (models.length > 0) {
      list.push({ name: 'model', description: t('agents.detail.commandModel'), takesArg: true });
    }
    if (isWorking) {
      list.push({ name: 'stop', description: t('agents.detail.commandStop') });
    }
    list.push({ name: 'logs', description: t('agents.detail.commandLogs') });
    list.push({ name: 'files', description: t('agents.detail.commandFiles') });
    list.push({ name: 'clear', description: t('agents.detail.commandClear') });
    return list;
  }, [models.length, isWorking, t]);

  const runCommand = (name: string, args: string) => {
    switch (name) {
      case 'model': {
        const target = models.find((m) => m.value === args || m.label.toLowerCase() === args.toLowerCase());
        if (target && defaultsId) {
          agentDefaults.set(defaultsId, target.value);
        }
        break;
      }
      case 'stop':
        void cancel();
        break;
      case 'logs':
        setActiveTab('logs');
        break;
      case 'files':
        setActiveTab('files');
        break;
      case 'clear':
        setLog('');
        void ClearAgentLog(agentId).catch((err) => toastError({ title: t('agents.detail.couldNotClear'), err }));
        break;
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col gap-3">
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

        <TabsContent forceMount value="files" className={cn('m-0 flex min-h-0 flex-1 flex-col', activeTab !== 'files' && 'hidden')}>
          <AgentDetailFilesTab agent={agent} />
        </TabsContent>
      </Tabs>

      {agent && (
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
      )}

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
        {pendingQuestion ? (
          <div className="flex items-start gap-2">
            <AskUserQuestionPanel
              payload={pendingQuestion.payload}
              projectPath={project?.path}
              onCancel={() => {
                answeredQuestions.add(questionKey(agentId, pendingQuestion.toolUseId));
                void RespondToAgentQuestion(agentId, pendingQuestion.toolUseId, 'User dismissed the question without answering.').catch(() => {});
                setPendingQuestion(null);
              }}
              onSubmit={(answers) => {
                answeredQuestions.add(questionKey(agentId, pendingQuestion.toolUseId));
                const content = JSON.stringify(answers);
                void RespondToAgentQuestion(agentId, pendingQuestion.toolUseId, content).catch((err) => toastError({ title: t('agents.askUserQuestion.failed', { defaultValue: 'Could not send answer' }), err }));
                setPendingQuestion(null);
              }}
            />
          </div>
        ) : (
          <>
            {isDraft && models.length === 0 && <p className="mb-2 text-xs text-destructive">{t('agents.new.noModels')}</p>}
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
                inputRef={inputRef}
                placeholder={isDraft ? t('agents.new.taskPlaceholder') : (sendDisabledReason ?? t('agents.detail.sendPlaceholder'))}
                value={message}
                onChange={setMessage}
                onAttach={(path) => void addAttachment(path)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    void send();
                  }
                }}
                disabled={(!isDraft && !!sendDisabledReason) || sending}
                projectPath={project?.path}
                commands={slashCommands}
                onCommand={runCommand}
                className="max-h-48 min-h-10 resize-none field-sizing-content"
              />
              {message.trim() || attachments.length > 0 || !isWorking ? (
                <Button size="icon" title={t('agents.detail.send')} disabled={(!isDraft && !!sendDisabledReason) || sending || (!message.trim() && attachments.length === 0) || (isDraft && !spawnModel)} onClick={() => void send()} className="size-10">
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
    </div>
  );
}
