export interface SentryConfig {
  token?: string;
  org?: string;
  projects?: string;
  url?: string;
}

export interface ConnectedSentryConfig {
  token: string;
  org: string;
  projects: string[];
  url?: string;
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
