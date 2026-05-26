import { File, Terminal } from 'lucide-react';
import { type ChangeEvent, type ClipboardEvent, type KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Textarea } from '@/components/ui/textarea';
import { toastError } from '@/lib/toast-error';
import { cn } from '@/lib/utils';
import { ListProjectDir, PasteClipboardImage, SaveTempImage } from '@/wailsjs/go/main/App';
import { highlightSegments } from './file-mentions';

const UNDO_LIMIT = 200;
const DEBOUNCE_MS = 300;

function extForMime(mime: string): string {
  switch (mime) {
    case 'image/jpeg':
      return '.jpg';
    case 'image/gif':
      return '.gif';
    case 'image/webp':
      return '.webp';
    case 'image/bmp':
      return '.bmp';
    default:
      return '.png';
  }
}

function useUndoStack(value: string, onChange: (v: string) => void) {
  const undoStack = useRef<string[]>([]);
  const redoStack = useRef<string[]>([]);
  const baseValue = useRef(value);
  const prevValue = useRef(value);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    if (value === prevValue.current) {
      return;
    }
    const prev = prevValue.current;
    prevValue.current = value;
    if (value === baseValue.current) {
      return;
    }

    clearTimeout(timerRef.current);

    const diff = value.length - prev.length;
    const wordBoundary = diff === 1 && /\s$/.test(value) && !/\s$/.test(prev);
    const bulk = Math.abs(diff) > 1;

    if (wordBoundary || bulk) {
      undoStack.current.push(baseValue.current);
      if (undoStack.current.length > UNDO_LIMIT) {
        undoStack.current.shift();
      }
      redoStack.current = [];
      baseValue.current = value;
      return;
    }

    timerRef.current = setTimeout(() => {
      if (value !== baseValue.current) {
        undoStack.current.push(baseValue.current);
        if (undoStack.current.length > UNDO_LIMIT) {
          undoStack.current.shift();
        }

        redoStack.current = [];
        baseValue.current = value;
      }
    }, DEBOUNCE_MS);
  }, [value]);

  const undo = useCallback(() => {
    clearTimeout(timerRef.current);
    if (value !== baseValue.current) {
      redoStack.current.push(value);
      onChange(baseValue.current);
      return;
    }
    const prev = undoStack.current.pop();
    if (prev !== undefined) {
      redoStack.current.push(value);
      baseValue.current = prev;
      onChange(prev);
    }
  }, [value, onChange]);

  const redo = useCallback(() => {
    clearTimeout(timerRef.current);
    const next = redoStack.current.pop();
    if (next !== undefined) {
      undoStack.current.push(value);
      baseValue.current = next;
      onChange(next);
    }
  }, [value, onChange]);

  const reset = useCallback(() => {
    clearTimeout(timerRef.current);
    undoStack.current = [];
    redoStack.current = [];
    baseValue.current = '';
    prevValue.current = '';
  }, []);

  return { undo, redo, reset };
}

const MAX_FILES = 300;
const MAX_RESULTS = 50;

