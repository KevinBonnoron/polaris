import { ChevronLeft, ChevronRight, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { Markdown } from './log-format';
import { MentionTextarea } from './mention-textarea';

export interface AskQuestionOption {
  label: string;
  description?: string;
}

export interface AskQuestion {
  question: string;
  header?: string;
  multiSelect?: boolean;
  options: AskQuestionOption[];
}

export interface AskUserQuestionPayload {
  questions: AskQuestion[];
}

interface Props {
  payload: AskUserQuestionPayload;
  projectPath: string | undefined;
  onSubmit: (answers: Array<{ question: string; answer: string | string[] }>) => void;
  onCancel: () => void;
}

const OTHER_KEY = '__other__';

export function AskUserQuestionPanel({ payload, projectPath, onSubmit, onCancel }: Props) {
  const { t } = useTranslation();
  const [step, setStep] = useState(0);
  const [single, setSingle] = useState<Record<number, string>>({});
  const [multi, setMulti] = useState<Record<number, Set<string>>>({});
  const [other, setOther] = useState<Record<number, string>>({});

  const total = payload.questions.length;
  const current = payload.questions[step];
  const isLast = step === total - 1;

  // Esc dismisses the question/plan, matching the "Esc to cancel" hint and the X
  // button. Capture phase so it fires even while a textarea inside has focus.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        onCancel();
      }
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, [onCancel]);

  const isAnswered = (idx: number) => {
    const q = payload.questions[idx];
    if (q.multiSelect) {
      const picked = multi[idx];
      if (!picked || picked.size === 0) {
        return false;
      }
      if (picked.has(OTHER_KEY) && !(other[idx] ?? '').trim()) {
        return false;
      }
      return true;
    }
    const choice = single[idx];
    if (!choice) {
      return false;
    }
    if (choice === OTHER_KEY && !(other[idx] ?? '').trim()) {
      return false;
    }
    return true;
  };

  const toggleMulti = (label: string) => {
    setMulti((prev) => {
      const next = new Set(prev[step] ?? []);
      if (next.has(label)) next.delete(label);
      else next.add(label);
      return { ...prev, [step]: next };
    });
  };

  const submit = () => {
    const answers = payload.questions.map((q, idx) => {
      if (q.multiSelect) {
        const picked = Array.from(multi[idx] ?? []);
        if (picked.includes(OTHER_KEY)) {
          const replaced = picked.map((p) => (p === OTHER_KEY ? `Other: ${other[idx] ?? ''}` : p));
          return { question: q.question, answer: replaced };
        }
        return { question: q.question, answer: picked };
      }
      const choice = single[idx];
      if (choice === OTHER_KEY) {
        return { question: q.question, answer: `Other: ${other[idx] ?? ''}` };
      }
      return { question: q.question, answer: choice ?? '' };
    });
    onSubmit(answers);
  };

  const optionsWithOther = [...current.options, { label: OTHER_KEY, description: undefined } as AskQuestionOption];
  const canAdvance = isAnswered(step);
  const allAnswered = payload.questions.every((_, idx) => isAnswered(idx));

  // Auto-advance to the next step on a single-select pick (skipping Other,
  // since picking it opens an input the user still needs to fill). Multi-select
  // questions don't auto-advance — the user is mid-selection.
  const pickSingle = (label: string) => {
    setSingle((prev) => ({ ...prev, [step]: label }));
    if (label === OTHER_KEY || isLast) {
      return;
    }
    setStep((s) => Math.min(total - 1, s + 1));
  };

  return (
    <div className="flex w-full shrink-0 flex-col gap-3 rounded-md border border-border bg-muted/30 p-3">
      <div className="flex shrink-0 items-center justify-between gap-3">
        <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('agents.askUserQuestion.title', { defaultValue: 'Agent is asking a question' })}</span>
        <div className="flex items-center gap-3">
          {total > 1 && (
            <div className="flex items-center gap-1.5">
              {payload.questions.map((_, idx) => (
                <button
                  type="button"
                  // biome-ignore lint/suspicious/noArrayIndexKey: questions order is stable for the lifetime of the panel
                  key={idx}
                  onClick={() => setStep(idx)}
                  aria-label={t('agents.askUserQuestion.goToStep', { defaultValue: 'Go to question {{n}}', n: idx + 1 })}
                  className={cn('size-2 rounded-full transition-colors', idx === step ? 'bg-primary' : isAnswered(idx) ? 'bg-primary/40' : 'bg-muted-foreground/30 hover:bg-muted-foreground/50')}
                />
              ))}
            </div>
          )}
          <button type="button" onClick={onCancel} aria-label={t('agents.askUserQuestion.cancel', { defaultValue: 'Cancel' })} className="rounded-sm text-muted-foreground transition-colors hover:text-foreground">
            <X className="size-4" />
          </button>
        </div>
      </div>

      <div className="flex flex-col gap-2">
        {current.header && <span className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">{current.header}</span>}
        <ScrollArea className="max-h-64 text-sm text-foreground [&_*]:font-sans [&_code]:font-mono">
          <Markdown>{current.question}</Markdown>
        </ScrollArea>
        <div className="flex flex-col gap-1.5">
          {optionsWithOther.map((opt) => {
            const isOther = opt.label === OTHER_KEY;
            const checked = current.multiSelect ? (multi[step]?.has(opt.label) ?? false) : single[step] === opt.label;
            return (
              <button
                type="button"
                key={opt.label}
                onClick={() => (current.multiSelect ? toggleMulti(opt.label) : pickSingle(opt.label))}
                className={cn('flex flex-col gap-0.5 rounded-md border px-3 py-2 text-left text-sm transition-colors', checked ? 'border-primary bg-primary/10' : 'border-border bg-background hover:bg-muted')}
              >
                <span className="font-medium text-foreground">{isOther ? t('agents.askUserQuestion.other', { defaultValue: 'Other' }) : opt.label}</span>
                {opt.description && <span className="text-xs text-muted-foreground">{opt.description}</span>}
              </button>
            );
          })}
        </div>
        {((current.multiSelect && multi[step]?.has(OTHER_KEY)) || (!current.multiSelect && single[step] === OTHER_KEY)) && (
          <MentionTextarea
            autoFocus
            placeholder={t('agents.askUserQuestion.otherPlaceholder', { defaultValue: 'Your answer' })}
            value={other[step] ?? ''}
            onChange={(value) => setOther((prev) => ({ ...prev, [step]: value }))}
            projectPath={projectPath}
            className="max-h-48 min-h-10 resize-none field-sizing-content"
          />
        )}
      </div>

      <div className="flex shrink-0 items-center justify-between gap-2">
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{t('agents.askUserQuestion.escToCancel', { defaultValue: 'Esc to cancel' })}</span>
        <div className="flex items-center gap-2">
          {total > 1 && (
            <>
              <Button variant="outline" size="sm" onClick={() => setStep((s) => Math.max(0, s - 1))} disabled={step === 0}>
                <ChevronLeft className="size-4" />
                {t('agents.askUserQuestion.previous', { defaultValue: 'Previous' })}
              </Button>
              <span className="text-xs tabular-nums text-muted-foreground">
                {step + 1} / {total}
              </span>
            </>
          )}
          {isLast ? (
            <Button size="sm" onClick={submit} disabled={!allAnswered}>
              {t('agents.askUserQuestion.submit', { defaultValue: 'Submit' })}
            </Button>
          ) : (
            <Button size="sm" onClick={() => setStep((s) => Math.min(total - 1, s + 1))} disabled={!canAdvance}>
              {t('agents.askUserQuestion.next', { defaultValue: 'Next' })}
              <ChevronRight className="size-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
