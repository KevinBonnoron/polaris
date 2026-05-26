import type { CSSProperties, ReactNode } from 'react';

// A file path: optional directory segments (which may be hidden, e.g. `.fallow/`)
// followed by a filename that is either `name.ext` or a dotfile of any length
// (`.gitignore`, `.dockerignore`, `.env`).
const DIR_SEGMENTS = '(?:[a-zA-Z0-9_.-]+\\/)*';
const FILE_NAME = '(?:[a-zA-Z0-9_.-]*\\.[a-zA-Z]{2,10}|\\.[a-zA-Z][a-zA-Z0-9_-]*)';

const FILE_PATH_RE = new RegExp(`(?:^|(?<=\\s))(@?${DIR_SEGMENTS}${FILE_NAME})(?=\\s|$)`, 'g');

const MENTION_RE = new RegExp(`(^|\\s)@(${DIR_SEGMENTS}${FILE_NAME})(?=\\s|$)`, 'g');

const HIDDEN: CSSProperties = { color: 'transparent' };

export function stripFileMentions(text: string): string {
  return text.replace(MENTION_RE, '$1$2');
}

interface HighlightOptions {
  hidden?: boolean;
  // Only highlight tokens that carry an explicit `@` (a real mention), so plain
  // paths pasted into a message — e.g. a `file: ../foo.yml` line from a log —
  // aren't decorated.
  requireAt?: boolean;
  // When set, only highlight a path the predicate recognises as an existing
  // project file. Pairs with the autocomplete file list.
  isKnownFile?: (path: string) => boolean;
}

export function highlightSegments(text: string, { hidden, requireAt, isKnownFile }: HighlightOptions = {}): ReactNode[] {
  const style = hidden ? HIDDEN : undefined;
  const segments: ReactNode[] = [];
  let last = 0;
  let idx = 0;
  for (const match of text.matchAll(FILE_PATH_RE)) {
    const raw = match[1];
    const hasAt = raw.startsWith('@');
    const path = hasAt ? raw.slice(1) : raw;
    // Skip non-mentions: leave them in the surrounding plain text.
    if ((requireAt && !hasAt) || (isKnownFile && !isKnownFile(path))) {
      continue;
    }
    const start = match.index;
    if (start > last) {
      segments.push(
        <span key={idx++} style={style}>
          {text.slice(last, start)}
        </span>,
      );
    }
    segments.push(
      <mark key={idx++} className="rounded-sm bg-blue-500/20 text-foreground dark:bg-blue-400/20" style={style}>
        {raw}
      </mark>,
    );
    last = start + match[0].length;
  }

  if (last < text.length) {
    segments.push(
      <span key={idx++} style={style}>
        {text.slice(last)}
      </span>,
    );
  }

  return segments;
}
