import { useLiveQuery } from '@tanstack/react-db';
import { useForm } from '@tanstack/react-form';
import { CheckCircle2, CircleAlert, Loader2, Pencil, Plus, Trash2, Wifi, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { customProvidersCollection } from '@/collections/custom-providers.collection';
import { PROVIDER_ICON_REGISTRY, resolveProviderIcon } from '@/components/brand-icons';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { AgentModelSelect } from '@/features/agents/agent-model-select';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { cn } from '@/lib/utils';
import { useAgentDefaults } from '@/state/agent-defaults';
import type { CustomProvider } from '@/types';
import { TestCustomProvider } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';

type Health = { state: 'testing' } | { state: 'done'; result: polaris.ProviderHealth };

function describeHealth(result: polaris.ProviderHealth, t: (k: string, o?: Record<string, unknown>) => string): { ok: boolean; text: string; sub?: string } {
  const map: Record<string, string> = {
    missing_endpoint: 'settings.customProvider.healthMissingEndpoint',
    invalid_url: 'settings.customProvider.healthInvalidUrl',
    unreachable: 'settings.customProvider.healthUnreachable',
    timeout: 'settings.customProvider.healthTimeout',
    auth_failed: 'settings.customProvider.healthAuthFailed',
    http_error: 'settings.customProvider.healthHttpError',
  };
  if (result.ok) {
    const parts = [t('settings.customProvider.healthOk', { latency: result.latencyMs })];
    if (result.toolModelsKnown) {
      parts.push(t('settings.customProvider.healthToolModels', { count: result.toolModels?.length ?? 0 }));
    } else if (result.discoveredModels?.length) {
      parts.push(t('settings.customProvider.healthModels', { count: result.discoveredModels.length }));
    }
    const sub = result.unknownModels?.length ? t('settings.customProvider.healthUnknownModels', { models: result.unknownModels.join(', ') }) : undefined;
    return { ok: true, text: parts.join(' · '), sub };
  }
  const key = map[result.code] ?? 'settings.customProvider.healthUnreachable';
  return { ok: false, text: t(key, { status: result.status }) };
}

function HealthLine({ health }: { health: Health }) {
  const { t } = useTranslation();
  if (health.state === 'testing') {
    return (
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" />
        {t('settings.customProvider.testing')}
      </p>
    );
  }
  const { ok, text, sub } = describeHealth(health.result, t);
  return (
    <div className="flex flex-col gap-0.5">
      <p className={cn('flex items-center gap-1.5 text-xs', ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive')}>
        {ok ? <CheckCircle2 className="size-3.5" /> : <CircleAlert className="size-3.5" />}
        {text}
      </p>
      {sub && <p className="pl-5 text-[11px] text-amber-600 dark:text-amber-400">{sub}</p>}
    </div>
  );
}

interface DraftModel {
  key: string;
  value: string;
}

interface DraftValues {
  name: string;
  icon: string;
  endpoint: string;
  apiKey: string;
  apiType: string;
  models: DraftModel[];
}

let nextModelKey = 0;
const makeModelKey = () => `m${++nextModelKey}`;

const emptyDraft = (): DraftValues => ({
  name: '',
  icon: '',
  endpoint: '',
  apiKey: '',
  apiType: 'OpenAI-compatible',
  models: [],
});

interface EditingState {
  id: string | null;
  initial: DraftValues;
}

async function runHealthCheck(provider: CustomProvider): Promise<polaris.ProviderHealth> {
  return TestCustomProvider(provider as never);
}

export function CustomProvidersSection() {
  const { t } = useTranslation();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const [editing, setEditing] = useState<EditingState | null>(null);
  const [rowHealth, setRowHealth] = useState<Record<string, Health>>({});

  const testRow = async (provider: CustomProvider) => {
    setRowHealth((prev) => ({ ...prev, [provider.id]: { state: 'testing' } }));
    const result = await runHealthCheck(provider);
    setRowHealth((prev) => ({ ...prev, [provider.id]: { state: 'done', result } }));
  };

  const startCreate = () => setEditing({ id: null, initial: emptyDraft() });
  const startEdit = (p: CustomProvider) =>
    setEditing({
      id: p.id,
      initial: {
        name: p.name,
        icon: p.icon ?? '',
        endpoint: p.endpoint,
        apiKey: p.apiKey,
        apiType: p.apiType,
        models: p.models.map((value) => ({ key: makeModelKey(), value })),
      },
    });
  const cancel = () => setEditing(null);

  const remove = (id: string) => {
    customProvidersCollection.delete(id);
    if (editing?.id === id) {
      setEditing(null);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between">
        <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.customProvider.section')}</h4>
        {editing === null && (
          <Button type="button" variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={startCreate}>
            <Plus className="size-3" />
            {t('settings.customProvider.add')}
          </Button>
        )}
      </div>

      {providers.length === 0 && editing === null && <p className="text-xs text-muted-foreground">{t('settings.customProvider.empty')}</p>}

      <div className="flex flex-col gap-2">
        {providers.map((p) => (
          <ProviderRow key={p.id} provider={p} health={rowHealth[p.id]} onTest={() => testRow(p)} onEdit={() => startEdit(p)} onRemove={() => remove(p.id)} />
        ))}
        {editing !== null && <ProviderForm key={editing.id ?? 'new'} editingId={editing.id} initial={editing.initial} onDone={cancel} />}
      </div>
    </div>
  );
}

interface ProviderRowProps {
  provider: CustomProvider;
  health?: Health;
  onTest: () => void;
  onEdit: () => void;
  onRemove: () => void;
}

function ProviderRow({ provider, health, onTest, onEdit, onRemove }: ProviderRowProps) {
  const { t } = useTranslation();
  const defaults = useAgentDefaults();
  const models = provider.models.map((m) => ({ value: m, label: m }));
  const ProviderIcon = resolveProviderIcon(provider.icon);
  return (
    <Card>
      <CardContent className="flex flex-col gap-2 py-2.5">
        <div className="flex items-center gap-3">
          {ProviderIcon && (
            <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <ProviderIcon className="size-3.5" />
            </div>
          )}
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium">{provider.name}</div>
            <div className="truncate font-mono text-[10px] text-muted-foreground">{provider.endpoint || t('settings.customProvider.noEndpoint')}</div>
          </div>
          {models.length > 0 && <AgentModelSelect models={models} value={defaults.get(provider.id) ?? models[0]?.value ?? ''} onChange={(model) => defaults.set(provider.id, model)} triggerClassName="w-fit min-w-[140px] max-w-[240px] shrink-0" />}
          <Button type="button" variant="ghost" size="icon" className="size-7" onClick={onTest} disabled={health?.state === 'testing'} title={t('settings.customProvider.test')}>
            <Wifi className="size-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" className="size-7" onClick={onEdit}>
            <Pencil className="size-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" className="size-7 text-destructive" onClick={onRemove}>
            <Trash2 className="size-3.5" />
          </Button>
        </div>
        {health && <HealthLine health={health} />}
      </CardContent>
    </Card>
  );
}

interface ProviderFormProps {
  editingId: string | null;
  initial: DraftValues;
  onDone: () => void;
}

function ProviderForm({ editingId, initial, onDone }: ProviderFormProps) {
  const { t } = useTranslation();
  const [health, setHealth] = useState<Health | null>(null);
  const [modelDraft, setModelDraft] = useState('');

  const test = async () => {
    const v = form.state.values;
    setHealth({ state: 'testing' });
    const result = await runHealthCheck({
      id: editingId ?? '',
      name: v.name.trim(),
      color: '',
      endpoint: v.endpoint.trim(),
      apiKey: v.apiKey,
      apiType: v.apiType,
      models: v.models.map((m) => m.value.trim()).filter(Boolean),
    });
    setHealth({ state: 'done', result });
    if (result.ok && result.resolvedEndpoint && result.resolvedEndpoint !== v.endpoint.trim()) {
      form.setFieldValue('endpoint', result.resolvedEndpoint);
    }
  };

  // Suggest only tool-capable models when the provider exposes capabilities
  // (Ollama), since the opencode harness requires tool calling. Otherwise we
  // can't tell, so offer every discovered model.
  const discovered = health?.state === 'done' && health.result.ok ? (health.result.toolModelsKnown ? (health.result.toolModels ?? []) : (health.result.discoveredModels ?? [])) : [];

  const form = useForm({
    defaultValues: initial,
    onSubmit: ({ value }) => {
      const payload = {
        name: value.name.trim(),
        color: '',
        icon: value.icon || undefined,
        endpoint: value.endpoint.trim(),
        apiKey: value.apiKey,
        apiType: value.apiType,
        models: value.models.map((m) => m.value.trim()).filter(Boolean),
      };
      if (editingId) {
        customProvidersCollection.update(editingId, (d) => {
          Object.assign(d, payload);
        });
      } else {
        customProvidersCollection.insert({ id: '', ...payload });
      }
      onDone();
    },
  });

  return (
    <Card className="border-primary/40">
      <CardContent className="flex flex-col gap-3 py-3">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
          className="flex flex-col gap-3"
        >
          <form.Field name="name" validators={{ onChange: validators.required(), onBlur: validators.required() }}>
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.name')}</Label>
                <Input value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('settings.customProvider.namePlaceholder')} aria-invalid={isInvalid(field)} />
                <FieldError field={field} />
              </div>
            )}
          </form.Field>

          <form.Field name="icon">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.icon')}</Label>
                <div className="flex flex-wrap gap-1">
                  {PROVIDER_ICON_REGISTRY.map(({ key, label, icon: Icon }) => (
                    <Tooltip key={key}>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => field.handleChange(field.state.value === key ? '' : key)}
                          className={cn('flex size-7 items-center justify-center rounded-md border transition-colors', field.state.value === key ? 'border-primary bg-primary/10 text-primary' : 'border-input bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground')}
                        >
                          <Icon className="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" className="text-xs">
                        {label}
                      </TooltipContent>
                    </Tooltip>
                  ))}
                </div>
              </div>
            )}
          </form.Field>

          <form.Field name="endpoint" validators={{ onChange: validators.required(), onBlur: validators.combine(validators.required(), validators.url()) }}>
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.endpoint')}</Label>
                <Input value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('settings.customProvider.endpointPlaceholder')} className="font-mono text-xs" aria-invalid={isInvalid(field)} />
                <FieldError field={field} />
              </div>
            )}
          </form.Field>

          <div className="grid grid-cols-2 gap-3">
            <form.Field name="apiKey">
              {(field) => (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.apiKey')}</Label>
                  <Input type="password" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('settings.customProvider.apiKeyPlaceholder')} />
                </div>
              )}
            </form.Field>

            <form.Field name="apiType">
              {(field) => (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.apiType')}</Label>
                  <Select value={field.state.value} onValueChange={field.handleChange}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="OpenAI-compatible">{t('settings.customProvider.apiTypeOpenai')}</SelectItem>
                      <SelectItem value="Anthropic-compatible">{t('settings.customProvider.apiTypeAnthropic')}</SelectItem>
                      <SelectItem value="Custom (raw HTTP)">{t('settings.customProvider.apiTypeRaw')}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
            </form.Field>
          </div>

          <form.Field name="models" mode="array" validators={{ onChange: ({ value }: { value: DraftModel[] }) => (value.some((m) => m.value.trim()) ? undefined : t('settings.customProvider.noModels')) }}>
            {(field) => {
              const has = (v: string) => field.state.value.some((m) => m.value.trim() === v);
              const add = (raw: string) => {
                const v = raw.trim();
                if (v && !has(v)) {
                  field.pushValue({ key: makeModelKey(), value: v });
                }
                setModelDraft('');
              };
              const suggestions = discovered.filter((id) => !has(id));
              return (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.models')}</Label>
                  <div className="flex flex-wrap items-center gap-1.5 rounded-md border border-input px-2 py-1.5 focus-within:ring-1 focus-within:ring-ring">
                    {field.state.value.map((m, index) => (
                      <span key={m.key} className="flex items-center gap-1 rounded bg-secondary px-1.5 py-0.5 font-mono text-[11px]">
                        {m.value}
                        <button type="button" className="text-muted-foreground hover:text-foreground" onClick={() => field.removeValue(index)}>
                          <X className="size-3" />
                        </button>
                      </span>
                    ))}
                    <input
                      value={modelDraft}
                      onChange={(e) => setModelDraft(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ',') {
                          e.preventDefault();
                          add(modelDraft);
                        } else if (e.key === 'Backspace' && !modelDraft && field.state.value.length > 0) {
                          field.removeValue(field.state.value.length - 1);
                        }
                      }}
                      onBlur={() => add(modelDraft)}
                      placeholder={field.state.value.length === 0 ? t('settings.customProvider.modelTagPlaceholder') : ''}
                      className="min-w-[8rem] flex-1 bg-transparent font-mono text-xs outline-none placeholder:text-muted-foreground"
                    />
                  </div>
                  {suggestions.length > 0 && (
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="text-[11px] text-muted-foreground">{t('settings.customProvider.discovered')}</span>
                      {suggestions.map((id) => (
                        <button key={id} type="button" onClick={() => add(id)} className="flex items-center gap-1 rounded-md border border-input px-2 py-0.5 font-mono text-[11px] text-muted-foreground transition hover:bg-accent hover:text-foreground">
                          <Plus className="size-3" />
                          {id}
                        </button>
                      ))}
                    </div>
                  )}
                  <FieldError field={field} />
                </div>
              );
            }}
          </form.Field>

          {health && <HealthLine health={health} />}

          <div className="flex items-center justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" size="sm" className="mr-auto gap-1.5" onClick={test} disabled={health?.state === 'testing'}>
              <Wifi className="size-3.5" />
              {t('settings.customProvider.test')}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={onDone}>
              {t('settings.customProvider.cancel')}
            </Button>
            <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
              {([canSubmit, isSubmitting]) => (
                <Button type="submit" size="sm" disabled={!canSubmit || isSubmitting}>
                  {editingId ? t('settings.customProvider.save') : t('settings.customProvider.addProvider')}
                </Button>
              )}
            </form.Subscribe>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
