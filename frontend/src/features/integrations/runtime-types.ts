export interface RunLineEvent {
  runId: string;
  stream: 'stdout' | 'stderr' | 'system';
  line: string;
  ts: number;
}

export interface RunExitEvent {
  runId: string;
  code: number;
  error?: string;
}

export interface RunLine extends RunLineEvent {
  seq: number;
}

export interface RunState {
  runId: string;
  scriptName: string;
  manifest?: string;
  lines: RunLine[];
  exited?: RunExitEvent;
  agentId?: string;
}

export interface WorkspaceCommand {
  args: string[];
  label: string;
  manifest: string;
}
