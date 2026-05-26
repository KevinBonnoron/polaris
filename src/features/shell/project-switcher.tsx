import { Check } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { useCurrentProject, useProjects } from '@/state/projects';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ProjectSwitcher({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const { projects } = useProjects();
  const { project: currentProject, setProjectId } = useCurrentProject();

  const pick = (id: string) => {
    setProjectId(id);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} className="overflow-hidden p-0 sm:max-w-[480px]">
        <DialogTitle className="sr-only">{t('projectSwitcher.title')}</DialogTitle>
        <DialogDescription className="sr-only">{t('projectSwitcher.description')}</DialogDescription>
        <Command>
          <CommandInput placeholder={t('projectSwitcher.placeholder')} />
          <CommandList>
            <CommandEmpty>{t('projectSwitcher.empty')}</CommandEmpty>
            <CommandGroup>
              {projects.map((p) => (
                <CommandItem key={p.id} value={p.name} onSelect={() => pick(p.id)}>
                  <ProjectAvatar project={p} className="size-5 rounded" textClassName="text-[9px]" />
                  <span className="flex-1">{p.name}</span>
                  {p.id === currentProject?.id && <Check className="ml-auto size-4" />}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}
