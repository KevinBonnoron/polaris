import type { gh } from '@/wailsjs/go/models';

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
export type Label = gh.Label;
export type WorkflowRun = gh.WorkflowRun;
export type WorkflowDispatchInput = gh.WorkflowDispatchInput;
export type WorkflowDispatchSpec = gh.WorkflowDispatchSpec;
