import type { repository } from '@/wailsjs/go/models';

export type RepoConfig = {
  provider?: string;
  owner?: string;
  repo?: string;
  baseUrl?: string;
};

export const PROVIDER_LABEL: Record<string, string> = {
  github: 'GitHub',
  gitlab: 'GitLab',
  bitbucket: 'Bitbucket',
};

// Re-export Wails-generated DTOs through a local module so the rest of the
// repository views never touch `@/wailsjs/go/models` directly — keeps imports
// stable across `wails generate` regenerations.
export type Label = repository.Label;
export type WorkflowRun = repository.WorkflowRun;
export type WorkflowDispatchInput = repository.WorkflowDispatchInput;
export type WorkflowDispatchSpec = repository.WorkflowDispatchSpec;
