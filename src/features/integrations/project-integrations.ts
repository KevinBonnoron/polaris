import type { Project, ProjectIntegrations } from '@/types';

export function getIntegrations(project: Project | null | undefined): ProjectIntegrations {
  return project?.integrations ?? {};
}

export function isConnected(project: Project | null | undefined, id: string): boolean {
  const map = getIntegrations(project);
  return Object.hasOwn(map, id);
}

export function withIntegration(project: Project, id: string, config: Record<string, unknown>): ProjectIntegrations {
  return { ...getIntegrations(project), [id]: config };
}

export function withoutIntegration(project: Project, id: string): ProjectIntegrations {
  const next = { ...getIntegrations(project) };
  delete next[id];
  return next;
}
