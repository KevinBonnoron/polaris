import { getIntegrations } from '@/features/integrations/project-integrations';
import type { Project } from '@/types';
import type { DokployConfig } from './types';

export function useDokployConfig(project: Project | null | undefined): { config: DokployConfig | null } {
  const config = (project ? (getIntegrations(project).dokploy as DokployConfig | undefined) : undefined) ?? null;
  return { config };
}
