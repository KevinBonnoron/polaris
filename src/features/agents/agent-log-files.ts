const WRITE_TOOLS = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit', 'Update']);
const TOOL_CALL_RE = /^(?:\[\d{2}:\d{2}:\d{2}\]\s?)?→ (?:\[#[^\]]+\]\s+)?(\S+)(?: · (.+))?/;

export type LogFileEntry = { path: string; tool: string };

function* iterWriteCalls(log: string): Iterable<{ tool: string; path: string }> {
  for (const line of log.split('\n')) {
    const match = line.match(TOOL_CALL_RE);
    if (!match) {
      continue;
    }
    const [, tool, rawPath] = match;
    if (!rawPath || !WRITE_TOOLS.has(tool)) {
      continue;
    }
    yield { tool, path: rawPath.trim() };
  }
}

export function extractFilesFromLog(log: string): LogFileEntry[] {
  const seen = new Map<string, string>();
  for (const { tool, path } of iterWriteCalls(log)) {
    if (!seen.has(path)) {
      seen.set(path, tool);
    }
  }
  return Array.from(seen, ([path, tool]) => ({ path, tool }));
}

export function countFilesFromLog(log: string): number {
  if (!log) {
    return 0;
  }
  const seen = new Set<string>();
  for (const { path } of iterWriteCalls(log)) {
    seen.add(path);
  }
  return seen.size;
}

export function countToolsFromLog(log: string): number {
  if (!log) {
    return 0;
  }
  let count = 0;
  for (const line of log.split('\n')) {
    if (TOOL_CALL_RE.test(line)) {
      count++;
    }
  }
  return count;
}
