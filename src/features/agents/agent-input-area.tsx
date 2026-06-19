import { useLiveQuery } from '@tanstack/react-db';
import { Mic, Paperclip, Send, Square, TriangleAlert, X } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
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
import { CancelAgent, ClearAgentLog, GetProjectFileStatuses, ListClaudeCodeSessions, PickFiles, ReadFileBase64, RespondToAgentQuestion, SendToAgent, SetAgentModel, SpawnAgent, TeleportClaudeSession } from '@/wailsjs/go/main/App';
import { EventsOn } from '@/wailsjs/runtime/runtime';
import { AskUserQuestionPanel, type AskUserQuestionPayload } from './ask-user-question-panel';
import { stripFileMentions } from './file-mentions';
import { MentionTextarea, type SlashCommand } from './mention-textarea';
import { basename, ImageZoom } from './user-message';

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
type DraftState = { message: string; attachments: { path: string; preview: string }[] };
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

  const provider = providers.find((p) => p.id === agent?.providerId) ?? undefined;
  const cliCfg = agent && !agent.providerId ? (agent.kind === 'opencode' ? opencode : cliKinds.find((k) => k.id === agent.kind)) : undefined;
  const models: { value: string; label: string }[] = cliCfg ? cliCfg.models : provider ? provider.models.map((m) => ({ value: m, label: m })) : [];
  const defaultsId = provider?.id ?? cliCfg?.id;
  const spawnModel = (defaultsId ? agentDefaults.get(defaultsId) : undefined) ?? models[0]?.value ?? '';
  const isolated = project?.isolatedDefault ?? false;

  const [message, setMessage] = useState(() => drafts.get(agentId)?.message ?? '');
  const [attachments, setAttachments] = useState<{ path: string; preview: string }[]>(() => drafts.get(agentId)?.attachments ?? []);
  const [sending, setSending] = useState(false);
  const [isRecording, setIsRecording] = useState(false);
  const recognitionRef = useRef<{ stop(): void } | null>(null);
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

  useEffect(() => {
    if (!isWorking) {
      return;
    }
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        CancelAgent(agentId).catch((err) => toastError({ title: t('agents.detail.couldNotStop'), err }));
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isWorking, agentId, t]);

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
    try {
      await SendToAgent(agentId, text);
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
          {attachments.length > 0 && (
            <div className="mb-2 flex flex-wrap gap-2">
              {attachments.map((att, i) => (
                <div key={att.path} className="group relative">
                  {att.preview ? <ImageZoom src={att.preview} name={basename(att.path)} /> : <div className="flex h-20 w-24 items-center justify-center rounded-md border bg-muted text-xs text-muted-foreground">{basename(att.path)}</div>}
                  <button type="button" onClick={() => setAttachments((prev) => prev.filter((_, j) => j !== i))} className="absolute -right-1.5 -top-1.5 z-10 flex size-5 items-center justify-center rounded-full bg-destructive text-destructive-foreground opacity-0 shadow-sm transition-opacity group-hover:opacity-100">
                    <X className="size-3" />
                  </button>
                </div>
              ))}
            </div>
          )}
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
