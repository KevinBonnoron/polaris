export interface PythonConfig {
  [key: string]: unknown;
  manifestPath?: string;
  packageManager?: string;
  startScript?: string;
  testScript?: string;
  buildScript?: string;
  runEnv?: string;
}

// addArgs/removeArgs/installArgs map a high-level package action to the argv each
// Python package manager expects, so the page stays manager-agnostic.
export function addArgs(pm: string, name: string, dev: boolean): string[] {
  switch (pm) {
    case 'uv':
      return dev ? ['add', '--dev', name] : ['add', name];
    case 'poetry':
      return dev ? ['add', '--group', 'dev', name] : ['add', name];
    case 'pdm':
      return dev ? ['add', '-d', name] : ['add', name];
    case 'pipenv':
      return dev ? ['install', '--dev', name] : ['install', name];
    default:
      return ['install', name];
  }
}

export function removeArgs(pm: string, name: string): string[] {
  switch (pm) {
    case 'uv':
    case 'poetry':
    case 'pdm':
      return ['remove', name];
    case 'pipenv':
      return ['uninstall', name];
    default:
      return ['uninstall', '-y', name];
  }
}

export function installArgs(pm: string): string[] {
  switch (pm) {
    case 'uv':
      return ['sync'];
    case 'poetry':
    case 'pdm':
    case 'pipenv':
      return ['install'];
    default:
      return ['install', '-r', 'requirements.txt'];
  }
}
