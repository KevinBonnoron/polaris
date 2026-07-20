import type { AnyFieldApi } from '@tanstack/react-form';
import { useForm } from '@tanstack/react-form';
import { Check, ChevronsUpDown, Loader2, Plus, X } from 'lucide-react';
import { type PropsWithChildren, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { projectsCollection } from '@/collections/projects.collection';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { cn } from '@/lib/utils';
import { useProjects } from '@/state/projects';
import type { Project } from '@/types';
import { DetectGitRemote, DetectProviderToken, ListTicketsFields, OpenExternalURL } from '@/wailsjs/go/main/App';
import { tickets } from '@/wailsjs/go/models';
import { findIntegration, findIntegrationForStorageKey, type Integration, type IntegrationField, type IntegrationFieldOption } from './integration-catalog';
import { effectiveStorageKey, getInstance, getInstances, isIntegrationConnected, withInstanceRemoved, withInstanceUpdate, withIntegration, withoutIntegration } from './project-integrations';

interface Props extends DialogModeProps {
  projectId: string;
  integrationId: string;
  instanceIndex?: number;
}

export function ConfigureIntegrationModal({ projectId, integrationId, instanceIndex, children, ...modeProps }: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const { projects } = useProjects();
  const project = useMemo(() => projects.find((p) => p.id === projectId) ?? null, [projects, projectId]);
  const integration = findIntegration(integrationId) ?? findIntegrationForStorageKey(integrationId, project?.integrations?.[integrationId] as Record<string, unknown> | undefined);

  const close = () => setOpen(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,560px)] sm:max-w-[560px]">
        {!project || !integration ? (
          <>
            <DialogHeader>
              <DialogTitle>{t('integrations.configure.notFound')}</DialogTitle>
              <DialogDescription>{t('integrations.configure.notFoundDesc')}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button type="button" onClick={close}>
                {t('common.close')}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <Body project={project} integration={integration} onClose={close} connected={isIntegrationConnected(project, integration)} instanceIndex={instanceIndex ?? 0} />
        )}
      </DialogContent>
    </Dialog>
  );
}

interface BodyProps {
  project: Project;
  integration: Integration;
  connected: boolean;
  onClose: () => void;
  instanceIndex: number;
}

function fieldValidator(field: IntegrationField) {
  const v: Array<(arg: { value: string }) => string | undefined> = [];
  if (field.required) {
    v.push(validators.required());
  }
  if (field.type === 'url') {
    v.push(validators.url());
  }
  if (v.length === 0) {
    return undefined;
  }
  return validators.combine(...v);
}

