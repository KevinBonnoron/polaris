import { Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { ClearAppLog, ReadAppLog } from '@/wailsjs/go/main/App';

type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN';

interface LogLine {
  raw: string;
  level: LogLevel;
}

function parseLevel(raw: string): LogLevel {
  if (raw.includes('[DEBUG]')) return 'DEBUG';
  if (raw.includes('[INFO]')) return 'INFO';
  if (raw.includes('[WARN]')) return 'WARN';
  if (raw.includes('[ERROR]')) return 'ERROR';
  return 'UNKNOWN';
}

const LEVEL_STYLES: Record<LogLevel, string> = {
  DEBUG: 'text-muted-foreground/60',
  INFO: 'text-foreground',
  WARN: 'text-amber-400',
  ERROR: 'text-destructive',
  UNKNOWN: 'text-muted-foreground',
};

const ALL_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR', 'UNKNOWN'];

export function LogsPage() {
  const { t } = useTranslation();
  const [lines, setLines] = useState<LogLine[]>([]);
  const [logPath, setLogPath] = useState<string>('');
  const [error, setError] = useState<string>('');
  const [activelevels, setActiveLevels] = useState<Set<LogLevel>>(new Set<LogLevel>(['INFO', 'WARN', 'ERROR', 'UNKNOWN']));
  const offsetRef = useRef<number>(0);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function poll() {
      if (cancelled) return;
      try {
        const tail = await ReadAppLog(offsetRef.current);
        if (!cancelled) {
          if (tail.path && !logPath) setLogPath(tail.path);
          if (tail.lines && tail.lines.length > 0) {
            setLines((prev) => [...prev, ...tail.lines.map((raw) => ({ raw, level: parseLevel(raw) }))]);
            offsetRef.current = tail.offset;
          }
          setError('');
        }
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
      if (!cancelled) setTimeout(poll, 2000);
    }

    void poll();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (pinned) bottomRef.current?.scrollIntoView({ behavior: 'instant' });
  }, [lines, pinned]);

  function handleScroll(e: React.UIEvent<HTMLDivElement>) {
    const el = e.currentTarget;
    setPinned(el.scrollHeight - el.scrollTop - el.clientHeight < 40);
  }

  function toggleLevel(level: LogLevel) {
    setActiveLevels((prev) => {
      const next = new Set(prev);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next;
    });
  }

  const visible = lines.filter(({ level }) => activelevels.has(level));

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-4 py-2">
        <h1 className="text-sm font-semibold">{t('logs.title')}</h1>
        <div className="flex gap-1">
          {ALL_LEVELS.map((level) => (
            <button key={level} type="button" onClick={() => toggleLevel(level)} className={cn('rounded px-1.5 py-0.5 font-mono text-[10px] transition-opacity', LEVEL_STYLES[level], activelevels.has(level) ? 'opacity-100 ring-1 ring-current' : 'opacity-30')}>
              {level}
            </button>
          ))}
        </div>
        {logPath && <span className="font-mono text-xs text-muted-foreground">{logPath}</span>}
        <button
          type="button"
          className="ml-auto text-muted-foreground hover:text-foreground"
          title={t('logs.clear')}
          onClick={() => {
            ClearAppLog()
              .then(() => {
                setLines([]);
                offsetRef.current = 0;
              })
              .catch(() => {});
          }}
        >
          <Trash2 className="size-3.5" />
        </button>
      </div>
      {error && <div className="border-b border-border bg-destructive/10 px-4 py-2 font-mono text-xs text-destructive">{error}</div>}
      <div className="relative flex-1 overflow-hidden">
        <div className="h-full overflow-y-auto bg-background p-4 font-mono text-xs" onScroll={handleScroll}>
          {visible.length === 0 ? (
            <span className="text-muted-foreground">{t('logs.empty')}</span>
          ) : (
            visible.map(({ raw, level }, i) => (
              <div key={i} className={cn('whitespace-pre-wrap break-all leading-5', LEVEL_STYLES[level])}>
                {raw}
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
        {!pinned && (
          <button
            type="button"
            onClick={() => {
              setPinned(true);
              bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
            }}
            className="absolute right-4 bottom-4 rounded-md border border-border bg-background px-2 py-1 text-xs text-muted-foreground shadow-sm hover:text-foreground"
          >
            {t('logs.scrollToBottom')}
          </button>
        )}
      </div>
    </div>
  );
}
