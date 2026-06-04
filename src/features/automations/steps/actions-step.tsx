import { GripVertical, Plus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { JiraStatus } from '@/collections/jira.issues.collection';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import type { useAgentClis } from '@/state/agent-clis';
import type { ActionKind, AutomationAction, JiraTransitionAction, NotificationAction, SendEmailAction, SendMessageAction, SpawnAgentAction } from '@/types';
import { defaultActionForKind, TRIGGERS } from './triggers';
import type { AutomationForm } from './types';

type AgentKindInfo = ReturnType<typeof useAgentClis>['kinds'][number];

interface Props {
  form: AutomationForm;
  agentKinds: AgentKindInfo[];
  statuses: JiraStatus[];
  hasJiraIntegration: boolean;
  hasResendIntegration: boolean;
  hasMessagingIntegration: boolean;
  messagingProviders: string[];
}

const ACTION_KINDS: ActionKind[] = ['spawn_agent', 'jira_transition', 'notification', 'send_email', 'send_message'];

export function ActionsStep({ form, agentKinds, statuses, hasJiraIntegration, hasResendIntegration, hasMessagingIntegration, messagingProviders }: Props) {
  const { t } = useTranslation();

  const availableKinds = ACTION_KINDS.filter((k) => {
    if (k === 'jira_transition') { return hasJiraIntegration; }
    if (k === 'send_email') { return hasResendIntegration; }
    if (k === 'send_message') { return hasMessagingIntegration; }
    return true;
  });

  const addAction = (kind: ActionKind) => {
    const triggerKind = form.state.values.trigger.kind;
    const template = kind === 'spawn_agent' ? TRIGGERS[triggerKind].defaultTemplate : '';
    const next = [...form.state.values.actions, defaultActionForKind(kind, template)];
    form.setFieldValue('actions', next);
  };

  const removeAction = (index: number) => {
    const next = form.state.values.actions.filter((_, i) => i !== index);
    form.setFieldValue('actions', next);
  };

  const updateAction = (index: number, patch: Partial<AutomationAction>) => {
    const next = form.state.values.actions.map((a, i) => (i === index ? ({ ...a, ...patch } as AutomationAction) : a));
    form.setFieldValue('actions', next);
  };

  return (
    <form.Subscribe selector={(state) => state.values.actions}>
      {(actions) => (
        <div className="flex flex-col gap-3">
          <div className="flex flex-col">
            <Label>{t('automations.actions.title')}</Label>
            <p className="text-xs text-muted-foreground">{t('automations.actions.desc')}</p>
          </div>

          {actions.length > 0 && (
            <div className="flex flex-col gap-3">
              {actions.map((action, index) => (
                <ActionCard
                  // biome-ignore lint/suspicious/noArrayIndexKey: actions are an ordered list with no stable id
                  key={index}
                  index={index}
                  action={action}
                  agentKinds={agentKinds}
                  statuses={statuses}
                  hasJiraIntegration={hasJiraIntegration}
                  hasResendIntegration={hasResendIntegration}
                  hasMessagingIntegration={hasMessagingIntegration}
                  messagingProviders={messagingProviders}
                  onChange={(patch) => updateAction(index, patch)}
                  onRemove={() => removeAction(index)}
                />
              ))}
            </div>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="flex w-full items-center justify-center gap-2 rounded-md border border-dashed py-6 text-sm text-muted-foreground transition-colors hover:border-blue-500 hover:bg-accent hover:text-foreground">
                <Plus className="size-4" />
                {t('automations.actions.addLabel')}
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="center" className="w-[var(--radix-dropdown-menu-trigger-width)]">
              {availableKinds.map((k) => (
                <DropdownMenuItem key={k} onSelect={() => addAction(k)}>
                  {t(`automations.actions.add.${k}`)}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
    </form.Subscribe>
  );
}

interface ActionCardProps {
  index: number;
  action: AutomationAction;
  agentKinds: AgentKindInfo[];
  statuses: JiraStatus[];
  hasJiraIntegration: boolean;
  hasResendIntegration: boolean;
  hasMessagingIntegration: boolean;
  messagingProviders: string[];
  onChange: (patch: Partial<AutomationAction>) => void;
  onRemove: () => void;
}

function ActionCard({ index, action, agentKinds, statuses, hasJiraIntegration, hasResendIntegration, hasMessagingIntegration, messagingProviders, onChange, onRemove }: ActionCardProps) {
  const { t } = useTranslation();

  return (
    <div className="rounded-md border p-3">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 text-sm font-medium">
          <GripVertical className="size-4 text-muted-foreground" />
          <span className="text-muted-foreground">#{index + 1}</span>
          <span>{t(`automations.actions.kinds.${action.kind}`)}</span>
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={onRemove} aria-label={t('automations.actions.remove')}>
          <Trash2 className="size-3.5" />
        </Button>
      </div>

      {action.kind === 'spawn_agent' && <SpawnAgentEditor action={action} agentKinds={agentKinds} onChange={onChange} />}
      {action.kind === 'jira_transition' && <JiraTransitionEditor action={action} statuses={statuses} hasJiraIntegration={hasJiraIntegration} onChange={onChange} />}
      {action.kind === 'notification' && <NotificationEditor action={action} onChange={onChange} />}
      {action.kind === 'send_email' && <SendEmailEditor action={action} hasResendIntegration={hasResendIntegration} onChange={onChange} />}
      {action.kind === 'send_message' && <SendMessageEditor action={action} hasMessagingIntegration={hasMessagingIntegration} messagingProviders={messagingProviders} onChange={onChange} />}
    </div>
  );
}

function SpawnAgentEditor({ action, agentKinds, onChange }: { action: SpawnAgentAction; agentKinds: AgentKindInfo[]; onChange: (patch: Partial<AutomationAction>) => void }) {
  const { t } = useTranslation();
  const kind = agentKinds.find((k) => k.id === action.agentKind);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.agentKind')}</Label>
        <Select value={action.agentKind} onValueChange={(v) => onChange({ agentKind: v })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {agentKinds.map((k) => (
              <SelectItem key={k.id} value={k.id} disabled={!k.installed}>
                {k.label}
                {!k.installed && ` · ${t('automations.agentKindNotInstalled')}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.model')}</Label>
        <Select value={action.model ?? ''} onValueChange={(v) => onChange({ model: v })}>
          <SelectTrigger>
            <SelectValue placeholder={t('automations.modelPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {(kind?.models ?? []).map((m) => (
              <SelectItem key={m.value} value={m.value}>
                {m.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.template')}</Label>
        <Textarea value={action.taskTemplate} onChange={(e) => onChange({ taskTemplate: e.target.value })} rows={5} className="font-mono text-xs" />
      </div>
    </div>
  );
}

function JiraTransitionEditor({ action, statuses, hasJiraIntegration, onChange }: { action: JiraTransitionAction; statuses: JiraStatus[]; hasJiraIntegration: boolean; onChange: (patch: Partial<AutomationAction>) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      {!hasJiraIntegration && <p className="text-xs text-amber-500">{t('automations.actions.noJiraIntegration')}</p>}

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.jiraToStatus')}</Label>
        <Select value={action.jiraToStatusId} onValueChange={(v) => onChange({ jiraToStatusId: v })}>
          <SelectTrigger>
            <SelectValue placeholder={t('automations.actions.jiraToStatusPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {statuses.flatMap((s) =>
              s.statusIds.map((id) => (
                <SelectItem key={id} value={id}>
                  {s.name}
                </SelectItem>
              )),
            )}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.jiraIssueKey')}</Label>
        <Input value={action.jiraIssueKey ?? ''} onChange={(e) => onChange({ jiraIssueKey: e.target.value })} placeholder={t('automations.actions.jiraIssueKeyPlaceholder')} className="font-mono text-xs" />
        <p className="text-xs text-muted-foreground">{t('automations.actions.jiraIssueKeyHint')}</p>
      </div>
    </div>
  );
}

function NotificationEditor({ action, onChange }: { action: NotificationAction; onChange: (patch: Partial<AutomationAction>) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.notifyTitle')}</Label>
        <Input value={action.notifyTitle} onChange={(e) => onChange({ notifyTitle: e.target.value })} placeholder={t('automations.actions.notifyTitlePlaceholder')} />
        <p className="text-xs text-muted-foreground">{t('automations.actions.notifyTitleHint')}</p>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.notifyKind')}</Label>
        <Select value={action.notifyKind ?? 'info'} onValueChange={(v) => onChange({ notifyKind: v as NotificationAction['notifyKind'] })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="info">{t('automations.actions.notifyKinds.info')}</SelectItem>
            <SelectItem value="success">{t('automations.actions.notifyKinds.success')}</SelectItem>
            <SelectItem value="warning">{t('automations.actions.notifyKinds.warning')}</SelectItem>
            <SelectItem value="error">{t('automations.actions.notifyKinds.error')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

function SendEmailEditor({ action, hasResendIntegration, onChange }: { action: SendEmailAction; hasResendIntegration: boolean; onChange: (patch: Partial<AutomationAction>) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      {!hasResendIntegration && <p className="text-xs text-amber-500">{t('automations.actions.noResendIntegration')}</p>}

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.emailTo')}</Label>
        <Input value={action.emailTo} onChange={(e) => onChange({ emailTo: e.target.value })} placeholder={t('automations.actions.emailToPlaceholder')} className="font-mono text-xs" />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.emailSubject')}</Label>
        <Input value={action.emailSubject} onChange={(e) => onChange({ emailSubject: e.target.value })} placeholder={t('automations.actions.emailSubjectPlaceholder')} />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.emailBody')}</Label>
        <Textarea value={action.emailBody ?? ''} onChange={(e) => onChange({ emailBody: e.target.value })} rows={4} placeholder={t('automations.actions.emailBodyPlaceholder')} />
        <p className="text-xs text-muted-foreground">{t('automations.actions.emailBodyHint')}</p>
      </div>
    </div>
  );
}

function SendMessageEditor({ action, hasMessagingIntegration, messagingProviders, onChange }: { action: SendMessageAction; hasMessagingIntegration: boolean; messagingProviders: string[]; onChange: (patch: Partial<AutomationAction>) => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      {!hasMessagingIntegration && <p className="text-xs text-amber-500">{t('automations.actions.noMessagingIntegration')}</p>}

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.messageProvider')}</Label>
        <Select value={action.messageProvider} onValueChange={(v) => onChange({ messageProvider: v as SendMessageAction['messageProvider'] })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {messagingProviders.map((p) => (
              <SelectItem key={p} value={p}>
                {p.charAt(0).toUpperCase() + p.slice(1)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.messageTitle')}</Label>
        <Input value={action.messageTitle ?? ''} onChange={(e) => onChange({ messageTitle: e.target.value })} placeholder={t('automations.actions.messageTitlePlaceholder')} className="font-mono text-xs" />
        <p className="text-xs text-muted-foreground">{t('automations.actions.messageTitleHint')}</p>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label>{t('automations.actions.messageBody')}</Label>
        <Textarea value={action.messageBody ?? ''} onChange={(e) => onChange({ messageBody: e.target.value })} rows={3} placeholder={t('automations.actions.messageBodyPlaceholder')} className="font-mono text-xs" />
      </div>
    </div>
  );
}
