import type { CSSProperties } from 'react';
import { cn } from '@/lib/utils';
import type { Project } from '@/types';
import { projectInitials } from './project-initials';

interface ProjectAvatarProps {
  project: Pick<Project, 'name' | 'color' | 'logo'> | null | undefined;
  className?: string;
  textClassName?: string;
  style?: CSSProperties;
}

export function ProjectAvatar({ project, className, textClassName, style }: ProjectAvatarProps) {
  const background = project?.logo ? undefined : (project?.color ?? 'var(--primary)');
  const initials = projectInitials(project?.name ?? '');

  return (
    <div className={cn('flex shrink-0 items-center justify-center overflow-hidden rounded-md text-sidebar-primary-foreground', className)} style={{ background, ...style }} aria-hidden>
      {project?.logo ? <img src={project.logo} alt="" className="size-full object-cover" /> : <span className={cn('select-none font-semibold leading-none', textClassName)}>{initials}</span>}
    </div>
  );
}
