export interface TaskfileConfig {
  [key: string]: unknown;
  manifestPath?: string;
  packageManager?: string;
  runEnv?: string;
  startTask?: string;
  testTask?: string;
  buildTask?: string;
}
