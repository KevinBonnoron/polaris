import { useLiveQuery } from '@tanstack/react-db';
import { useForm, useStore } from '@tanstack/react-form';
import { type PropsWithChildren, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { automationsCollection } from '@/collections/automations.collection';
import { projectsCollection } from '@/collections/projects.collection';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import { useJiraStatuses } from '@/features/integrations/jira/use-jira-sprint';
import { useAgentClis } from '@/state/agent-clis';
import type { AutomationSource, Project } from '@/types';
import { ActionsStep } from './steps/actions-step';
import { ReviewStep } from './steps/review-step';
import { SetupStep } from './steps/setup-step';
import { StepperHeader } from './steps/stepper-header';
import { TriggerStep } from './steps/trigger-step';
import { buildDefaultAutomation, projectIntegrations } from './steps/triggers';
import type { AutomationForm, Step } from './steps/types';

type JiraCfg = NonNullable<NonNullable<Project['integrations']>['jira']>;

interface Props {
  automationId?: string;
  projectId?: string;
  source?: AutomationSource;
}

export function AutomationEditModal({ automationId, projectId: payloadProjectId, source: payloadSource, children }: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { kinds: agentKinds } = useAgentClis();

  const { data: projects = [] } = useLiveQuery((q) => q.from({ p: projectsCollection }));
  const { data: automations = [] } = useLiveQuery((q) => q.from({ a: automationsCollection }));
  const existing = automationId ? automations.find((a) => a.id === automationId) : null;

  const projectId = existing?.projectId ?? payloadProjectId ?? '';
  const project = projects.find((p) => p.id === projectId) ?? null;
  const availableSources = useMemo(() => projectIntegrations(project), [project]);
  const initialSource = payloadSource ?? availableSources[0] ?? 'jira';

  const close = () => setOpen(false);

  const form = useForm({
    defaultValues: existing ?? buildDefaultAutomation(projectId, initialSource),
    onSubmit: async ({ value }) => {
      try {
        if (existing) {
          automationsCollection.update(existing.id, (draft) => {
            Object.assign(draft, value, { id: existing.id });
          });
        } else {
          automationsCollection.insert(value);
        }
        close();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : String(err));
      }
    },
  }) as AutomationForm;

  useEffect(() => {
    if (existing && existing.id !== form.state.values.id) {
      form.reset(existing);
    }
  }, [existing, form]);

  const jiraConfig: JiraCfg | null = (project?.integrations?.jira as JiraCfg | undefined) ?? null;
  const hasJira = Boolean(jiraConfig?.baseUrl && jiraConfig.email && jiraConfig.token && jiraConfig.projectKey);
  const resendConfig = project?.integrations?.resend as { apiKey?: string; fromEmail?: string } | undefined;
  const hasResend = Boolean(resendConfig?.apiKey && resendConfig?.fromEmail);

  const currentSource = useStore(form.store, (state) => state.values.source);

  const statusesCfg = useMemo(
    () =>
      hasJira && currentSource === 'jira' && jiraConfig
        ? {
            baseUrl: String(jiraConfig.baseUrl ?? ''),
            email: String(jiraConfig.email ?? ''),
            token: String(jiraConfig.token ?? ''),
            projectKey: String(jiraConfig.projectKey ?? ''),
          }
        : null,
    [hasJira, currentSource, jiraConfig],
  );
  const { statuses, loading: statusesLoading, error: statusError } = useJiraStatuses(statusesCfg);

  const isCreating = !existing;
  const [step, setStep] = useState<Step>(1);

  const stepValid = useStore(form.store, (state) => {
    const v = state.values;
    if (step === 1) {
      return v.name.trim().length > 0 && Boolean(v.source) && availableSources.includes(v.source) && (v.source !== 'jira' || hasJira);
    }
    if (step === 2) {
      if (v.trigger.kind === 'jira.transition' && !v.trigger.toStatusId) {
        return false;
      }
      return true;
    }
    if (step === 3) {
      if (v.actions.length === 0) {
        return false;
      }
      for (const action of v.actions) {
        if (action.kind === 'spawn_agent' && (!action.agentKind || !action.taskTemplate?.trim())) {
          return false;
        }
        if (action.kind === 'jira_transition' && !action.jiraToStatusId) {
          return false;
        }
        if (action.kind === 'notification' && !action.notifyTitle?.trim()) {
          return false;
        }
        if (action.kind === 'send_email' && (!action.emailTo?.trim() || !action.emailSubject?.trim())) {
          return false;
        }
      }
      return true;
    }
    return true;
  });

  const goNext = () => {
    if (step < 4) {
      setStep((step + 1) as Step);
    }
  };
  const goBack = () => {
    if (step > 1) {
      setStep((step - 1) as Step);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && form.state.isSubmitting) {
          return;
        }
        if (next && !existing) {
          form.reset(buildDefaultAutomation(projectId, initialSource));
          setStep(1);
        }
        setOpen(next);
      }}
    >
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="w-[min(95vw,860px)] gap-4 sm:max-w-[860px]">
        <DialogHeader>
          <DialogTitle>{existing ? t('automations.editTitle') : t('automations.newTitle')}</DialogTitle>
          <DialogDescription>{t('automations.editDesc')}</DialogDescription>
        </DialogHeader>

        {isCreating && <StepperHeader step={step} />}

        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (isCreating && step < 4) {
              if (stepValid) {
                goNext();
              }
              return;
            }
            void form.handleSubmit();
          }}
          className="flex flex-col gap-4"
        >
          <ScrollArea className="-mr-3 max-h-[70vh] pr-3">
            {isCreating ? (
              <>
                {step === 1 && <SetupStep form={form} availableSources={availableSources} hasJira={hasJira} />}
                {step === 2 && <TriggerStep form={form} hasJira={hasJira} statuses={statuses} statusesLoading={statusesLoading} statusError={statusError} />}
                {step === 3 && <ActionsStep form={form} agentKinds={agentKinds} statuses={statuses} hasJiraIntegration={hasJira} hasResendIntegration={hasResend} />}
                {step === 4 && <form.Subscribe selector={(state) => state.values}>{(values) => <ReviewStep values={values} agentKinds={agentKinds} statuses={statuses} />}</form.Subscribe>}
              </>
            ) : (
              <div className="flex flex-col gap-4">
                <SetupStep form={form} availableSources={availableSources} hasJira={hasJira} />
                <TriggerStep form={form} hasJira={hasJira} statuses={statuses} statusesLoading={statusesLoading} statusError={statusError} />
                <ActionsStep form={form} agentKinds={agentKinds} statuses={statuses} hasJiraIntegration={hasJira} hasResendIntegration={hasResend} />
              </div>
            )}
          </ScrollArea>

          <DialogFooter>
            {isCreating ? (
              <>
                <Button type="button" variant="ghost" onClick={step === 1 ? close : goBack} disabled={form.state.isSubmitting}>
                  {step === 1 ? t('common.cancel') : t('automations.back')}
                </Button>
                {step < 4 ? (
                  <Button type="button" disabled={!stepValid} onClick={goNext}>
                    {t('automations.next')}
                  </Button>
                ) : (
                  <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
                    {([canSubmit, isSubmitting]) => (
                      <Button type="submit" disabled={!canSubmit || isSubmitting}>
                        {isSubmitting ? t('automations.saving') : t('automations.create')}
                      </Button>
                    )}
                  </form.Subscribe>
                )}
              </>
            ) : (
              <>
                <Button type="button" variant="ghost" onClick={close} disabled={form.state.isSubmitting}>
                  {t('common.cancel')}
                </Button>
                <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
                  {([canSubmit, isSubmitting]) => (
                    <Button type="submit" disabled={!canSubmit || isSubmitting}>
                      {isSubmitting ? t('automations.saving') : t('common.save')}
                    </Button>
                  )}
                </form.Subscribe>
              </>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
