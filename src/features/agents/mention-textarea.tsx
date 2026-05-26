import { File } from 'lucide-react';
import { type ChangeEvent, type ClipboardEvent, type KeyboardEvent, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { ListProjectDir, PasteClipboardImage } from '@/wailsjs/go/main/App';

const MAX_FILES = 300;
const MAX_RESULTS = 50;

async function loadFilesFlat(projectPath: string): Promise<string[]> {
  const files: string[] = [];
  const queue: string[] = [''];
  while (queue.length > 0 && files.length < MAX_FILES) {
    const dir = queue.shift();
    if (dir === undefined) { break; }
    try {
      const entries = await ListProjectDir(projectPath, dir);
      for (const e of entries) {
        if (files.length >= MAX_FILES) { break; }
        if (e.isDir) {
          queue.push(e.path);
        } else {
          files.push(e.path);
        }
      }
    } catch {
      // skip inaccessible dirs
    }
  }
  return files;
}

const FILE_PATH_RE = /(?:^|(?<=\s))([a-zA-Z0-9_.\-]+(?:\/[a-zA-Z0-9_.\-]+)*\.[a-zA-Z]{2,10})(?=\s|$)/g;

const HIDDEN: React.CSSProperties = { color: 'transparent' };

function renderHighlighted(text: string): React.ReactNode[] {
  const segments: React.ReactNode[] = [];
  let last = 0;
  let idx = 0;
  FILE_PATH_RE.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = FILE_PATH_RE.exec(text)) !== null) {
    if (match.index > last) {
      segments.push(<span key={idx++} style={HIDDEN}>{text.slice(last, match.index)}</span>);
    }
    segments.push(
      <mark key={idx++} className="rounded-sm bg-blue-500/20 dark:bg-blue-400/20" style={HIDDEN}>{match[1]}</mark>,
    );
    last = FILE_PATH_RE.lastIndex;
  }
  if (last < text.length) {
    segments.push(<span key={idx++} style={HIDDEN}>{text.slice(last)}</span>);
  }
  return segments;
}

interface Props {
  value: string;
  onChange: (value: string) => void;
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  onAttach?: (path: string) => void;
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  autoFocus?: boolean;
  projectPath: string | undefined;
}

export function MentionTextarea({ value, onChange, onKeyDown, onAttach, disabled, placeholder, className, autoFocus, projectPath }: Props) {
  const { t } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const filesCache = useRef<string[] | null>(null);
  const loadingRef = useRef(false);

  const [open, setOpen] = useState(false);
  const [mentionStart, setMentionStart] = useState(0);
  const [mentionQuery, setMentionQuery] = useState('');
  const [files, setFiles] = useState<string[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);

  const filtered = files
    .filter((f) => !mentionQuery || f.toLowerCase().includes(mentionQuery.toLowerCase()))
    .slice(0, MAX_RESULTS);

  useEffect(() => {
    itemRefs.current[selectedIndex]?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  const triggerLoad = () => {
    if (!projectPath || filesCache.current || loadingRef.current) { return; }
    loadingRef.current = true;
    loadFilesFlat(projectPath)
      .then((result) => {
        filesCache.current = result;
        setFiles(result);
      })
      .catch(() => {})
      .finally(() => {
        loadingRef.current = false;
      });
  };

  const handleChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value;
    const pos = e.target.selectionStart ?? val.length;
    const textBefore = val.slice(0, pos);
    const match = textBefore.match(/@([^\s@]*)$/);

    if (match) {
      const start = pos - match[0].length;
      setMentionStart(start);
      if (match[1] !== mentionQuery) { setSelectedIndex(0); }
      setMentionQuery(match[1]);
      if (!open) {
        setOpen(true);
        triggerLoad();
      }
    } else if (open) {
      setOpen(false);
    }

    onChange(val);
  };

  const selectFile = (path: string) => {
    const before = value.slice(0, mentionStart);
    const after = value.slice(mentionStart + 1 + mentionQuery.length);
    const suffix = after.startsWith(' ') || after === '' ? after : ` ${after}`;
    onChange(`${before}${path}${suffix}`);
    setOpen(false);
    textareaRef.current?.focus();
  };

  const insertAtCursor = (text: string) => {
    const el = textareaRef.current;
    if (!el) {
      onChange(value ? `${value}\n${text}` : text);
      return;
    }
    const start = el.selectionStart;
    const end = el.selectionEnd;
    const newVal = value.slice(0, start) + text + value.slice(end);
    onChange(newVal);
    requestAnimationFrame(() => {
      el.setSelectionRange(start + text.length, start + text.length);
    });
  };

  const handlePaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    e.preventDefault();
    const plainText = e.clipboardData.getData('text/plain');
    PasteClipboardImage()
      .then((path) => { onAttach ? onAttach(path) : insertAtCursor(path); })
      .catch(() => {
        if (plainText) { insertAtCursor(plainText); }
      });
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (open) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, filtered.length - 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
        return;
      }
      if (e.key === 'Enter' && filtered[selectedIndex]) {
        e.preventDefault();
        selectFile(filtered[selectedIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.stopPropagation();
        setOpen(false);
        return;
      }
    }
    onKeyDown?.(e);
  };

  const syncOverlayScroll = () => {
    if (overlayRef.current && textareaRef.current) {
      overlayRef.current.scrollTop = textareaRef.current.scrollTop;
    }
  };

  return (
      <div className="relative flex-1">
        {open && (
          <div className="absolute bottom-full left-0 z-50 mb-1 w-full min-w-[320px] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md">
            <ScrollArea className="max-h-52">
              {filtered.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">{t('agents.detail.mentionNoFiles')}</p>
              ) : (
                <div className="py-1">
                  {filtered.map((path, i) => (
                    <button
                      key={path}
                      ref={(el) => { itemRefs.current[i] = el; }}
                      type="button"
                      className={cn(
                        'flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent hover:text-accent-foreground',
                        i === selectedIndex && 'bg-accent text-accent-foreground',
                      )}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        selectFile(path);
                      }}
                      onMouseEnter={() => setSelectedIndex(i)}
                    >
                      <File className="size-3.5 shrink-0 text-muted-foreground" />
                      <span className="truncate font-mono text-xs">{path}</span>
                    </button>
                  ))}
                </div>
              )}
            </ScrollArea>
          </div>
        )}
        <div
          ref={overlayRef}
          aria-hidden
          className="pointer-events-none absolute inset-px overflow-hidden whitespace-pre-wrap break-words rounded-[calc(var(--radius)-1px)] px-3 py-2 text-base md:text-sm"
        >
          {renderHighlighted(value)}
        </div>
        <Textarea
          ref={textareaRef}
          autoFocus={autoFocus}
          value={value}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          onScroll={syncOverlayScroll}
          disabled={disabled}
          placeholder={placeholder}
          className={cn('w-full !bg-transparent', className)}
        />
      </div>
  );
}
