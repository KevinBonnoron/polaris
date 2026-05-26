import { useForm } from '@tanstack/react-form';
import { Folder } from 'lucide-react';
import { type PropsWithChildren, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { INTEGRATIONS } from '@/features/integrations/integration-catalog';
import { FieldError, isInvalid, validators } from '@/lib/form';
import { toastError } from '@/lib/toast-error';
import { type DialogModeProps, useDialogMode } from '@/lib/use-dialog-mode';
import { useCurrentProject, useProjects } from '@/state/projects';
import type { ProjectIntegrations } from '@/types';
import { CloneRepository, OpenDirectory, UpsertProject } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';
import { deriveProjectName, randomProjectColor } from './project-colors';
import { ProjectLogoColorFields } from './project-logo-color-fields';
import { makeUniquePathValidator } from './unique-path';

interface LocalFormValues {
  path: string;
  name: string;
  color: string;
  logo: string | undefined;
}

interface CloneFormValues {
  url: string;
  parentDir: string;
  name: string;
  color: string;
  logo: string | undefined;
}

export function AddProjectModal({ children, ...modeProps }: PropsWithChildren<DialogModeProps>) {
  const { t } = useTranslation();
  const { open, setOpen } = useDialogMode(modeProps);
  const { setProjectId } = useCurrentProject();
  const { projects } = useProjects();
  const [browsing, setBrowsing] = useState(false);
  const uniquePath = makeUniquePathValidator(projects, 'projects.add.duplicatePath', t);

  const localForm = useForm({
    defaultValues: {
      path: '',
      name: '',
      color: randomProjectColor(),
      logo: undefined,
    } as LocalFormValues,
    onSubmit: async ({ value }) => {
      const path = value.path.trim();
      const name = value.name.trim();
      try {
        const integrations = await detectInitialIntegrations(path);
        const created = await UpsertProject({
          id: '',
          name,
          color: value.color,
          logo: value.logo || undefined,
          path,
          integrations,
        } as unknown as polaris.Project);
        if (created?.id) {
          setProjectId(created.id);
        }
        toast.success(t('projects.add.added', { name }));
        setOpen(false);
      } catch (err) {
        toastError({ title: t('projects.add.couldNotAdd'), err });
      }
    },
  });

  const cloneForm = useForm({
    defaultValues: {
      url: '',
      parentDir: '',
      name: '',
      color: randomProjectColor(),
      logo: undefined,
    } as CloneFormValues,
    onSubmit: async ({ value }) => {
      const url = value.url.trim();
      const parentDir = value.parentDir.trim();
      const name = value.name.trim();
      try {
        const clonedPath = await CloneRepository(url, parentDir);
        const integrations = await detectInitialIntegrations(clonedPath);
        const created = await UpsertProject({
          id: '',
          name: name || deriveRepoName(url),
          color: value.color,
          logo: value.logo || undefined,
          path: clonedPath,
          integrations,
        } as unknown as polaris.Project);
        if (created?.id) {
          setProjectId(created.id);
        }
        toast.success(t('projects.add.added', { name: name || deriveRepoName(url) }));
        setOpen(false);
      } catch (err) {
        toastError({ title: t('projects.add.couldNotClone'), err });
      }
    },
  });

  const handlePathChange = (next: string) => {
    const prev = localForm.state.values.path;
    const currentName = localForm.state.values.name;
    localForm.setFieldValue('path', next);
    const derived = deriveProjectName(next);
    if (!currentName || currentName === deriveProjectName(prev)) {
      localForm.setFieldValue('name', derived);
    }
  };

  const handleUrlChange = (next: string) => {
    const prev = cloneForm.state.values.url;
    const currentName = cloneForm.state.values.name;
    cloneForm.setFieldValue('url', next);
    const derived = deriveRepoName(next);
    if (!currentName || currentName === deriveRepoName(prev)) {
      cloneForm.setFieldValue('name', derived);
    }
  };

  const browse = async (onSelect: (path: string) => void, title: string) => {
    setBrowsing(true);
    try {
      const selected = await OpenDirectory(title);
      if (selected) {
        onSelect(selected);
      }
    } catch (err) {
      toastError({ title: t('projects.add.pickerUnavailable'), err });
    } finally {
      setBrowsing(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {children !== undefined && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent className="w-[min(95vw,840px)] sm:max-w-[840px] gap-4">
        <DialogHeader>
          <DialogTitle>{t('projects.add.title')}</DialogTitle>
          <DialogDescription>{t('projects.add.description')}</DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="local">
          <TabsContent value="local">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void localForm.handleSubmit();
              }}
              className="flex flex-col gap-4"
            >
              <div className="grid grid-cols-1 gap-6 sm:grid-cols-[1fr_auto_1fr]">
                <div className="flex flex-col gap-4">
                  <TabsList variant="line" className="w-full border-b border-border">
                    <TabsTrigger value="local" className="flex-1">
                      {t('projects.add.modeLocal')}
                    </TabsTrigger>
                    <TabsTrigger value="clone" className="flex-1">
                      {t('projects.add.modeClone')}
                    </TabsTrigger>
                  </TabsList>

                  <localForm.Field name="path" validators={{ onChange: validators.combine(validators.required(), uniquePath) }}>
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="add-project-path" className="text-xs text-muted-foreground">
                          {t('projects.add.folder')}
                        </Label>
                        <div className="flex gap-2">
                          <Input id="add-project-path" autoFocus value={field.state.value} onChange={(e) => handlePathChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.add.folderPlaceholder')} className="font-mono" aria-invalid={isInvalid(field)} />
                          <Button type="button" variant="outline" onClick={() => browse(handlePathChange, t('projects.add.pickerTitle'))} disabled={browsing}>
                            <Folder />
                            {browsing ? t('projects.add.picking') : t('projects.add.browse')}
                          </Button>
                        </div>
                        <FieldError field={field} />
                      </div>
                    )}
                  </localForm.Field>
                </div>

                <Separator orientation="vertical" className="hidden sm:block" />

                <div className="flex flex-col gap-4">
                  <localForm.Field name="name" validators={{ onChange: validators.required() }}>
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="add-project-name" className="text-xs text-muted-foreground">
                          {t('projects.add.name')}
                        </Label>
                        <Input id="add-project-name" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.add.namePlaceholder')} aria-invalid={isInvalid(field)} />
                        <FieldError field={field} />
                      </div>
                    )}
                  </localForm.Field>

                  <ProjectLogoColorFields form={localForm} colorLabel={t('projects.add.color')} />
                </div>
              </div>

              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                  {t('projects.add.cancel')}
                </Button>
                <localForm.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
                  {([canSubmit, isSubmitting]) => (
                    <Button type="submit" disabled={!canSubmit || isSubmitting}>
                      {isSubmitting ? t('projects.add.adding') : t('projects.add.addCta')}
                    </Button>
                  )}
                </localForm.Subscribe>
              </DialogFooter>
            </form>
          </TabsContent>

          <TabsContent value="clone">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void cloneForm.handleSubmit();
              }}
              className="flex flex-col gap-4"
            >
              <div className="grid grid-cols-1 gap-6 sm:grid-cols-[1fr_auto_1fr]">
                <div className="flex flex-col gap-4">
                  <TabsList variant="line" className="w-full border-b border-border">
                    <TabsTrigger value="local" className="flex-1">
                      {t('projects.add.modeLocal')}
                    </TabsTrigger>
                    <TabsTrigger value="clone" className="flex-1">
                      {t('projects.add.modeClone')}
                    </TabsTrigger>
                  </TabsList>

                  <cloneForm.Field name="url" validators={{ onChange: validators.required() }}>
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="clone-url" className="text-xs text-muted-foreground">
                          {t('projects.add.cloneUrl')}
                        </Label>
                        <Input id="clone-url" autoFocus value={field.state.value} onChange={(e) => handleUrlChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.add.cloneUrlPlaceholder')} className="font-mono" aria-invalid={isInvalid(field)} />
                        <FieldError field={field} />
                      </div>
                    )}
                  </cloneForm.Field>

                  <cloneForm.Field name="parentDir" validators={{ onChange: validators.required() }}>
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="clone-into" className="text-xs text-muted-foreground">
                          {t('projects.add.cloneInto')}
                        </Label>
                        <div className="flex gap-2">
                          <Input id="clone-into" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.add.cloneIntoPlaceholder')} className="font-mono" aria-invalid={isInvalid(field)} />
                          <Button type="button" variant="outline" onClick={() => browse((p) => field.handleChange(p), t('projects.add.clonePickerTitle'))} disabled={browsing}>
                            <Folder />
                            {browsing ? t('projects.add.picking') : t('projects.add.browse')}
                          </Button>
                        </div>
                        <FieldError field={field} />
                      </div>
                    )}
                  </cloneForm.Field>
                </div>

                <Separator orientation="vertical" className="hidden sm:block" />

                <div className="flex flex-col gap-4">
                  <cloneForm.Field name="name">
                    {(field) => (
                      <div className="flex flex-col gap-2">
                        <Label htmlFor="clone-name" className="text-xs text-muted-foreground">
                          {t('projects.add.name')}
                        </Label>
                        <Input id="clone-name" value={field.state.value} onChange={(e) => field.handleChange(e.target.value)} onBlur={field.handleBlur} placeholder={t('projects.add.namePlaceholder')} />
                      </div>
                    )}
                  </cloneForm.Field>

                  <ProjectLogoColorFields form={cloneForm} colorLabel={t('projects.add.color')} />
                </div>
              </div>

              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                  {t('projects.add.cancel')}
                </Button>
                <cloneForm.Subscribe selector={(state) => [state.canSubmit, state.isSubmitting] as const}>
                  {([canSubmit, isSubmitting]) => (
                    <Button type="submit" disabled={!canSubmit || isSubmitting}>
                      {isSubmitting ? t('projects.add.cloning') : t('projects.add.cloneCta')}
                    </Button>
                  )}
                </cloneForm.Subscribe>
              </DialogFooter>
            </form>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function deriveRepoName(url: string): string {
  const trimmed = url.trim();
  if (!trimmed) return '';
  const withoutGit = trimmed.replace(/\.git$/, '');
  const parts = withoutGit.replace(/[:@]/, '/').split('/');
  return parts[parts.length - 1] || '';
}

async function detectInitialIntegrations(path: string): Promise<ProjectIntegrations> {
  const integrations: ProjectIntegrations = {};
  if (!path) {
    return integrations;
  }

  await Promise.all(
    INTEGRATIONS.map(async ({ id, detect, multi }) => {
      if (!detect) {
        return;
      }

      try {
        const result = await detect(path);
        if (!result) return;
        if (multi && Array.isArray(result)) {
          integrations[id] = { _instances: result };
        } else if (multi && !Array.isArray(result)) {
          integrations[id] = { _instances: [result] };
        } else if (!Array.isArray(result)) {
          integrations[id] = result;
        }
      } catch {
        /* per-integration detection is best-effort */
      }
    }),
  );

  return integrations;
}
