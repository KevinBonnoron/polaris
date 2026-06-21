import { useLiveQuery } from '@tanstack/react-db';
import { Clock, Mic, Paperclip, Send, Square, TriangleAlert, X, Zap } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { agentsCollection } from '@/collections/agents.collection';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { toastError } from '@/lib/toast-error';
import { useAgentClis } from '@/state/agent-clis';
import { useAgentDefaults } from '@/state/agent-defaults';
import { selectAgent } from '@/state/agent-selection';
import { useCurrentProject } from '@/state/projects';
import type { Agent } from '@/types';
import { CancelAgent, ClearAgentLog, ClearAgentQueuedMessage, GetProjectFileStatuses, InterruptAndSendToAgent, ListClaudeCodeSessions, PickFiles, RespondToAgentQuestion, SendToAgent, SetAgentModel, SpawnAgent, StopAndRetractLastMessage, TeleportClaudeSession } from '@/wailsjs/go/main/App';
import { EventsOn } from '@/wailsjs/runtime/runtime';
import { AskUserQuestionPanel, type AskUserQuestionPayload } from './ask-user-question-panel';
import { type Attachment, AttachmentPreviews, loadAttachment } from './attachments';
import { stripFileMentions } from './file-mentions';
import { MentionTextarea, type SlashCommand } from './mention-textarea';

const answeredQuestions = new Set<string>();
const questionKey = (agentId: string, toolUseId: string) => `${agentId}:${toolUseId}`;

export const NO_TOOLS_SENTINEL = '__no_tools__';
export const TOOL_PRESETS: Record<string, string[]> = {
  all: [],
  readonly: ['Read', 'Glob', 'Grep', 'LS', 'TodoRead', 'WebFetch', 'WebSearch', 'Task'],
  'no-web': ['Read', 'Write', 'Edit', 'MultiEdit', 'Bash', 'Glob', 'Grep', 'LS', 'Task', 'TodoRead', 'TodoWrite', 'NotebookEdit', 'NotebookRead'],
  'no-tools': [NO_TOOLS_SENTINEL],
};
const speechRecognitionAvailable = 'SpeechRecognition' in window || 'webkitSpeechRecognition' in window;
type DraftState = { message: string; attachments: Attachment[] };
const drafts = new Map<string, DraftState>();

interface Props {
  agentId: string;
  agent: Agent | null;
  inputRef: React.RefObject<HTMLTextAreaElement | null>;
  onLogRefresh: () => void;
  onSetActiveTab: (tab: string) => void;
  onClearLog: () => void;
  allowedTools: string[];
  onAllowedToolsChange: (tools: string[]) => void;
}

