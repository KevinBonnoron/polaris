import type { polaris } from '@/wailsjs/go/models';

type StreamEvent = polaris.StreamEvent;

const WRITE_TOOLS = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit', 'Update']);

type LogFileEntry = { path: string; tool: string };

function extractFilesFromLog(events: StreamEvent[]): LogFileEntry[] {
  const seen = new Map<string, string>();
  for (const evt of events) {
    if (evt.type !== 'tool_call' || !evt.name || !WRITE_TOOLS.has(evt.name)) continue;
    const path = evt.content?.trim();
    if (path && !seen.has(path)) {
      seen.set(path, evt.name);
    }
  }
  return Array.from(seen, ([path, tool]) => ({ path, tool }));
}

function countFilesFromLog(events: StreamEvent[]): number {
  if (!events?.length) return 0;
  const seen = new Set<string>();
  for (const evt of events) {
    if (evt.type !== 'tool_call' || !evt.name || !WRITE_TOOLS.has(evt.name)) continue;
    const path = evt.content?.trim();
    if (path) seen.add(path);
  }
  return seen.size;
}

export function countToolsFromLog(events: StreamEvent[]): number {
  if (!events?.length) return 0;
  return events.filter((e) => e.type === 'tool_call').length;
}