async function loadFilesFlat(projectPath: string): Promise<string[]> {
  const files: string[] = [];
  const queue: string[] = [''];
  while (queue.length > 0 && files.length < MAX_FILES) {
    const dir = queue.shift();
    if (dir === undefined) {
      break;
    }
    try {
      const entries = await ListProjectDir(projectPath, dir);
      for (const e of entries) {
        if (files.length >= MAX_FILES) {
          break;
        }
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

export interface SlashCommand {
  name: string;
  description: string;
  takesArg?: boolean;
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
  inputRef?: React.RefObject<HTMLTextAreaElement | null>;
  projectPath: string | undefined;
  commands?: SlashCommand[];
  onCommand?: (name: string, args: string) => void;
}

export function MentionTextarea({ value, onChange, onKeyDown, onAttach, disabled, placeholder, className, autoFocus, inputRef, projectPath, commands, onCommand }: Props) {
  const { t } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const filesCache = useRef<string[] | null>(null);
  const loadingRef = useRef(false);
  const { undo, redo } = useUndoStack(value, onChange);

  const [menu, setMenu] = useState<'file' | 'command' | null>(null);
  const [triggerStart, setTriggerStart] = useState(0);
  const [query, setQuery] = useState('');
  const [files, setFiles] = useState<string[]>([]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const fileSet = useMemo(() => new Set(files), [files]);

  const fileItems = files.filter((f) => !query || f.toLowerCase().includes(query.toLowerCase())).slice(0, MAX_RESULTS);
  const commandItems = (commands ?? []).filter((c) => c.name.toLowerCase().startsWith(query.toLowerCase()));
  const items = menu === 'command' ? commandItems : menu === 'file' ? fileItems : [];

  useEffect(() => {
    itemRefs.current[selectedIndex]?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  const triggerLoad = () => {
    if (!projectPath || filesCache.current || loadingRef.current) {
      return;
    }
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
    const slashMatch = commands?.length ? textBefore.match(/(?:^|\n)\/(\w*)$/) : null;
    const atMatch = textBefore.match(/@([^\s@]*)$/);

    if (slashMatch) {
      setTriggerStart(pos - 1 - slashMatch[1].length);
      if (slashMatch[1] !== query) {
        setSelectedIndex(0);
      }
      setQuery(slashMatch[1]);
      setMenu('command');
    } else if (atMatch) {
      setTriggerStart(pos - atMatch[0].length);
      if (atMatch[1] !== query) {
        setSelectedIndex(0);
      }
      setQuery(atMatch[1]);
      if (menu !== 'file') {
        setMenu('file');
        triggerLoad();
      }
    } else if (menu !== null) {
      setMenu(null);
    }

    onChange(val);
  };

  const replaceTrigger = (text: string): { before: string; after: string } => {
    const before = value.slice(0, triggerStart);
    const after = value.slice(triggerStart + 1 + query.length);
    onChange(`${before}${text}${after}`);
    return { before, after };
  };

  const selectFile = (path: string) => {
    const after = value.slice(triggerStart + 1 + query.length);
    const suffix = after.startsWith(' ') ? after : ` ${after}`;
    const before = value.slice(0, triggerStart);
    onChange(`${before}@${path}${suffix}`);
    setMenu(null);
    const caret = before.length + 1 + path.length + 1;
    requestAnimationFrame(() => {
      const el = textareaRef.current;
      el?.focus();
      el?.setSelectionRange(caret, caret);
    });
  };

  const selectCommand = (cmd: SlashCommand) => {
    setMenu(null);
    if (cmd.takesArg) {
      const insert = `/${cmd.name} `;
      const { before } = replaceTrigger(insert);
      const caret = before.length + insert.length;
      requestAnimationFrame(() => {
        const el = textareaRef.current;
        el?.focus();
        el?.setSelectionRange(caret, caret);
      });
    } else {
      replaceTrigger('');
      onCommand?.(cmd.name, '');
    }
  };

  const trySubmitCommand = (): boolean => {
    if (!commands?.length) {
      return false;
    }
    const m = value.match(/^\/(\w+)(?:\s+([\s\S]*))?$/);
    if (!m) {
      return false;
    }
    const cmd = commands.find((c) => c.name === m[1]);
    if (!cmd) {
      return false;
    }
    onChange('');
    onCommand?.(cmd.name, (m[2] ?? '').trim());
    return true;
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

  const attachPastedImage = (file: File) => {
    const reader = new FileReader();
    reader.onload = () => {
      const base64 = String(reader.result).split(',')[1] ?? '';
      const ext = extForMime(file.type);
      SaveTempImage(base64, ext)
        .then((path) => {
          onAttach ? onAttach(path) : insertAtCursor(path);
        })
        .catch((err) => toastError({ title: t('agents.detail.couldNotPasteImage'), err }));
    };
    reader.onerror = () => toastError({ title: t('agents.detail.couldNotPasteImage'), err: reader.error });
    reader.readAsDataURL(file);
  };

  const handlePaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    e.preventDefault();
    const imageItem = Array.from(e.clipboardData.items).find((it) => it.type.startsWith('image/'));
    const file = imageItem?.getAsFile();
    if (file) {
      attachPastedImage(file);
      return;
    }
    const plainText = e.clipboardData.getData('text/plain');
    PasteClipboardImage()
      .then((path) => {
        onAttach ? onAttach(path) : insertAtCursor(path);
      })
      .catch((err) => {
        if (plainText) {
          insertAtCursor(plainText);
          return;
        }
        toastError({ title: t('agents.detail.couldNotPasteImage'), err });
      });
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key === 'z' && !e.shiftKey) {
      e.preventDefault();
      undo();
      return;
    }
    if ((mod && e.key === 'z' && e.shiftKey) || (mod && e.key === 'y')) {
      e.preventDefault();
      redo();
      return;
    }
    if (menu !== null) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, items.length - 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
        return;
      }
      if (e.key === 'Enter') {
        if (menu === 'command' && commandItems[selectedIndex]) {
          e.preventDefault();
          selectCommand(commandItems[selectedIndex]);
          return;
        }
        if (menu === 'file' && fileItems[selectedIndex]) {
          e.preventDefault();
          selectFile(fileItems[selectedIndex]);
          return;
        }
      }
      if (e.key === 'Escape') {
        e.stopPropagation();
        setMenu(null);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      if (trySubmitCommand()) {
        e.preventDefault();
        return;
      }
      setMenu(null);
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
      {menu !== null && (
        <div className="absolute bottom-full left-0 z-50 mb-1 w-full min-w-[320px] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md">
          <ScrollArea className="max-h-52" viewportProps={{ className: 'max-h-52' }}>
            {items.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">{menu === 'command' ? t('agents.detail.commandNone') : t('agents.detail.mentionNoFiles')}</p>
            ) : menu === 'command' ? (
              <div className="py-1">
                {commandItems.map((cmd, i) => (
                  <button
                    key={cmd.name}
                    ref={(el) => {
                      itemRefs.current[i] = el;
                    }}
                    type="button"
                    className={cn('flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent hover:text-accent-foreground', i === selectedIndex && 'bg-accent text-accent-foreground')}
                    onMouseDown={(e) => {
                      e.preventDefault();
                      selectCommand(cmd);
                    }}
                  >
                    <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="shrink-0 font-mono text-xs">/{cmd.name}</span>
                    <span className="truncate text-xs text-muted-foreground">{cmd.description}</span>
                  </button>
                ))}
              </div>
            ) : (
              <div className="py-1">
                {fileItems.map((path, i) => (
                  <button
                    key={path}
                    ref={(el) => {
                      itemRefs.current[i] = el;
                    }}
                    type="button"
                    className={cn('flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-accent hover:text-accent-foreground', i === selectedIndex && 'bg-accent text-accent-foreground')}
                    onMouseDown={(e) => {
                      e.preventDefault();
                      selectFile(path);
                    }}
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
      <div ref={overlayRef} aria-hidden className="pointer-events-none absolute inset-px overflow-hidden whitespace-pre-wrap break-words rounded-[calc(var(--radius)-1px)] px-3 py-2 text-base md:text-sm">
        {highlightSegments(value, { hidden: true, requireAt: true, isKnownFile: (p) => fileSet.has(p) })}
      </div>
      <Textarea
        ref={(el) => {
          textareaRef.current = el;
          if (inputRef) {
            inputRef.current = el;
          }
        }}
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