export function AgentInputArea({ agentId, agent, inputRef, onLogRefresh, onSetActiveTab, onClearLog, allowedTools, onAllowedToolsChange }: Props) {
  const { t } = useTranslation();
  const { project, projectId } = useCurrentProject();
  const { kinds: cliKinds, opencode } = useAgentClis();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const agentDefaults = useAgentDefaults();

  const status = agent?.status;
  const isDraft = status === 'draft';
  const isWorking = status === 'working';
  const queuedMessage = agent?.queuedMessage ?? '';

  const provider = providers.find((p) => p.id === agent?.providerId) ?? undefined;
  const cliCfg = agent && !agent.providerId ? (agent.kind === 'opencode' ? opencode : cliKinds.find((k) => k.id === agent.kind)) : undefined;
  const models: { value: string; label: string }[] = cliCfg ? cliCfg.models : provider ? provider.models.map((m) => ({ value: m, label: m })) : [];
  const defaultsId = provider?.id ?? cliCfg?.id;
  const spawnModel = (defaultsId ? agentDefaults.get(defaultsId) : undefined) ?? models[0]?.value ?? '';
  const isolated = project?.isolatedDefault ?? false;

  const [message, setMessage] = useState(() => drafts.get(agentId)?.message ?? '');
  const [attachments, setAttachments] = useState<Attachment[]>(() => drafts.get(agentId)?.attachments ?? []);
  const [sending, setSending] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const recognitionRef = useRef<{ stop(): void } | null>(null);
  // Last message sent to this agent, recalled into the input with ArrowUp. Reset
  // on agent switch so ArrowUp never recalls a different agent's message.
  const lastSentRef = useRef('');
  useEffect(() => {
    lastSentRef.current = '';
  }, [agentId]);
  const [pendingQuestion, setPendingQuestion] = useState<{ toolUseId: string; payload: AskUserQuestionPayload } | null>(null);
  const [teleportSessions, setTeleportSessions] = useState<{ value: string; label: string }[]>([]);
  const [dirtyWorktree, setDirtyWorktree] = useState(false);
  const [dirtyChecked, setDirtyChecked] = useState(false);
  const [warnDismissed, setWarnDismissed] = useState(false);

  useEffect(() => {
    const el = inputRef.current;
    if (!el) {
      return;
    }
    el.focus();
    const raf = requestAnimationFrame(() => {
      if (!inputRef.current) {
        return;
      }
      inputRef.current.focus();
      const len = inputRef.current.value.length;
      inputRef.current.setSelectionRange(len, len);
    });
    return () => cancelAnimationFrame(raf);
  }, [inputRef]);

  useEffect(() => {
    if (message || attachments.length > 0) {
      drafts.set(agentId, { message, attachments });
    } else {
      drafts.delete(agentId);
    }
  }, [agentId, message, attachments]);

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
    // EventsOn returns a per-listener unsubscribe; EventsOff(eventName) would
    // drop every mounted card's handler for this event, not just this one.
    return EventsOn(eventName, handler);
  }, [agentId]);

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

  useEffect(() => {
    if (!isDraft || !agent?.projectId) {
      setTeleportSessions([]);
      return;
    }
    ListClaudeCodeSessions(agent.projectId)
      .then((sessions) =>
        setTeleportSessions(
          sessions.map((s) => ({
            value: s.id,
            label: s.summary || s.id.slice(0, 8),
            dimmed: s.imported,
          })),
        ),
      )
      .catch(() => {});
  }, [isDraft, agent?.projectId]);

  useEffect(() => {
    if (!isDraft || isolated || !projectId) {
      setDirtyWorktree(false);
      setDirtyChecked(true);
      return;
    }
    let active = true;
    setDirtyChecked(false);
    const fallback = window.setTimeout(() => {
      if (active) {
        setDirtyWorktree(false);
        setDirtyChecked(true);
      }
    }, 5000);
    GetProjectFileStatuses(projectId)
      .then((files) => active && setDirtyWorktree(files.length > 0))
      .catch(() => active && setDirtyWorktree(false))
      .finally(() => {
        if (active) {
          window.clearTimeout(fallback);
          setDirtyChecked(true);
        }
      });
    return () => {
      active = false;
      window.clearTimeout(fallback);
    };
  }, [isDraft, isolated, projectId]);

  useEffect(() => {
    return () => {
      recognitionRef.current?.stop();
    };
  }, []);

  const addFilePaths = useCallback(
    (files: string[]) => {
      if (!files.length) {
        return;
      }
      const norm = (p: string) => p.replace(/\\/g, '/');
      const base = project?.path ? norm(project.path).replace(/\/?$/, '/') : '';
      const refs = files.map((f) => {
        const fNorm = norm(f);
        return base && fNorm.startsWith(base) ? fNorm.slice(base.length) : f;
      });
      setMessage((prev) => (prev ? `${prev}\n${refs.join('\n')}` : refs.join('\n')));
    },
    [project?.path],
  );

  const pickFiles = async () => {
    const files = await PickFiles(project?.path ?? '').catch((err: unknown) => {
      toastError({ title: t('agents.detail.pickerUnavailable'), err });
      return null;
    });
    if (!files?.length) {
      return;
    }
    addFilePaths(files);
  };

  const addAttachment = async (path: string) => {
    const att = await loadAttachment(path);
    setAttachments((prev) => [...prev, att]);
  };

  // Native file drop. main.go relays only drops landing on the conversation's
  // [data-file-drop-target] element; route to this card via the agent id it carries.
  useEffect(() => {
    return EventsOn('files:dropped', (payload: { files?: string[]; details?: { attributes?: Record<string, string> } }) => {
      if (payload?.details?.attributes?.['data-agent-id'] !== agentId) {
        return;
      }
      addFilePaths(payload.files ?? []);
    });
  }, [agentId, addFilePaths]);

  const cancel = async () => {
    try {
      await CancelAgent(agentId);
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotStop'), err });
    }
  };

  // Drop text into the input and place the caret at the end, ready to edit.
  const recallIntoInput = useCallback(
    (text: string) => {
      setMessage(text);
      requestAnimationFrame(() => {
        const el = inputRef.current;
        if (el) {
          el.focus();
          el.setSelectionRange(text.length, text.length);
        }
      });
    },
    [inputRef],
  );

  // Clear the queued message on the backend, surfacing failures: callers that
  // recall it for editing must not proceed if it's still live, or it would be
  // sent again at the next turn.
  const clearQueued = useCallback(async (): Promise<boolean> => {
    try {
      await ClearAgentQueuedMessage(agentId);
      return true;
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotClearQueued'), err });
      return false;
    }
  }, [agentId, t]);

  // Escape and ArrowUp are handled at the window level so they work the same way
  // regardless of which control has focus (ArrowUp still requires the input to be
  // focused and empty, so it never hijacks caret movement or list navigation).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const el = inputRef.current;
      if (e.key === 'Escape' && isWorking) {
        e.preventDefault();
        // A queued message would be dropped by the stop, so bring it back into the
        // input to edit; the in-flight (logged) message stays as history.
        if (queuedMessage) {
          const queued = queuedMessage;
          // Clear the queue explicitly (CancelAgent also clears it on success, but
          // surface a failure), then stop and bring the message back to edit.
          void clearQueued();
          CancelAgent(agentId).catch((err) => toastError({ title: t('agents.detail.couldNotStop'), err }));
          recallIntoInput(queued);
          return;
        }
        // No queue: stop, and when the turn hadn't responded yet the backend
        // retracts the last logged message and returns it so we can edit it.
        void (async () => {
          try {
            const retracted = await StopAndRetractLastMessage(agentId);
            if (retracted) {
              recallIntoInput(retracted);
              onLogRefresh();
            }
          } catch (err) {
            toastError({ title: t('agents.detail.couldNotStop'), err });
          }
        })();
        return;
      }
      if (e.key === 'ArrowUp') {
        const active = document.activeElement as HTMLElement | null;
        // Don't steal ArrowUp from another text field, or while editing our own.
        const inOtherField = !!active && active !== el && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || !!active.isContentEditable);
        if (inOtherField || (el && el.value.trim())) {
          return;
        }
        // A queued message is recalled like Escape — regardless of focus — since
        // that is the explicit "edit the pending message" gesture. The last-sent
        // fallback only fires when the input itself is focused, so it never
        // hijacks ArrowUp used to scroll elsewhere.
        if (queuedMessage) {
          e.preventDefault();
          const queued = queuedMessage;
          // Agent keeps working here, so only recall once the queue is actually
          // cleared — otherwise it would be sent again at the next turn.
          void (async () => {
            if (await clearQueued()) {
              recallIntoInput(queued);
            }
          })();
        } else if (active === el && lastSentRef.current) {
          e.preventDefault();
          recallIntoInput(lastSentRef.current);
        }
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isWorking, agentId, t, onLogRefresh, queuedMessage, recallIntoInput, clearQueued, inputRef]);

  const toggleRecording = () => {
    if (isRecording) {
      recognitionRef.current?.stop();
      return;
    }
    type SpeechRecognitionCtor = new () => {
      lang: string;
      interimResults: boolean;
      onresult: ((e: SpeechRecognitionEvent) => void) | null;
      onend: (() => void) | null;
      onerror: (() => void) | null;
      start(): void;
      stop(): void;
    };
    const win = window as unknown as { SpeechRecognition?: SpeechRecognitionCtor; webkitSpeechRecognition?: SpeechRecognitionCtor };
    const API = win.SpeechRecognition ?? win.webkitSpeechRecognition;
    if (!API) {
      toast.error(t('agents.detail.speechNotSupported'));
      return;
    }
    const recognition = new API();
    recognition.lang = navigator.language;
    recognition.interimResults = false;
    recognition.onresult = (e: SpeechRecognitionEvent) => {
      let transcript = '';
      for (let i = 0; i < e.results.length; i++) {
        transcript += e.results[i]?.[0]?.transcript ?? '';
      }
      setMessage((prev) => (prev ? `${prev} ${transcript}` : transcript));
    };
    recognition.onend = () => {
      setIsRecording(false);
      recognitionRef.current = null;
    };
    recognition.onerror = () => {
      setIsRecording(false);
      recognitionRef.current = null;
    };
    recognitionRef.current = recognition;
    recognition.start();
    setIsRecording(true);
  };

  const send = async (interrupt = false) => {
    const parts = [stripFileMentions(message).trim(), ...attachments.map((a) => a.path)];
    let text = parts.filter(Boolean).join('\n');
    // The interrupt button can fire with an empty input to apply the already
    // queued message right now instead of waiting for the turn to end.
    if (!text && interrupt && queuedMessage) {
      text = queuedMessage;
    }
    if (!text || sending) {
      return;
    }
    if (isDraft && agent) {
      const pid = agent.projectId;
      setSending(true);
      try {
        const model = agent.model ?? spawnModel;
        const input = provider
          ? { projectId: pid, kind: 'opencode', providerId: provider.id, task: text, model, isolated }
          : cliCfg?.id === 'opencode'
            ? { projectId: pid, kind: 'opencode', task: text, model, isolated }
            : cliCfg
              ? { projectId: pid, kind: cliCfg.id, task: text, model, binary: cliCfg.binary, isolated, allowedTools: allowedTools.length > 0 ? allowedTools : undefined }
              : null;
        const spawned = input ? await SpawnAgent(input) : null;
        if (spawned) {
          setMessage('');
          setAttachments([]);
          // The backend already persisted the spawned agent and emitted a change
          // event, so the collection syncs it in on its own. Re-inserting it here
          // would re-upsert the (intentionally empty) summary and race with — and
          // clobber — the title the backend generates asynchronously.
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
    setSending(true);
    lastSentRef.current = text;
    try {
      await (interrupt ? InterruptAndSendToAgent(agentId, text) : SendToAgent(agentId, text));
      setMessage('');
      setAttachments([]);
      onLogRefresh();
    } catch (err) {
      toastError({ title: t('agents.detail.couldNotSend'), err });
    } finally {
      setSending(false);
    }
  };

  const slashCommands = useMemo<SlashCommand[]>(() => {
    const list: SlashCommand[] = [];
    if (models.length > 0) {
      list.push({ name: 'model', description: t('agents.detail.commandModel'), takesArg: true, args: models.map((m) => ({ value: m.value, label: m.label })) });
    }
    if (isWorking) {
      list.push({ name: 'stop', description: t('agents.detail.commandStop') });
    }
    list.push({ name: 'logs', description: t('agents.detail.commandLogs') });
    list.push({ name: 'files', description: t('agents.detail.commandFiles') });
    list.push({ name: 'clear', description: t('agents.detail.commandClear') });
    if (isDraft && cliCfg?.id === 'claude-code') {
      list.push({
        name: 'tools',
        description: t('agents.detail.commandTools'),
        takesArg: true,
        args: [
          { value: 'all', label: t('agents.detail.toolsAll'), description: t('agents.detail.toolsAllDesc') },
          { value: 'readonly', label: t('agents.detail.toolsReadonly'), description: 'Read, Glob, Grep, LS, TodoRead, WebFetch, WebSearch, Task' },
          { value: 'no-web', label: t('agents.detail.toolsNoWeb'), description: 'Read, Write, Edit, MultiEdit, Bash, Glob, Grep, LS, Task, TodoRead, TodoWrite' },
          { value: 'no-tools', label: t('agents.detail.toolsNone'), description: t('agents.detail.toolsNoneDesc') },
        ],
      });
    }
    if (isDraft && teleportSessions.length > 0) {
      list.push({ name: 'teleport', description: t('agents.detail.commandTeleport'), takesArg: true, args: teleportSessions });
    }
    return list;
  }, [models, isWorking, t, isDraft, teleportSessions, cliCfg]);

  const runCommand = (name: string, args: string) => {
    switch (name) {
      case 'model': {
        const target = models.find((m) => m.value === args || m.label.toLowerCase() === args.toLowerCase());
        if (!target) {
          break;
        }
        if (isDraft) {
          if (agent) {
            agentsCollection.update(agent.id, (d) => {
              d.model = target.value;
            });
          }
        } else if (agent) {
          void SetAgentModel(agent.id, target.value).catch((err) => toastError({ title: t('agents.detail.couldNotSetModel'), err }));
        }
        break;
      }
      case 'stop':
        void cancel();
        break;
      case 'logs':
        onSetActiveTab('logs');
        break;
      case 'files':
        onSetActiveTab('files');
        break;
      case 'clear':
        onClearLog();
        void ClearAgentLog(agentId).catch((err) => toastError({ title: t('agents.detail.couldNotClear'), err }));
        break;
      case 'tools': {
        const preset = TOOL_PRESETS[args];
        if (preset !== undefined) {
          onAllowedToolsChange(preset);
        }
        break;
      }
      case 'teleport': {
        if (!agent) {
          break;
        }
        void TeleportClaudeSession(agent.projectId, args)
          .then(async (imported) => {
            if (!imported) {
              return;
            }
            try {
              await agentsCollection.insert(imported as unknown as Agent);
            } catch {
              // already synced via the agents:changed event — safe to ignore
            }
            try {
              await agentsCollection.delete(agent.id);
            } catch {
              // draft cleanup is best-effort
            }
            selectAgent(imported.id);
          })
          .catch((err) => toastError({ title: t('agents.detail.teleportFailed'), err }));
        break;
      }
    }
  };

  return (
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
              void RespondToAgentQuestion(agentId, pendingQuestion.toolUseId, content).catch((err) => toastError({ title: t('agents.askUserQuestion.failed'), err }));
              setPendingQuestion(null);
            }}
          />
        </div>
      ) : (
        <>
          {isDraft && models.length === 0 && <p className="mb-2 text-xs text-destructive">{t('agents.new.noModels')}</p>}
          <AttachmentPreviews attachments={attachments} onRemove={(i) => setAttachments((prev) => prev.filter((_, j) => j !== i))} />
          {isDraft && !isolated && projectId && !dirtyChecked ? (
            <>
              <Skeleton className="mb-2 h-8 w-full shrink-0 rounded-md" />
              <div className="flex items-start gap-2">
                <Skeleton className="size-10 shrink-0 rounded-md" />
                <Skeleton className="h-10 min-h-10 flex-1 rounded-md" />
                <Skeleton className="size-10 shrink-0 rounded-md" />
              </div>
            </>
          ) : (
            <>
              {dirtyWorktree && !warnDismissed && (
                <div className="mb-2 flex shrink-0 items-center gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                  <TriangleAlert className="size-3.5 shrink-0" />
                  <span className="flex-1">{t('agents.new.sharedBranchWarn')}</span>
                  <button type="button" aria-label={t('agents.new.dismissWarning')} onClick={() => setWarnDismissed(true)} className="shrink-0 rounded-sm opacity-70 hover:opacity-100">
                    <X className="size-3.5" />
                  </button>
                </div>
              )}
              {queuedMessage && (
                <div className="mb-2 flex items-center gap-2 rounded-md border border-border/60 bg-muted/40 px-2.5 py-1.5 text-xs text-muted-foreground">
                  <Clock className="size-3.5 shrink-0" />
                  <button
                    type="button"
                    title={t('agents.detail.editQueued')}
                    onClick={() => {
                      const queued = queuedMessage;
                      void (async () => {
                        if (await clearQueued()) {
                          recallIntoInput(queued);
                        }
                      })();
                    }}
                    className="flex-1 truncate text-left hover:text-foreground"
                  >
                    <span className="font-medium text-foreground/80">{t('agents.detail.queuedLabel')}</span> {queuedMessage}
                  </button>
                  <button type="button" aria-label={t('agents.detail.cancelQueued')} onClick={() => void clearQueued()} className="shrink-0 rounded-sm opacity-70 hover:opacity-100">
                    <X className="size-3.5" />
                  </button>
                </div>
              )}
              <div className="flex items-start gap-2">
                <Button variant="outline" size="icon" title={t('agents.detail.attachFiles')} onClick={() => void pickFiles()} className="size-10 shrink-0">
                  <Paperclip className="size-4" />
                </Button>
                <MentionTextarea
                  inputRef={inputRef}
                  placeholder={isDraft ? t('agents.new.taskPlaceholder') : t('agents.detail.sendPlaceholder')}
                  value={message}
                  onChange={setMessage}
                  onAttach={(path) => void addAttachment(path)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      void send();
                    }
                  }}
                  disabled={sending}
                  projectPath={project?.path}
                  commands={slashCommands}
                  onCommand={runCommand}
                  className="max-h-48 min-h-10 resize-none field-sizing-content"
                />
                {speechRecognitionAvailable && (
                  <Button variant={isRecording ? 'destructive' : 'outline'} size="icon" title={isRecording ? t('agents.detail.stopRecording') : t('agents.detail.recordVoice')} onClick={toggleRecording} className="size-10 shrink-0">
                    <Mic className="size-4" />
                  </Button>
                )}
                {isWorking && agent?.kind === 'claude-code' && (message.trim() || attachments.length > 0 || queuedMessage) && (
                  <Button variant="destructive" size="icon" title={t('agents.detail.interruptAndSend')} disabled={sending} onClick={() => void send(true)} className="size-10 shrink-0">
                    <Zap className="size-4" />
                  </Button>
                )}
                {message.trim() || attachments.length > 0 || !isWorking ? (
                  <Button size="icon" title={t('agents.detail.send')} disabled={sending || (!message.trim() && attachments.length === 0) || (isDraft && !spawnModel)} onClick={() => void send()} className="size-10">
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
        </>
      )}
    </div>
  );
}