function Body({ project, integration, connected, onClose, instanceIndex }: BodyProps) {
  const { t } = useTranslation();
  const isMulti = !!integration.multi;
  const storageKey = effectiveStorageKey(integration);
  const initial = useMemo(() => {
    const stored = (isMulti ? getInstance(project, storageKey, instanceIndex) : undefined) ?? ((getInstance(project, storageKey, 0) ?? {}) as Record<string, unknown>);
    const seed: Record<string, string> = {};
    for (const f of integration.fields) {
      const raw = stored[f.key];
      if (raw != null && raw !== '') {
        seed[f.key] = String(raw);
      } else if (f.defaultValue) {
        seed[f.key] = f.defaultValue;
      } else {
        seed[f.key] = '';
      }
    }
    return seed;
  }, [project, integration, isMulti, instanceIndex, storageKey]);

  const [removing, setRemoving] = useState(false);
  const [detectedHint, setDetectedHint] = useState<string | null>(null);
  const [tokenHint, setTokenHint] = useState<string | null>(null);

  const storedCustomFields = useMemo(() => {
    if (integration.id !== 'jira') return [];
    const stored = (isMulti ? getInstance(project, storageKey, instanceIndex) : undefined) ?? getInstance(project, storageKey, 0) ?? {};
    const raw = (stored as Record<string, unknown>).customFields;
    return Array.isArray(raw) ? (raw as Array<{ id: string; label: string }>) : [];
  }, [integration.id, isMulti, project, storageKey, instanceIndex]);

  const customFieldsRef = useRef<Array<{ id: string; label: string }>>(storedCustomFields);

  const form = useForm({
    defaultValues: initial,
    onSubmit: async ({ value }) => {
      try {
        const config: Record<string, unknown> = { ...integration.fixedValues };
        for (const f of integration.fields) {
          const v = value[f.key]?.trim();
          if (v) {
            config[f.key] = v;
          }
        }
        if (integration.id === 'jira' && customFieldsRef.current.length > 0) {
          config.customFields = customFieldsRef.current;
        }
        const next = isMulti ? withInstanceUpdate(project, storageKey, instanceIndex, config) : withIntegration(project, storageKey, config);
        const tx = projectsCollection.update(project.id, (draft) => {
          draft.integrations = next;
        });
        await tx.isPersisted.promise;
        toast.success(connected ? t('integrations.configure.updated', { name: integration.name }) : t('integrations.configure.connected', { name: integration.name }));
        onClose();
      } catch (err) {
        toastError({ title: t('integrations.configure.couldNotSave', { name: integration.name }), err });
      }
    },
  });

  const provider = integration.fixedValues?.provider;

  useEffect(() => {
    if (storageKey !== 'repository') {
      return;
    }
    const currentProvider = provider ?? form.getFieldValue('provider') ?? '';
    if (currentProvider !== 'github' && currentProvider !== 'gitlab') {
      return;
    }
    if (form.getFieldValue('token')?.trim()) {
      return;
    }
    let cancelled = false;
    DetectProviderToken(currentProvider)
      .then((tok) => {
        if (cancelled || !tok?.token) {
          return;
        }
        if (!form.getFieldValue('token')?.trim()) {
          form.setFieldValue('token', tok.token);
          setTokenHint(tok.source);
        }
      })
      .catch(() => {
        /* silent: token discovery is best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [storageKey, provider, form]);

  useEffect(() => {
    if (storageKey !== 'repository' || !project.path) {
      return;
    }
    let cancelled = false;
    DetectGitRemote(project.path)
      .then((remote) => {
        if (cancelled || !remote || !remote.url) {
          return;
        }
        const fill = (key: string, val: string) => {
          if (val && !form.getFieldValue(key)?.trim()) {
            form.setFieldValue(key, val);
          }
        };
        fill('owner', remote.owner ?? '');
        fill('repo', remote.repo ?? '');
        if (remote.baseUrl && remote.host && remote.host !== 'github.com') {
          fill('baseUrl', remote.baseUrl);
        }
        setDetectedHint(remote.url);
      })
      .catch(() => {
        /* silent: detection is best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [storageKey, project.path, form]);

  const Icon = integration.icon;

  const disconnect = async () => {
    setRemoving(true);
    try {
      const instances = getInstances(project, storageKey);
      const next = isMulti && instances.length > 1 ? withInstanceRemoved(project, storageKey, instanceIndex) : withoutIntegration(project, storageKey);
      const tx = projectsCollection.update(project.id, (draft) => {
        draft.integrations = next;
      });
      await tx.isPersisted.promise;
      toast.success(t('integrations.configure.disconnected', { name: integration.name }));
      onClose();
    } catch (err) {
      toastError({ title: t('integrations.configure.couldNotDisconnect', { name: integration.name }), err });
      setRemoving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-3">
          <div className={cn('flex size-10 shrink-0 items-center justify-center rounded-md', integration.tint)}>
            <Icon className="size-5" />
          </div>
          <div className="flex min-w-0 flex-col">
            <DialogTitle className="truncate">{connected ? t('integrations.configure.titleConfigure', { name: integration.name }) : t('integrations.configure.titleConnect', { name: integration.name })}</DialogTitle>
            <DialogDescription className="truncate">{project.name}</DialogDescription>
          </div>
        </div>
      </DialogHeader>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          void form.handleSubmit();
        }}
        className="flex flex-col gap-4"
      >
        <ScrollArea className="max-h-[70vh] -mr-6 pr-6">
          <div className="flex flex-col gap-4 pb-1">
            {detectedHint && (
              <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                {t('integrations.configure.detectedFrom')} <span className="font-mono">{detectedHint}</span>
              </p>
            )}
            {tokenHint && (
              <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                {t('integrations.configure.tokenFrom')} <span className="font-mono">{tokenHint}</span>
              </p>
            )}
            {integration.fields.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('integrations.configure.noConfig')}</p>
            ) : (
              integration.fields.map((f) => {
                const validate = fieldValidator(f);
                return (
                  <form.Field key={f.key} name={f.key} validators={validate ? { onChange: validate, onBlur: validate } : undefined}>
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <div className="flex items-center justify-between">
                          <Label htmlFor={`int-${integration.id}-${f.key}`} className="text-xs text-muted-foreground">
                            {f.label}
                            {f.required && <span className="ml-1 text-destructive">*</span>}
                          </Label>
                          {f.helpUrl && (
                            <button type="button" className="text-xs text-primary hover:underline" onClick={() => OpenExternalURL(f.helpUrl!)}>
                              {f.help}
                            </button>
                          )}
                        </div>
                        {f.type === 'select' ? (
                          <Select value={field.state.value ?? ''} onValueChange={field.handleChange}>
                            <SelectTrigger id={`int-${integration.id}-${f.key}`} aria-invalid={isInvalid(field)}>
                              <SelectValue placeholder={f.placeholder ?? t('integrations.configure.selectPlaceholder')} />
                            </SelectTrigger>
                            <SelectContent>
                              {f.options?.map((opt) => (
                                <SelectItem key={opt.value} value={opt.value}>
                                  {opt.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : f.type === 'autocomplete' ? (
                          <AutocompleteInput fieldDef={f} field={field} integrationId={integration.id} formValues={initial} />
                        ) : (
                          <Input
                            id={`int-${integration.id}-${f.key}`}
                            type={f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'}
                            inputMode={f.type === 'number' ? 'numeric' : undefined}
                            value={field.state.value ?? ''}
                            onChange={(e) => field.handleChange(e.target.value)}
                            onBlur={field.handleBlur}
                            placeholder={f.placeholder}
                            autoComplete="off"
                            aria-invalid={isInvalid(field)}
                          />
                        )}
                        <FieldError field={field} />
                        {f.help && !f.helpUrl && <p className="text-xs text-muted-foreground">{f.help}</p>}
                      </div>
                    )}
                  </form.Field>
                );
              })
            )}

            {integration.id === 'jira' && <JiraCustomFieldsSection formValues={initial} form={form} customFieldsRef={customFieldsRef} initialFields={storedCustomFields} />}
          </div>
        </ScrollArea>

        <DialogFooter className="justify-between gap-2 sm:justify-between">
          <div>
            {connected && (
              <Button type="button" variant="ghost" className="text-destructive hover:text-destructive" onClick={disconnect} disabled={removing || form.state.isSubmitting}>
                {removing ? t('integrations.configure.disconnecting') : t('integrations.configure.disconnect')}
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={form.state.isSubmitting || removing}>
              {t('common.cancel')}
            </Button>
            <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
              {([canSubmit, isSubmitting]) => (
                <Button type="submit" disabled={!canSubmit || isSubmitting || removing}>
                  {isSubmitting ? t('integrations.configure.saving') : connected ? t('integrations.configure.save') : t('integrations.configure.connect')}
                </Button>
              )}
            </form.Subscribe>
          </div>
        </DialogFooter>
      </form>
    </>
  );
}

interface AutocompleteInputProps {
  fieldDef: IntegrationField;
  field: AnyFieldApi;
  integrationId: string;
  formValues: Record<string, string>;
}

function splitCsv(raw: string): string[] {
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

function AutocompleteInput({ fieldDef, field, integrationId, formValues }: AutocompleteInputProps) {
  const { t } = useTranslation();
  const [options, setOptions] = useState<IntegrationFieldOption[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const loadedRef = useRef(false);

  const loadSuggestions = useCallback(
    async (force: boolean) => {
      if (!fieldDef.loadOptions || (loadedRef.current && !force)) {
        return;
      }
      loadedRef.current = true;
      setLoading(true);
      try {
        const values = (field.form.state.values as Record<string, string>) ?? formValues;
        setOptions(await fieldDef.loadOptions(values));
      } catch {
        setOptions([]);
      } finally {
        setLoading(false);
      }
    },
    [fieldDef, field, formValues],
  );

  useEffect(() => {
    void loadSuggestions(false);
  }, [loadSuggestions]);

  const currentValue = field.state.value ?? '';
  const selected = fieldDef.multiple ? splitCsv(currentValue) : currentValue ? [currentValue] : [];
  const isSelected = (val: string) => selected.includes(val);

  const toggle = (val: string) => {
    if (fieldDef.multiple) {
      const next = isSelected(val) ? selected.filter((v) => v !== val) : [...selected, val];
      field.handleChange(next.join(', '));
      return;
    }
    field.handleChange(val === currentValue ? '' : val);
    setOpen(false);
  };

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o);
        if (o) {
          void loadSuggestions(true);
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button id={`int-${integrationId}-${fieldDef.key}`} variant="outline" className="h-auto min-h-9 w-full justify-between font-normal" aria-invalid={isInvalid(field)}>
          {selected.length > 0 ? (
            fieldDef.multiple ? (
              <span className="flex flex-wrap gap-1">
                {selected.map((s) => (
                  <Badge key={s} variant="secondary">
                    {s}
                  </Badge>
                ))}
              </span>
            ) : (
              currentValue
            )
          ) : (
            <span className="text-muted-foreground">{fieldDef.placeholder}</span>
          )}
          <ChevronsUpDown className="ml-2 size-3.5 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start" sideOffset={4} portal={false}>
        <Command>
          <CommandList>
            <CommandEmpty className="py-3">{loading ? t('integrations.configure.loadingOptions') : t('integrations.configure.noOptions')}</CommandEmpty>
            <CommandGroup>
              {options.map((opt) => (
                <CommandItem key={opt.value} value={opt.value} onSelect={() => toggle(opt.value)}>
                  <Check className={cn('mr-2 size-3.5', isSelected(opt.value) ? 'opacity-100' : 'opacity-0')} />
                  {opt.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

interface JiraCustomFieldsSectionProps {
  formValues: Record<string, string>;
  form: { getFieldValue: (key: string) => string | undefined };
  customFieldsRef: React.RefObject<Array<{ id: string; label: string }>>;
  initialFields: Array<{ id: string; label: string }>;
}

function JiraCustomFieldsSection({ formValues, form, customFieldsRef, initialFields }: JiraCustomFieldsSectionProps) {
  const { t } = useTranslation();
  const [fields, setFields] = useState<Array<{ id: string; label: string }>>(initialFields);
  const [available, setAvailable] = useState<tickets.JiraField[]>([]);
  const [loadingFields, setLoadingFields] = useState(false);
  const [showPicker, setShowPicker] = useState(false);
  const [search, setSearch] = useState('');

  const update = (next: Array<{ id: string; label: string }>) => {
    setFields(next);
    customFieldsRef.current = next;
  };

  const loadAndShow = async () => {
    if (showPicker) {
      setShowPicker(false);
      return;
    }
    if (available.length > 0) {
      setShowPicker(true);
      return;
    }
    const baseUrl = form.getFieldValue('baseUrl') ?? formValues.baseUrl ?? '';
    const email = form.getFieldValue('email') ?? formValues.email ?? '';
    const token = form.getFieldValue('token') ?? formValues.token ?? '';
    if (!baseUrl || !email || !token) return;
    setLoadingFields(true);
    try {
      const result = await ListTicketsFields(tickets.Config.createFrom({ provider: 'jira', baseUrl, email, token }));
      setAvailable(result ?? []);
      setShowPicker(true);
    } catch (err) {
      toastError({ title: t('integrations.jira.loadFieldsFailed'), err });
    } finally {
      setLoadingFields(false);
    }
  };

  const toggle = (field: tickets.JiraField) => {
    const exists = fields.some((f) => f.id === field.id);
    update(exists ? fields.filter((f) => f.id !== field.id) : [...fields, { id: field.id, label: field.name }]);
  };

  const filtered = search ? available.filter((f) => f.name.toLowerCase().includes(search.toLowerCase()) || f.id.toLowerCase().includes(search.toLowerCase())) : available;

  return (
    <div className="flex flex-col gap-2 border-t pt-4">
      <div className="flex items-center justify-between">
        <Label className="cursor-pointer text-xs text-muted-foreground" onClick={loadAndShow}>
          {t('integrations.jira.customFields')}
        </Label>
        <Button type="button" size="sm" variant="outline" className="h-7 gap-1 px-2 text-xs" onClick={loadAndShow} disabled={loadingFields}>
          {loadingFields ? <Loader2 className="size-3 animate-spin" /> : <Plus className="size-3" />}
          {t('integrations.jira.addField')}
        </Button>
      </div>

      {showPicker && (
        <div className="flex flex-col rounded-md border">
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('integrations.jira.searchFields')} className="rounded-b-none border-0 border-b text-xs focus-visible:ring-0" autoFocus />
          <ScrollArea className="h-48 pr-1.5">
            <div className="flex flex-col">
              {filtered.length === 0 ? (
                <p className="px-3 py-2 text-xs text-muted-foreground">{t('common.noResults')}</p>
              ) : (
                filtered.map((f) => {
                  const selected = fields.some((cf) => cf.id === f.id);
                  return (
                    <button
                      key={f.id}
                      type="button"
                      className="flex w-full items-center gap-2 px-3 py-1.5 text-xs hover:bg-accent"
                      onClick={() => {
                        toggle(f);
                        setShowPicker(false);
                        setSearch('');
                      }}
                    >
                      <Check className={cn('size-3 shrink-0', selected ? 'opacity-100 text-primary' : 'opacity-0')} />
                      <span className="flex-1 truncate text-left">{f.name}</span>
                      <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{f.id}</span>
                    </button>
                  );
                })
              )}
            </div>
          </ScrollArea>
        </div>
      )}

      {fields.length === 0 && !showPicker ? (
        <p className="text-xs text-muted-foreground">{t('integrations.jira.noCustomFields')}</p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {fields.map((f, i) => (
            <div key={f.id} className="flex items-center gap-1.5">
              <span className="w-32 shrink-0 truncate font-mono text-[11px] text-muted-foreground" title={f.id}>
                {f.id}
              </span>
              <Input
                value={f.label}
                onChange={(e) => {
                  const next = [...fields];
                  next[i] = { ...f, label: e.target.value };
                  update(next);
                }}
                className="h-7 flex-1 text-xs"
                placeholder={t('integrations.jira.fieldLabelPlaceholder')}
              />
              <Button type="button" size="sm" variant="ghost" className="h-7 w-7 shrink-0 p-0" onClick={() => update(fields.filter((_, j) => j !== i))}>
                <X className="size-3" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
