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
