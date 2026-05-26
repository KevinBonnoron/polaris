import { useLiveQuery } from '@tanstack/react-db';
import { useForm } from '@tanstack/react-form';
import { Pencil, Plus, Trash2, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { customProvidersCollection } from '@/db';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { cn } from '@/lib/utils';
import type { CustomProvider } from '@/types';

const SWATCHES = ['#3b82f6', '#10b981', '#8b5cf6', '#f59e0b', '#ef4444', '#06b6d4', '#ec4899', '#84cc16'];

interface DraftModel {
  key: string;
  value: string;
}

interface DraftValues {
  name: string;
  color: string;
  endpoint: string;
  apiKey: string;
  apiType: string;
  models: DraftModel[];
}

let nextModelKey = 0;
const makeModelKey = () => `m${++nextModelKey}`;

const emptyDraft = (): DraftValues => ({
  name: '',
  color: SWATCHES[0],
  endpoint: '',
  apiKey: '',
  apiType: 'OpenAI-compatible',
  models: [],
});

interface EditingState {
  id: string | null;
  initial: DraftValues;
}

export function CustomProvidersSection() {
  const { t } = useTranslation();
  const { data: providers = [] } = useLiveQuery((q) => q.from({ p: customProvidersCollection }));
  const [editing, setEditing] = useState<EditingState | null>(null);

  const startCreate = () => setEditing({ id: null, initial: emptyDraft() });
  const startEdit = (p: CustomProvider) =>
    setEditing({
      id: p.id,
      initial: {
        name: p.name,
        color: p.color,
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
          <ProviderRow key={p.id} provider={p} onEdit={() => startEdit(p)} onRemove={() => remove(p.id)} />
        ))}
        {editing !== null && <ProviderForm key={editing.id ?? 'new'} editingId={editing.id} initial={editing.initial} onDone={cancel} />}
      </div>
    </div>
  );
}

interface ProviderRowProps {
  provider: CustomProvider;
  onEdit: () => void;
  onRemove: () => void;
}

function ProviderRow({ provider, onEdit, onRemove }: ProviderRowProps) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardContent className="flex items-center gap-3 py-2.5">
        <div className="size-8 shrink-0 rounded-md" style={{ background: provider.color }} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium">{provider.name}</div>
          <div className="truncate font-mono text-[10px] text-muted-foreground">{provider.endpoint || t('settings.customProvider.noEndpoint')}</div>
        </div>
        <div className="text-[10px] text-muted-foreground">{t('settings.customProvider.modelCount', { count: provider.models.length })}</div>
        <Button type="button" variant="ghost" size="icon" className="size-7" onClick={onEdit}>
          <Pencil className="size-3.5" />
        </Button>
        <Button type="button" variant="ghost" size="icon" className="size-7 text-destructive" onClick={onRemove}>
          <Trash2 className="size-3.5" />
        </Button>
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

  const form = useForm({
    defaultValues: initial,
    onSubmit: ({ value }) => {
      const payload = {
        name: value.name.trim(),
        color: value.color,
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
          <div className="grid grid-cols-2 gap-3">
            <form.Field name="name" validators={{ onChange: validators.required(), onBlur: validators.required() }}>
              {(field) => (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.name')}</Label>
                  <Input value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('settings.customProvider.namePlaceholder')} aria-invalid={isInvalid(field)} />
                  <FieldError field={field} />
                </div>
              )}
            </form.Field>

            <form.Field name="color">
              {(field) => (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.color')}</Label>
                  <div className="flex flex-wrap gap-1.5">
                    {SWATCHES.map((c) => (
                      <button key={c} type="button" onClick={() => field.handleChange(c)} className={cn('size-6 rounded-md border-2 transition', field.state.value === c ? 'border-foreground' : 'border-transparent')} style={{ background: c }} aria-label={c} />
                    ))}
                  </div>
                </div>
              )}
            </form.Field>
          </div>

          <form.Field name="endpoint" validators={{ onBlur: validators.url() }}>
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

          <form.Field name="models" mode="array">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label className="text-[11px] text-muted-foreground">{t('settings.customProvider.models')}</Label>
                {field.state.value.map((m, index) => (
                  <form.Field key={m.key} name={`models[${index}].value`}>
                    {(modelField) => (
                      <div className="flex items-center gap-1.5">
                        <Input value={modelField.state.value} onChange={(e) => modelField.handleChange(e.target.value)} onBlur={modelField.handleBlur} placeholder={t('settings.customProvider.modelPlaceholder')} className="font-mono text-xs" />
                        <Button type="button" variant="ghost" size="icon" className="size-7 shrink-0" onClick={() => field.removeValue(index)}>
                          <X className="size-3.5" />
                        </Button>
                      </div>
                    )}
                  </form.Field>
                ))}
                <Button type="button" variant="outline" size="sm" className="h-8 gap-1 text-xs" onClick={() => field.pushValue({ key: makeModelKey(), value: '' })}>
                  <Plus className="size-3" />
                  {t('settings.customProvider.addModel')}
                </Button>
              </div>
            )}
          </form.Field>

          <div className="flex justify-end gap-2 pt-1">
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
