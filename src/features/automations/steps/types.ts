import type { ReactFormExtendedApi } from '@tanstack/react-form';
import type { Automation } from '@/types';

// biome-ignore lint/suspicious/noExplicitAny: validator generics are not used at the form level; field-level safety comes from `form.Field name="..."` paths.
export type AutomationForm = ReactFormExtendedApi<Automation, any, any, any, any, any, any, any, any, any, any, any>;

export type Step = 1 | 2 | 3 | 4;
export const STEPS: Step[] = [1, 2, 3, 4];
export const STEP_LABELS: Record<Step, 'setup' | 'trigger' | 'agent' | 'review'> = {
  1: 'setup',
  2: 'trigger',
  3: 'agent',
  4: 'review',
};
