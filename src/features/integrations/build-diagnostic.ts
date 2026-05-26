interface DiagDependency {
  name: string;
  version: string;
  type: string;
  locations?: { workspace: string; manifest: string; version: string; type: string }[];
}

interface DiagOutdated {
  name: string;
  current: string;
  wanted: string;
  latest: string;
  workspace?: string;
}

interface DiagVuln {
  name: string;
  severity: string;
  title: string;
  url: string;
}

interface DiagWorkspace {
  name: string;
  manifest: string;
  isRoot: boolean;
}

export interface DiagnosticInput {
  runtime: string;
  projectName: string;
  manifestPath: string;
  packageManager: string;
  installed: boolean;
  packages: DiagDependency[];
  outdated: DiagOutdated[];
  unusedKeys: string[];
  vulns: DiagVuln[];
  workspaces: DiagWorkspace[];
  typeLabels: Record<string, string>;
}

const SEVERITY_ORDER = ['critical', 'high', 'moderate', 'low', 'info'];

export function buildDiagnostic(input: DiagnosticInput): string {
  const { runtime, projectName, manifestPath, packageManager, installed, packages, outdated, unusedKeys, vulns, workspaces, typeLabels } = input;
  const lines: string[] = [];

  lines.push(`# ${runtime} diagnostic — ${projectName}`);
  lines.push('');
  lines.push('Snapshot of the project dependency state for review by an agent before making changes.');
  lines.push('');
  lines.push(`- Manifest: ${manifestPath}`);
  lines.push(`- Package manager: ${packageManager}`);
  lines.push(`- Dependencies installed: ${installed ? 'yes' : 'no (run install)'}`);
  lines.push(`- Declared packages: ${packages.length}`);

  if (workspaces.length > 1) {
    lines.push(`- Workspaces (${workspaces.length}):`);
    for (const ws of workspaces) {
      lines.push(`  - ${ws.isRoot ? `${ws.name} (root)` : ws.name} — ${ws.manifest}`);
    }
  }

  lines.push('');
  lines.push(`## Outdated (${outdated.length})`);
  if (outdated.length === 0) {
    lines.push('None.');
  } else {
    for (const o of [...outdated].sort((a, b) => a.name.localeCompare(b.name))) {
      const wanted = o.wanted && o.wanted !== o.latest ? `, wanted ${o.wanted}` : '';
      const ws = o.workspace ? ` [${o.workspace}]` : '';
      lines.push(`- ${o.name}: ${o.current} → ${o.latest}${wanted}${ws}`);
    }
  }

  lines.push('');
  lines.push(`## Unused (${unusedKeys.length})`);
  if (unusedKeys.length === 0) {
    lines.push('None.');
  } else {
    for (const key of [...unusedKeys].sort()) {
      const [workspace, name] = key.includes('::') ? key.split('::') : ['', key];
      lines.push(`- ${name}${workspace ? ` [${workspace}]` : ''}`);
    }
  }

  lines.push('');
  lines.push(`## Vulnerabilities (${vulns.length})`);
  if (vulns.length === 0) {
    lines.push('None.');
  } else {
    const sorted = [...vulns].sort((a, b) => {
      const sa = SEVERITY_ORDER.indexOf(a.severity);
      const sb = SEVERITY_ORDER.indexOf(b.severity);
      return (sa < 0 ? SEVERITY_ORDER.length : sa) - (sb < 0 ? SEVERITY_ORDER.length : sb);
    });
    for (const v of sorted) {
      const sev = v.severity ? `${v.severity}: ` : '';
      const url = v.url ? ` (${v.url})` : '';
      lines.push(`- ${sev}${v.name} — ${v.title}${url}`);
    }
  }

  lines.push('');
  lines.push('## Declared dependencies');
  const groups = new Map<string, DiagDependency[]>();
  for (const pkg of packages) {
    const list = groups.get(pkg.type) ?? [];
    list.push(pkg);
    groups.set(pkg.type, list);
  }
  for (const [type, deps] of groups) {
    lines.push('');
    lines.push(`### ${typeLabels[type] ?? type} (${deps.length})`);
    for (const dep of [...deps].sort((a, b) => a.name.localeCompare(b.name))) {
      lines.push(`- ${dep.name}@${dep.version}`);
    }
  }

  lines.push('');
  return lines.join('\n');
}

// A spec is a dist-tag / wildcard (e.g. "latest", "next", "*") rather than a
// concrete version or range — these hide the actually installed version, so the
// resolved `current` is shown instead when known.
export function isTagSpec(spec: string): boolean {
  return /^[a-z*]/i.test(spec.trim());
}
