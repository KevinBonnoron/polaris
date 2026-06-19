export interface DokployConfig {
  baseUrl?: string;
  apiKey?: string;
  projects?: string;
}

export interface ConnectedDokployConfig {
  baseUrl: string;
  apiKey: string;
  projects: string[];
}

export function parseProjects(raw: string | undefined): string[] {
  if (!raw) {
    return [];
  }
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}
