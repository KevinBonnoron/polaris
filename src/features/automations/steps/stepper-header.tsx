import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { STEP_LABELS, STEPS, type Step } from './types';

interface Props {
  step: Step;
}

export function StepperHeader({ step }: Props) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      {STEPS.map((s, idx) => (
        <div key={s} className="flex flex-1 items-center gap-2">
          <span className={cn('flex size-6 shrink-0 items-center justify-center rounded-full border text-xs', s === step && 'border-blue-500 bg-blue-500/10 text-blue-200', s < step && 'border-blue-500/40 bg-blue-500/5 text-blue-300', s > step && 'border-border text-muted-foreground')}>{s}</span>
          <span className={cn('text-sm', s === step ? 'text-foreground' : 'text-muted-foreground')}>{t(`automations.steps.${STEP_LABELS[s]}`)}</span>
          {idx < STEPS.length - 1 && <div className={cn('h-px flex-1', s < step ? 'bg-blue-500/40' : 'bg-border')} />}
        </div>
      ))}
    </div>
  );
}
