import type { AnyFieldApi } from '@tanstack/react-form';
import { useForm } from '@tanstack/react-form';
import { Check, ChevronsUpDown } from 'lucide-react';
import { type PropsWithChildren, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { projectsCollection } from '@/collections/projects.collection';
import { Button } from '@/components/ui/button';
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { cn } from '@/lib/utils';
import { useProjects } from '@/state/projects';
import type { Project } from '@/types';
import { DetectGitRemote, DetectProviderToken } from '@/wailsjs/go/main/App';
import { findIntegration, type Integration, type IntegrationField, type IntegrationFieldOption } from './integration-catalog';
import { getIntegrations, isConnected, withIntegration, withoutIntegration } from './project-integrations';

interface Props extends DialogModeProps {
  projectId: string;
  integrationId: string;
}

export function ConfigureIntegrationModal({ projectId, integrationId, children, ...modeProps }: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const { projects } = useProjects();
  const project = useMemo(() => projects.find((p) => p.id === projectId) ?? null, [projects, projectId]);
  const integration = findIntegration(integrationId);

  const close = () => setOpen(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,560px)] sm:max-w-[560px] gap-4">
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
          <Body project={project} integration={integration} onClose={close} connected={isConnected(project, integration.id)} />
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

function Body({ project, integration, connected, onClose }: BodyProps) {
  const { t } = useTranslation();
  const initial = useMemo(() => {
    const stored = (getIntegrations(project)[integration.id] ?? {}) as Record<string, unknown>;
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
  }, [project, integration]);

  const [removing, setRemoving] = useState(false);
  const [detectedHint, setDetectedHint] = useState<string | null>(null);
  const [tokenHint, setTokenHint] = useState<string | null>(null);

  const form = useForm({
    defaultValues: initial,
    onSubmit: async ({ value }) => {
      try {
        const config: Record<string, unknown> = {};
        for (const f of integration.fields) {
          const v = value[f.key]?.trim();
          if (v) {
            config[f.key] = v;
          }
        }
        const next = withIntegration(project, integration.id, config);
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

  useEffect(() => {
    if (integration.id !== 'repository') {
      return;
    }
    const currentProvider = form.getFieldValue('provider') ?? '';
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
  }, [integration.id, form]);

  useEffect(() => {
    if (integration.id !== 'repository' || !project.path) {
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
        if (remote.provider && remote.provider !== 'unknown') {
          fill('provider', remote.provider);
        }
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
  }, [integration.id, project.path, form]);

  const Icon = integration.icon;

  const disconnect = async () => {
    setRemoving(true);
    try {
      const next = withoutIntegration(project, integration.id);
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
                    <Label htmlFor={`int-${integration.id}-${f.key}`} className="text-xs text-muted-foreground">
                      {f.label}
                      {f.required && <span className="ml-1 text-destructive">*</span>}
                    </Label>
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
                    {f.help && <p className="text-xs text-muted-foreground">{f.help}</p>}
                  </div>
                )}
              </form.Field>
            );
          })
        )}

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

function AutocompleteInput({ fieldDef, field, integrationId, formValues }: AutocompleteInputProps) {
  const [options, setOptions] = useState<IntegrationFieldOption[]>([]);
  const [open, setOpen] = useState(false);
  const loadedRef = useRef(false);

  const loadSuggestions = useCallback(async () => {
    if (!fieldDef.loadOptions || loadedRef.current) {
      return;
    }
    loadedRef.current = true;
    const opts = await fieldDef.loadOptions(formValues);
    setOptions(opts);
  }, [fieldDef, formValues]);

  useEffect(() => {
    void loadSuggestions();
  }, [loadSuggestions]);

  const currentValue = field.state.value ?? '';

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button id={`int-${integrationId}-${fieldDef.key}`} variant="outline" className="w-full justify-between font-normal" aria-invalid={isInvalid(field)}>
          {currentValue || <span className="text-muted-foreground">{fieldDef.placeholder}</span>}
          <ChevronsUpDown className="ml-2 size-3.5 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start" sideOffset={4}>
        <Command>
          <CommandList>
            <CommandEmpty>No scripts found</CommandEmpty>
            <CommandGroup>
              {options.map((opt) => (
                <CommandItem
                  key={opt.value}
                  value={opt.value}
                  onSelect={(val) => {
                    field.handleChange(val === currentValue ? '' : val);
                    setOpen(false);
                  }}
                >
                  <Check className={cn('mr-2 size-3.5', currentValue === opt.value ? 'opacity-100' : 'opacity-0')} />
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
