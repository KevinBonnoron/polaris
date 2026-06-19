import { useForm } from '@tanstack/react-form';
import { type PropsWithChildren, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { CreateTicketsIssue, ListTicketsIssueTypes } from '@/wailsjs/go/main/App';
import { tickets } from '@/wailsjs/go/models';
import type { ConnectedTicketsConfig } from './types';

interface Props {
  config: ConnectedTicketsConfig;
  onCreated: () => void;
}

interface FormValues {
  summary: string;
  typeId: string;
}

export function CreateTicketsIssueModal({ config, onCreated, children }: PropsWithChildren<Props>) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const [types, setTypes] = useState<tickets.IssueType[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);

  const form = useForm({
    defaultValues: { summary: '', typeId: '' } as FormValues,
    onSubmit: async ({ value }) => {
      try {
        const cfg = tickets.Config.createFrom(config);
        const input = tickets.CreateIssueInput.createFrom({ summary: value.summary.trim(), issueTypeId: value.typeId });
        const key = await CreateTicketsIssue(cfg, input);
        toast.success(t('integrations.tickets.created', { key }));
        onCreated();
        setOpen(false);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : String(err));
      }
    },
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    const cfg = tickets.Config.createFrom(config);
    ListTicketsIssueTypes(cfg)
      .then((list) => {
        setTypes(list ?? []);
        const fallback = list?.find((tt) => tt.name.toLowerCase() === 'task') ?? list?.[0];
        if (fallback) {
          form.setFieldValue('typeId', fallback.id);
        }
      })
      .catch((err) => setLoadError(err instanceof Error ? err.message : String(err)));
  }, [config, form, open]);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && form.state.isSubmitting) {
          return;
        }
        setOpen(next);
      }}
    >
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="w-[min(95vw,520px)] gap-4">
        <DialogHeader>
          <DialogTitle>{t('integrations.tickets.createTitle')}</DialogTitle>
          <DialogDescription>{t('integrations.tickets.createDesc', { project: config.projectKey })}</DialogDescription>
        </DialogHeader>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
          className="flex flex-col gap-4"
        >
          <form.Field name="summary" validators={{ onChange: validators.required(), onBlur: validators.required() }}>
            {(field) => (
              <div className="flex flex-col gap-2">
                <Label htmlFor="tickets-summary">{t('integrations.tickets.summary')}</Label>
                <Input
                  id="tickets-summary"
                  autoFocus
                  value={field.state.value}
                  onChange={(e) => field.handleChange(e.target.value)}
                  onBlur={field.handleBlur}
                  placeholder={t('integrations.tickets.summaryPlaceholder')}
                  aria-invalid={isInvalid(field)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                      void form.handleSubmit();
                    }
                  }}
                />
                <FieldError field={field} />
              </div>
            )}
          </form.Field>

          <form.Field name="typeId" validators={{ onChange: validators.required('common.validation.pickOne') }}>
            {(field) => (
              <div className="flex flex-col gap-2">
                <Label>{t('integrations.tickets.issueType')}</Label>
                <Select value={field.state.value} onValueChange={field.handleChange}>
                  <SelectTrigger aria-invalid={isInvalid(field)}>
                    <SelectValue placeholder={t('integrations.configure.selectPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {types.map((tt) => (
                      <SelectItem key={tt.id} value={tt.id}>
                        {tt.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError field={field} />
                {loadError && <p className="text-xs text-destructive">{loadError}</p>}
              </div>
            )}
          </form.Field>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={form.state.isSubmitting}>
              {t('common.cancel')}
            </Button>
            <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
              {([canSubmit, isSubmitting]) => (
                <Button type="submit" disabled={!canSubmit || isSubmitting}>
                  {isSubmitting ? t('integrations.tickets.creating') : t('integrations.tickets.createCta')}
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
