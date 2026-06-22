import type { IntegrationConfig, Project, ProjectIntegrations } from '@/types';
import type { Integration } from './integration-catalog';

const INSTANCES_KEY = '_instances';
const LAUNCH_KEY = '_launch';

// Stored under a reserved key in the integrations map: survives the backend JSON round-trip and is never iterated as an integration (the UI walks the fixed catalog).
export type LaunchTarget = { kind: string; instanceIndex: number };

export function getLaunchTarget(project: Project | null | undefined): LaunchTarget | null {
  const raw = getIntegrations(project)[LAUNCH_KEY] as { kind?: unknown; instanceIndex?: unknown } | undefined;
  if (!raw || typeof raw.kind !== 'string') {
    return null;
  }
  return { kind: raw.kind, instanceIndex: Number(raw.instanceIndex) || 0 };
}

export function withLaunchTarget(project: Project, target: LaunchTarget): ProjectIntegrations {
  return { ...getIntegrations(project), [LAUNCH_KEY]: { ...target } };
}

export function getIntegrations(project: Project | null | undefined): ProjectIntegrations {
  return project?.integrations ?? {};
}

export function isConnected(project: Project | null | undefined, id: string): boolean {
  const map = getIntegrations(project);
  return Object.hasOwn(map, id);
}

export function isIntegrationConnected(project: Project | null | undefined, integration: Integration): boolean {
  const key = integration.storageKey ?? integration.id;
  const config = getIntegrations(project)[key];
  if (!config) {
    return false;
  }
  if (!integration.fixedValues) {
    return true;
  }
  return Object.entries(integration.fixedValues).every(([k, v]) => config[k] === v);
}

export function effectiveStorageKey(integration: Integration): string {
  return integration.storageKey ?? integration.id;
}

export function withIntegration(project: Project, id: string, config: Record<string, unknown>): ProjectIntegrations {
  return { ...getIntegrations(project), [id]: config };
}

export function withoutIntegration(project: Project, id: string): ProjectIntegrations {
  const next = { ...getIntegrations(project) };
  delete next[id];
  return next;
}

export function getInstances(project: Project | null | undefined, id: string): IntegrationConfig[] {
  const raw = getIntegrations(project)[id];
  if (!raw) {
    return [];
  }
  const arr = raw[INSTANCES_KEY];
  if (Array.isArray(arr)) {
    return arr as IntegrationConfig[];
  }
  return [raw];
}

export function getInstance(project: Project | null | undefined, id: string, index: number): IntegrationConfig | undefined {
  return getInstances(project, id)[index];
}

function withInstances(project: Project, id: string, configs: IntegrationConfig[]): ProjectIntegrations {
  return withIntegration(project, id, { [INSTANCES_KEY]: configs });
}

export function withInstanceUpdate(project: Project, id: string, index: number, config: IntegrationConfig): ProjectIntegrations {
  const instances = [...getInstances(project, id)];
  instances[index] = config;
  return withInstances(project, id, instances);
}

export function withInstanceRemoved(project: Project, id: string, index: number): ProjectIntegrations {
  const instances = getInstances(project, id).filter((_, i) => i !== index);
  if (instances.length === 0) {
    return withoutIntegration(project, id);
  }
  return withInstances(project, id, instances);
}
