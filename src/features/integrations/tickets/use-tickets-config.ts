import { useCallback } from 'react';
import { projectsCollection } from '@/collections/projects.collection';
import { getIntegrations } from '@/features/integrations/project-integrations';
import type { Project } from '@/types';
import type { TicketsConfig } from './types';

export function useTicketsConfig(project: Project | null | undefined): {
  config: TicketsConfig | null;
  update: (mutator: (draft: TicketsConfig) => void) => void;
} {
  const config = (project ? (getIntegrations(project).tickets as TicketsConfig | undefined) : undefined) ?? null;

  const update = useCallback(
    (mutator: (draft: TicketsConfig) => void) => {
      if (!project) {
        return;
      }

      const existing = (getIntegrations(project).tickets ?? {}) as TicketsConfig;
      const draft: TicketsConfig = { ...existing };
      mutator(draft);
      projectsCollection.update(project.id, (d) => {
        d.integrations = { ...(d.integrations ?? {}), tickets: draft as Record<string, unknown> };
      });
    },
    [project],
  );

  return { config, update };
}
