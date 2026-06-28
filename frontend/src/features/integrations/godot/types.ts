export interface GodotConfig {
  [key: string]: unknown;
  manifestPath?: string;
  packageManager?: string;
  runEnv?: string;
  playCommand?: string;
}
