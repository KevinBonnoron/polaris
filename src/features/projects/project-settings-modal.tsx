import { useForm } from '@tanstack/react-form';
import type { LucideIcon } from 'lucide-react';
import { Link as LinkIcon, Settings as SettingsIcon, Trash2 } from 'lucide-react';
import { type PropsWithChildren, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { projectsCollection } from '@/collections/projects.collection';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { IntegrationCard } from '@/features/integrations/integration-card';
import { INTEGRATIONS } from '@/features/integrations/integration-catalog';
import { isConnected, withIntegration } from '@/features/integrations/project-integrations';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { cn } from '@/lib/utils';
import { useProjects } from '@/state/projects';
import type { Project } from '@/types';
import { ProjectAvatar } from './project-avatar';
import { ProjectLogoColorFields } from './project-logo-color-fields';
import { makeUniquePathValidator } from './unique-path';

type Section = 'general' | 'integrations' | 'danger';

const NAV: { id: Section; labelKey: 'navGeneral' | 'navIntegrations' | 'navDanger'; icon: LucideIcon }[] = [
  { id: 'general', labelKey: 'navGeneral', icon: SettingsIcon },
  { id: 'integrations', labelKey: 'navIntegrations', icon: LinkIcon },
  { id: 'danger', labelKey: 'navDanger', icon: Trash2 },
];

interface FormValues {
  name: string;
  color: string;
  logo: string | undefined;
  path: string;
  isolatedDefault: boolean;
  branchPrefix: string;
}

interface Props extends DialogModeProps {
  projectId: string;
}

export function ProjectSettingsModal({ projectId, children, ...modeProps }: PropsWithChildren<Props>) {
  const { open, setOpen } = useDialogMode(modeProps);
  const { projects } = useProjects();
  const project = useMemo(() => projects.find((p) => p.id === projectId) ?? null, [projects, projectId]);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,820px)] sm:max-w-[820px] h-[min(85vh,640px)] grid-rows-[auto_1fr_auto] gap-4 p-6">{project ? <ProjectSettingsContent project={project} onClose={() => setOpen(false)} /> : <MissingProject onClose={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

function MissingProject({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('projects.settings.notFoundTitle')}</DialogTitle>
        <DialogDescription>{t('projects.settings.notFoundDesc')}</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button type="button" onClick={onClose}>
          {t('projects.settings.close')}
        </Button>
      </DialogFooter>
    </>
  );
}

interface ContentProps {
  project: Project;
  onClose: () => void;
}

