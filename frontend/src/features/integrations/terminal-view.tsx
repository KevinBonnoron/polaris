import { Terminal, useTerminal } from '@wterm/react';
import { useEffect, useRef, useState } from 'react';
import { patchScrollToBottom } from './terminal-scroll-fix';
import { TerminalScrollbar } from './terminal-scrollbar';
import { useTerminalAutoscroll } from './use-terminal-autoscroll';

interface RunLine {
  line: string;
  stream: string;
  seq: number;
}

interface TerminalViewProps {
  runId: string | undefined;
  lines: RunLine[];
}

export function TerminalView({ runId, lines }: TerminalViewProps) {
  const { ref, write } = useTerminal();
  const [resized, setResized] = useState(false);
  const [scroller, setScroller] = useState<HTMLElement | null>(null);
  const firstResizeRef = useRef(false);
  const countRef = useRef(0);
  const linesRef = useRef(lines);
  linesRef.current = lines;
  const mountedRef = useRef(false);

  useTerminalAutoscroll(scroller);

  // Replay on first resize (correct terminal dimensions)
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once on resized
  useEffect(() => {
    if (!resized) {
      return;
    }
    const initial = linesRef.current;
    for (const ln of initial) {
      emitLine(write, ln);
    }
    countRef.current = initial.length;
  }, [resized]);

  const lineCount = lines.length;
  // biome-ignore lint/correctness/useExhaustiveDependencies: lineCount is the trigger
  useEffect(() => {
    if (!resized) {
      return;
    }
    const slice = lines.slice(countRef.current);
    for (const ln of slice) {
      emitLine(write, ln);
    }
    countRef.current = lines.length;
  }, [lineCount, resized]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: runId is the trigger
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true;
      return;
    }
    write('\x1bc');
    countRef.current = 0;
  }, [runId]);

  return (
    <div className="relative h-full w-full">
      <Terminal
        ref={ref}
        className="h-full w-full"
        rows={1}
        autoResize
        cursorBlink={false}
        onResize={() => {
          if (!firstResizeRef.current) {
            firstResizeRef.current = true;
            setResized(true);
            setScroller(patchScrollToBottom(ref.current?.instance));
          }
        }}
      />
      <TerminalScrollbar scroller={scroller} />
    </div>
  );
}

function emitLine(write: (data: string) => void, ln: RunLine) {
  if (ln.stream === 'system') {
    write(`\x1b[2m${ln.line}\x1b[0m\r\n`);
  } else {
    write(`${ln.line}\r\n`);
  }
}
