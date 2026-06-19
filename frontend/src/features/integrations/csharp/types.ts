export interface CSharpConfig {
  [key: string]: unknown;
  manifestPath?: string;
  packageManager?: string;
  startScript?: string;
  testScript?: string;
  buildScript?: string;
  runEnv?: string;
}

// addArgs/removeArgs/installArgs map a high-level package action to the dotnet
// CLI argv. A spec is `name@version` (from formatSpec); a bare name has no
// version. The dotnet CLI has no dev-dependency concept, so `dev` is ignored.
export function addArgs(_pm: string, spec: string, _dev: boolean): string[] {
  const [name, version] = spec.split('@');
  if (version) {
    return ['add', 'package', name, '--version', version];
  }
  return ['add', 'package', spec];
}

export function removeArgs(_pm: string, name: string): string[] {
  return ['remove', 'package', name];
}

export function installArgs(_pm: string): string[] {
  return ['restore'];
}
