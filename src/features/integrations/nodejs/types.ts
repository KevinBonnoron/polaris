export interface NodejsConfig {
  [key: string]: unknown;
  manifestPath?: string;
  packageManager?: string;
  startScript?: string;
  testScript?: string;
  buildScript?: string;
  runEnv?: string;
  extraScripts?: string;
}

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
