import { useCallback } from 'react';
import { projectsCollection } from '@/collections/projects.collection';
import { getIntegrations, withIntegration } from '@/features/integrations/project-integrations';
import type { Project } from '@/types';
import type { JiraConfig } from './types';

export function useJiraConfig(project: Project | null | undefined): {
  config: JiraConfig | null;
  update: (mutator: (draft: JiraConfig) => void) => void;
} {
  const config = (project ? (getIntegrations(project).jira as JiraConfig | undefined) : undefined) ?? null;

  const update = useCallback(
    (mutator: (draft: JiraConfig) => void) => {
      if (!project) {
        return;
      }

      const existing = (getIntegrations(project).jira ?? {}) as JiraConfig;
      const draft: JiraConfig = { ...existing };
      mutator(draft);
      const nextIntegrations = withIntegration(project, 'jira', draft as Record<string, unknown>);
      projectsCollection.update(project.id, (d) => {
        d.integrations = nextIntegrations;
      });
    },
    [project],
  );

  return { config, update };
}
