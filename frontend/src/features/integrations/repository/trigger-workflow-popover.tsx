import { useForm } from '@tanstack/react-form';
import { Play } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { toastError } from '@/lib/toast-error';
import { TriggerRepoWorkflow } from '@/wailsjs/go/main/App';
import { BranchCombobox } from './branch-combobox';
import type { WorkflowDispatchInput, WorkflowDispatchSpec } from './types';

const BRANCH_HINT = /\bbranch(es)?\b|\bref\b/i;

function looksLikeBranch(input: WorkflowDispatchInput): boolean {
  if (input.type !== 'string' && input.type !== '') {
    return false;
  }
  return BRANCH_HINT.test(input.name) || BRANCH_HINT.test(input.description);
}

interface Props {
  owner: string;
  repo: string;
  workflowId: number;
  workflowName: string;
  defaultRef: string;
  spec: WorkflowDispatchSpec;
  onTriggered: () => void;
}

interface FormValues {
  ref: string;
  values: Record<string, string>;
}

export function TriggerWorkflowPopover({ owner, repo, workflowId, workflowName, defaultRef, spec, onTriggered }: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const initialValues = useMemo(() => initialInputValues(spec.inputs), [spec.inputs]);

  const form = useForm({
    defaultValues: {
      ref: defaultRef,
      values: initialValues,
    } as FormValues,
    onSubmit: async ({ value }) => {
      const refTrimmed = value.ref.trim();
      try {
        await TriggerRepoWorkflow(owner, repo, workflowId, refTrimmed, value.values);
        toast.success(t('integrations.repository.triggeredWorkflow', { name: workflowName, ref: refTrimmed }));
        setOpen(false);
        onTriggered();
      } catch (err) {
        toastError({ title: t('integrations.repository.triggerFailed'), err });
      }
    },
  });

  const handleOpenChange = (next: boolean) => {
    if (next) {
      form.reset({ ref: defaultRef, values: initialValues });
    }
    setOpen(next);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange} modal>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs">
          <Play className="size-3.5" />
          {t('integrations.repository.runButton')}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 max-h-[70vh] overflow-y-auto" onCloseAutoFocus={(e) => e.preventDefault()}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
          className="flex flex-col gap-3"
        >
          <div className="flex flex-col gap-1">
            <span className="text-sm font-medium">{t('integrations.repository.runWorkflow')}</span>
            <span className="truncate text-xs text-muted-foreground">{workflowName}</span>
          </div>

          <form.Field name="ref" validators={{ onChange: validators.required(), onBlur: validators.required() }}>
            {(field) => (
              <Field label={t('integrations.repository.branchOrTag')} required>
                <BranchCombobox owner={owner} repo={repo} value={field.state.value} onChange={field.handleChange} placeholder="main" disabled={form.state.isSubmitting} />
                <FieldError field={field} />
              </Field>
            )}
          </form.Field>

          {spec.inputs.map((input) => (
            <form.Field
              key={input.name}
              name={`values.${input.name}`}
              validators={
                input.required
                  ? {
                      onChange: validators.required(),
                      onBlur: validators.required(),
                    }
                  : undefined
              }
            >
              {(field) => (
                <InputControl
                  input={input}
                  value={field.state.value ?? ''}
                  disabled={form.state.isSubmitting}
                  owner={owner}
                  repo={repo}
                  invalid={isInvalid(field)}
                  onChange={field.handleChange}
                  onBlur={field.handleBlur}
                  error={field.state.meta.isTouched ? (field.state.meta.errors.find((e) => typeof e === 'string') as string | undefined) : undefined}
                />
              )}
            </form.Field>
          ))}

          <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
            {([canSubmit, isSubmitting]) => (
              <Button type="submit" size="sm" disabled={!canSubmit || isSubmitting}>
                <Play className="size-3.5" />
                {isSubmitting ? t('integrations.repository.triggering') : t('integrations.repository.runWorkflow')}
              </Button>
            )}
          </form.Subscribe>
        </form>
      </PopoverContent>
    </Popover>
  );
}

function initialInputValues(inputs: WorkflowDispatchInput[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const input of inputs) {
    if (input.default) {
      out[input.name] = input.default;
    } else if (input.type === 'boolean') {
      out[input.name] = 'false';
    } else if (input.type === 'choice' && input.options.length > 0) {
      out[input.name] = input.options[0];
    } else {
      out[input.name] = '';
    }
  }
  return out;
}

function Field({ label, required, children, hint }: { label: string; required?: boolean; children: React.ReactNode; hint?: string }) {
  return (
    <div className="flex flex-col gap-1 text-xs">
      <span className="font-medium">
        {label}
        {required && <span className="ml-0.5 text-destructive">*</span>}
      </span>
      {children}
      {hint && <span className="text-muted-foreground">{hint}</span>}
    </div>
  );
}

interface InputControlProps {
  input: WorkflowDispatchInput;
  value: string;
  disabled: boolean;
  owner: string;
  repo: string;
  invalid: boolean;
  onChange: (v: string) => void;
  onBlur: () => void;
  error: string | undefined;
}

function InputControl({ input, value, disabled, owner, repo, invalid, onChange, onBlur, error }: InputControlProps) {
  const { t } = useTranslation();
  const label = input.description || input.name;

  if (looksLikeBranch(input)) {
    return (
      <Field label={label} required={input.required}>
        <BranchCombobox owner={owner} repo={repo} value={value} onChange={onChange} placeholder={input.default || 'main'} disabled={disabled} />
        {error && <p className="text-xs text-destructive">{error}</p>}
      </Field>
    );
  }

  if (input.type === 'boolean') {
    return (
      <div className="flex flex-col gap-1 text-xs">
        <div className="flex items-center justify-between gap-2">
          <span className="font-medium">
            {label}
            {input.required && <span className="ml-0.5 text-destructive">*</span>}
          </span>
          <Switch checked={value === 'true'} onCheckedChange={(checked) => onChange(checked ? 'true' : 'false')} disabled={disabled} />
        </div>
        {input.description && input.description !== label && <span className="text-muted-foreground">{input.description}</span>}
      </div>
    );
  }

  if (input.type === 'choice' && input.options.length > 0) {
    return (
      <Field label={label} required={input.required}>
        <Select value={value} onValueChange={onChange} disabled={disabled}>
          <SelectTrigger size="sm" aria-invalid={invalid}>
            <SelectValue placeholder={t('integrations.repository.selectPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {input.options.map((opt) => (
              <SelectItem key={opt} value={opt}>
                {opt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {error && <p className="text-xs text-destructive">{error}</p>}
      </Field>
    );
  }

  return (
    <Field label={label} required={input.required}>
      <Input type={input.type === 'number' ? 'number' : 'text'} value={value} onChange={(e) => onChange(e.target.value)} onBlur={onBlur} placeholder={input.default || ''} disabled={disabled} aria-invalid={invalid} />
      {error && <p className="text-xs text-destructive">{error}</p>}
    </Field>
  );
}
