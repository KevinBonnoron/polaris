import { useLiveQuery } from '@tanstack/react-db';
import { Check } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { agentsCollection } from '@/collections/agents.collection';
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { ProjectAvatar } from '@/features/projects/project-avatar';
import { sortProjects, useProjectSort } from '@/features/projects/project-sort';
import { useCurrentProject, useProjects } from '@/state/projects';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ProjectSwitcher({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const { projects } = useProjects();
  const { project: currentProject, setProjectId } = useCurrentProject();
  const { data: agents = [] } = useLiveQuery((q) => q.from({ a: agentsCollection }));
  const [projectSort] = useProjectSort();
  const [query, setQuery] = useState('');

  const sortedProjects = useMemo(() => sortProjects(projects, agents, projectSort), [projects, agents, projectSort]);
  const visibleProjects = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? sortedProjects.filter((p) => p.name.toLowerCase().includes(q)) : sortedProjects;
  }, [sortedProjects, query]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setQuery('');
    }
    onOpenChange(nextOpen);
  };

  const pick = (id: string) => {
    handleOpenChange(false);
    requestAnimationFrame(() => setProjectId(id));
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton={false} className="overflow-hidden p-0 sm:max-w-[480px]">
        <DialogTitle className="sr-only">{t('projectSwitcher.title')}</DialogTitle>
        <DialogDescription className="sr-only">{t('projectSwitcher.description')}</DialogDescription>
        <Command shouldFilter={false}>
          <CommandInput placeholder={t('projectSwitcher.placeholder')} value={query} onValueChange={setQuery} />
          <CommandList>
            <CommandEmpty>{t('projectSwitcher.empty')}</CommandEmpty>
            <CommandGroup>
              {visibleProjects.map((p) => (
                <CommandItem key={p.id} value={p.id} onSelect={() => pick(p.id)}>
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
