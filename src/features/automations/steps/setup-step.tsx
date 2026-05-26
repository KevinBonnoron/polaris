import { useTranslation } from 'react-i18next';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { FieldError, isInvalid, validators } from '@/lib/form';
import type { AutomationSource } from '@/types';
import { applyTriggerKind, DEFAULT_KIND_FOR_SOURCE, SOURCE_ICONS } from './triggers';
import type { AutomationForm } from './types';

interface Props {
  form: AutomationForm;
  availableSources: AutomationSource[];
  hasJira: boolean;
}

export function SetupStep({ form, availableSources, hasJira }: Props) {
  const { t } = useTranslation();

  const switchSource = (source: AutomationSource) => {
    if (source !== form.state.values.source) {
      applyTriggerKind(form, DEFAULT_KIND_FOR_SOURCE[source]);
    }
  };

  return (
    <form.Subscribe selector={(state) => state.values}>
      {(values) => (
        <div className="flex flex-col gap-4">
          <div className="flex items-start gap-3">
            <form.Field name="name" validators={{ onChange: validators.required(), onBlur: validators.required() }}>
              {(field) => (
                <div className="flex flex-1 flex-col gap-2">
                  <Label htmlFor="auto-name">{t('automations.name')}</Label>
                  <Input id="auto-name" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('automations.namePlaceholder')} aria-invalid={isInvalid(field)} />
                  <FieldError field={field} />
                </div>
              )}
            </form.Field>
            <form.Field name="enabled">
              {(field) => (
                <div className="flex flex-col gap-2">
                  <Label aria-hidden className="invisible">
                    &nbsp;
                  </Label>
                  <div className="flex h-9 items-center gap-2">
                    <Switch checked={field.state.value} onCheckedChange={field.handleChange} />
                    <span className="text-sm">{field.state.value ? t('automations.enabled') : t('automations.disabled')}</span>
                  </div>
                </div>
              )}
            </form.Field>
          </div>

          <div className="flex flex-col gap-2">
            <Label>{t('automations.integration')}</Label>
            {availableSources.length === 0 ? (
              <p className="rounded-md border border-dashed bg-muted/40 px-3 py-2 text-xs text-muted-foreground">{t('automations.noIntegrations')}</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {availableSources.map((s) => {
                  const Icon = SOURCE_ICONS[s];
                  const active = values.source === s;
                  return (
                    <button key={s} type="button" onClick={() => switchSource(s)} className={`flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition ${active ? 'border-blue-500 bg-blue-500/10 text-blue-200' : 'border-border text-muted-foreground hover:bg-accent'}`}>
                      <Icon className="size-3.5" />
                      {t(`automations.sources.${s}`)}
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          {values.source === 'jira' && !hasJira && <p className="rounded-md border border-dashed bg-muted/40 px-3 py-2 text-xs text-muted-foreground">{t('automations.jiraRequired')}</p>}
        </div>
      )}
    </form.Subscribe>
  );
}
