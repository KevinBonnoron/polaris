import type { Project } from '@/types';

/**
 * A project is "automatable" if at least one integration we know how to drive
 * an automation from is connected. Used by both the sidebar (to hide the
 * Automations entry entirely) and the page (to filter the project list).
 */
export function projectHasAutomatable(project: Project): boolean {
  const integrations = project.integrations ?? {};
  return Object.hasOwn(integrations, 'tickets') || Object.hasOwn(integrations, 'repository') || Object.hasOwn(integrations, 'sentry') || Object.hasOwn(integrations, 'dokploy');
}
