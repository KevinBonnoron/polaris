import { getIntegrations } from '@/features/integrations/project-integrations';
import type { Project } from '@/types';
import type { SentryConfig } from './types';

export function useSentryConfig(project: Project | null | undefined): { config: SentryConfig | null } {
  const config = (project ? (getIntegrations(project).sentry as SentryConfig | undefined) : undefined) ?? null;
  return { config };
}