function ProjectSettingsContent({ project, onClose }: ContentProps) {
  const { t } = useTranslation();
  const { projects } = useProjects();
  const [section, setSection] = useState<Section>('general');
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [redetecting, setRedetecting] = useState(false);
  const [configureId, setConfigureId] = useState<string | null>(null);
  const uniquePath = makeUniquePathValidator(projects, 'projects.settings.duplicatePath', t, project.id);

  const form = useForm({
    defaultValues: {
      name: project.name,
      color: project.color,
      logo: project.logo,
      path: project.path ?? '',
      isolatedDefault: project.isolatedDefault ?? false,
      branchPrefix: project.branchPrefix ?? 'polaris/',
    } as FormValues,
    onSubmit: async ({ value }) => {
      try {
        const tx = projectsCollection.update(project.id, (draft) => {
          draft.name = value.name.trim();
          draft.color = value.color;
          draft.logo = value.logo || undefined;
          draft.path = value.path.trim() || undefined;
          draft.isolatedDefault = value.isolatedDefault;
          draft.branchPrefix = value.branchPrefix.trim() || 'polaris/';
        });
        await tx.isPersisted.promise;
        toast.success(t('projects.settings.updated'));
      } catch (err) {
        toastError({ title: t('projects.settings.couldNotUpdate'), err });
      }
    },
  });

  useEffect(() => {
    form.reset({
      name: project.name,
      color: project.color,
      logo: project.logo,
      path: project.path ?? '',
      isolatedDefault: project.isolatedDefault ?? false,
      branchPrefix: project.branchPrefix ?? 'polaris/',
    });
  }, [project.name, project.color, project.logo, project.path, project.isolatedDefault, project.branchPrefix, form]);

  const redetect = async () => {
    const path = project.path;
    if (!path) {
      return;
    }

    setRedetecting(true);
    try {
      const results = await Promise.all(
        INTEGRATIONS.map(async (i) => {
          if (!i.detect || isConnected(project, i.id)) {
            return null;
          }
          try {
            const config = await i.detect(path);
            return config ? ({ id: i.id, config } as const) : null;
          } catch {
            return null;
          }
        }),
      );
      const added = results.filter((r): r is { id: string; config: Record<string, unknown> } => r !== null);
      if (added.length === 0) {
        toast.message(t('projects.settings.redetectedNone'));
        return;
      }

      const tx = projectsCollection.update(project.id, (draft) => {
        let next = draft.integrations ?? {};
        for (const item of added) {
          next = withIntegration({ ...draft, integrations: next }, item.id, item.config);
        }
        draft.integrations = next;
      });
      await tx.isPersisted.promise;
      const names = added.map((a) => INTEGRATIONS.find((i) => i.id === a.id)?.name ?? a.id).join(', ');
      toast.success(t('projects.settings.redetectedAdded', { count: added.length }), { description: names });
    } catch (err) {
      toastError({ title: t('projects.settings.couldNotRedetect'), err });
    } finally {
      setRedetecting(false);
    }
  };

  const remove = async () => {
    setDeleting(true);
    try {
      const tx = projectsCollection.delete(project.id);
      await tx.isPersisted.promise;
      toast.success(t('projects.settings.deleted', { name: project.name }));
      onClose();
    } catch (err) {
      toastError({ title: t('projects.settings.couldNotDelete'), err });
      setDeleting(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-3">
          <ProjectAvatar project={project} className="size-6 rounded" textClassName="text-[10px]" />
          <div className="flex min-w-0 flex-col">
            <DialogTitle className="truncate">{t('projects.settings.title')}</DialogTitle>
            <DialogDescription className="truncate">{project.name}</DialogDescription>
          </div>
        </div>
      </DialogHeader>

      <form
        id="project-settings-form"
        onSubmit={(e) => {
          e.preventDefault();
          void form.handleSubmit();
        }}
        className="flex min-h-0 gap-6"
      >
        <nav className="flex w-44 shrink-0 flex-col gap-1">
          {NAV.map((entry) => {
            const Icon = entry.icon;
            const active = section === entry.id;
            const danger = entry.id === 'danger';
            return (
              <button
                key={entry.id}
                type="button"
                onClick={() => setSection(entry.id)}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                  active ? (danger ? 'bg-destructive/10 font-medium text-destructive' : 'bg-accent font-medium text-accent-foreground') : danger ? 'text-destructive/80 hover:bg-destructive/10' : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground',
                )}
              >
                <Icon className="size-4" />
                {t(`projects.settings.${entry.labelKey}` as const)}
              </button>
            );
          })}
        </nav>

        <ScrollArea className="-mr-4 min-h-0 flex-1 pr-6">
          {section === 'general' && (
            <div className="flex flex-col gap-5">
              <form.Field name="name" validators={{ onChange: validators.required() }}>
                {(field) => (
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="project-settings-name" className="text-xs text-muted-foreground">
                      {t('projects.settings.name')}
                    </Label>
                    <Input id="project-settings-name" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.settings.namePlaceholder')} aria-invalid={isInvalid(field)} />
                    <FieldError field={field} />
                  </div>
                )}
              </form.Field>

              <ProjectLogoColorFields form={form} colorLabel={t('projects.settings.color')} />

              <form.Field name="path" validators={{ onChange: uniquePath }}>
                {(field) => (
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="project-settings-path" className="text-xs text-muted-foreground">
                      {t('projects.settings.localPath')}
                    </Label>
                    <Input id="project-settings-path" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.settings.localPathPlaceholder')} className="font-mono" aria-invalid={isInvalid(field)} />
                    <FieldError field={field} />
                  </div>
                )}
              </form.Field>

              <form.Field name="isolatedDefault">
                {(field) => (
                  <div className="flex items-start justify-between gap-4 rounded-md border border-border p-3">
                    <div className="flex min-w-0 flex-col gap-1">
                      <Label htmlFor="project-settings-isolated" className="text-sm">
                        {t('projects.settings.isolatedDefault')}
                      </Label>
                      <p className="text-xs text-muted-foreground">{t('projects.settings.isolatedDefaultHint')}</p>
                    </div>
                    <Switch id="project-settings-isolated" checked={field.state.value} onCheckedChange={(v) => field.handleChange(v)} />
                  </div>
                )}
              </form.Field>

              <form.Subscribe selector={(state) => state.values.isolatedDefault}>
                {(isolatedOn) =>
                  isolatedOn ? (
                    <form.Field name="branchPrefix">
                      {(field) => (
                        <div className="flex flex-col gap-2">
                          <Label htmlFor="project-settings-branch-prefix" className="text-xs text-muted-foreground">
                            {t('projects.settings.branchPrefix')}
                          </Label>
                          <Input id="project-settings-branch-prefix" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder="polaris/" className="font-mono" />
                          <p className="text-[11px] text-muted-foreground">{t('projects.settings.branchPrefixHint')}</p>
                        </div>
                      )}
                    </form.Field>
                  ) : null
                }
              </form.Subscribe>
            </div>
          )}

          {section === 'integrations' && (
            <div className="flex flex-col gap-3">
              <div className="flex items-start justify-between gap-3">
                <p className="text-xs text-muted-foreground">{t('projects.settings.integrationsHint')}</p>
                {project.path && (
                  <Button type="button" variant="outline" size="sm" onClick={redetect} disabled={redetecting}>
                    {redetecting ? t('projects.settings.redetecting') : t('projects.settings.redetect')}
                  </Button>
                )}
              </div>
              <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                {INTEGRATIONS.map((integration) => (
                  <IntegrationCard key={integration.id} integration={integration} connected={isConnected(project, integration.id)} onToggle={setConfigureId} />
                ))}
              </div>
            </div>
          )}

          {section === 'danger' && (
            <div className="flex flex-col gap-3">
              <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-destructive">{t('projects.settings.deleteTitle')}</div>
                    <p className="mt-1 text-xs text-muted-foreground">{t('projects.settings.deleteDesc', { name: project.name })}</p>
                  </div>
                  {confirmDelete ? (
                    <div className="flex shrink-0 gap-2">
                      <Button type="button" variant="outline" size="sm" onClick={() => setConfirmDelete(false)} disabled={deleting}>
                        {t('projects.settings.cancel')}
                      </Button>
                      <Button type="button" variant="destructive" size="sm" onClick={remove} disabled={deleting}>
                        {deleting ? t('projects.settings.deleting') : t('projects.settings.confirm')}
                      </Button>
                    </div>
                  ) : (
                    <Button type="button" variant="destructive" size="sm" onClick={() => setConfirmDelete(true)}>
                      {t('projects.settings.deleteCta')}
                    </Button>
                  )}
                </div>
              </div>
            </div>
          )}
        </ScrollArea>
      </form>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          {t('projects.settings.close')}
        </Button>
        {section === 'general' && (
          <form.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting, state.isDirty] as const}>
            {([canSubmit, isSubmitting, isDirty]) => (
              <Button type="submit" form="project-settings-form" disabled={!canSubmit || isSubmitting || !isDirty}>
                {isSubmitting ? t('projects.settings.saving') : t('projects.settings.saveChanges')}
              </Button>
            )}
          </form.Subscribe>
        )}
      </DialogFooter>

      {configureId && <ConfigureIntegrationModal projectId={project.id} integrationId={configureId} open={true} onOpenChange={(o) => !o && setConfigureId(null)} />}
    </>
  );
}
