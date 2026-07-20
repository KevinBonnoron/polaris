import { Check, Clipboard, Search, X } from 'lucide-react';
import { useRef, useState } from 'react';
import type { ClipboardEvent, KeyboardEvent, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/badge';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

export interface FilterToken {
  key: string;
  value: string;
}

export interface FilterDef {
  key: string;
  label: string;
  icon?: ReactNode;
  multi?: boolean;
  options: Array<{ value: string; label: string }>;
}

interface Props {
  tokens: FilterToken[];
  onTokensChange: (tokens: FilterToken[]) => void;
  defs: FilterDef[];
  placeholder?: string;
}

function quoteIfNeeded(value: string): string {
  return /[\s:]/.test(value) ? `"${value.replace(/"/g, '\\"')}"` : value;
}

function serializeTokens(tokens: FilterToken[]): string {
  return tokens.map((t) => (t.key === 'term' ? quoteIfNeeded(t.value) : `${t.key}:${quoteIfNeeded(t.value)}`)).join(' ');
}

function parseFilterString(text: string, defs: FilterDef[]): FilterToken[] {
  const parts: string[] = [];
  const re = /(\w+):"((?:[^"\\]|\\.)*)"|(\w+):(\S+)|"((?:[^"\\]|\\.)*)"|(\S+)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m[1] && m[2] !== undefined) {
      parts.push(`${m[1]}:${m[2].replace(/\\"/g, '"')}`);
    } else if (m[3] && m[4]) {
      parts.push(`${m[3]}:${m[4]}`);
    } else if (m[5] !== undefined) {
      parts.push(m[5].replace(/\\"/g, '"'));
    } else if (m[6]) {
      parts.push(m[6]);
    }
  }
  const out: FilterToken[] = [];
  for (const part of parts) {
    const colonIdx = part.indexOf(':');
    if (colonIdx > 0) {
      const key = part.slice(0, colonIdx);
      const value = part.slice(colonIdx + 1);
      if (value && defs.some((d) => d.key === key)) {
        out.push({ key, value });
        continue;
      }
    }
    out.push({ key: 'term', value: part });
  }
  return out;
}

function mergeTokens(existing: FilterToken[], incoming: FilterToken[], defs: FilterDef[]): FilterToken[] {
  let next = [...existing];
  for (const t of incoming) {
    const def = defs.find((d) => d.key === t.key);
    if (def?.multi) {
      if (!next.some((e) => e.key === t.key && e.value === t.value)) {
        next.push(t);
      }
    } else {
      next = [...next.filter((e) => e.key !== t.key), t];
    }
  }
  return next;
}

export function FilterBar({ tokens, onTokensChange, defs, placeholder }: Props) {
  const { t } = useTranslation();
  const [input, setInput] = useState('');
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const anchorRef = useRef<HTMLDivElement>(null);

  const colonIdx = input.indexOf(':');
  const mode = colonIdx === -1 ? 'keys' : 'values';
  const keyPart = colonIdx === -1 ? input : input.slice(0, colonIdx);
  const valuePartRaw = colonIdx === -1 ? '' : input.slice(colonIdx + 1);
  const valuePart = valuePartRaw.toLowerCase();
  const activeDef = mode === 'values' ? defs.find((d) => d.key === keyPart) : null;

  const selectedKeys = new Set(tokens.filter((t) => t.key !== 'term').map((t) => t.key));
  const keySuggestions = defs.filter((d) => {
    if (d.options.length === 0) return false;
    if (!d.multi && selectedKeys.has(d.key)) return false;
    return !keyPart || d.key.startsWith(keyPart.toLowerCase()) || d.label.toLowerCase().startsWith(keyPart.toLowerCase());
  });
  const selectedValues = activeDef?.multi ? new Set(tokens.filter((t) => t.key === activeDef.key).map((t) => t.value)) : new Set<string>();
  const valueSuggestions = activeDef ? activeDef.options.filter((o) => !selectedValues.has(o.value) && (!valuePart || o.value.toLowerCase().includes(valuePart) || o.label.toLowerCase().includes(valuePart))) : [];

  const hasContent = mode === 'keys' ? keySuggestions.length > 0 : true;
  const showPopover = open && hasContent && defs.length > 0;

  const addToken = (key: string, value: string) => {
    const def = defs.find((d) => d.key === key);
    if (def?.multi) {
      if (!tokens.some((t) => t.key === key && t.value === value)) {
        onTokensChange([...tokens, { key, value }]);
      }
    } else {
      onTokensChange([...tokens.filter((t) => t.key !== key), { key, value }]);
    }
    setInput('');
    setOpen(false);
    inputRef.current?.focus();
  };

  const removeToken = (key: string, value: string) => {
    onTokensChange(tokens.filter((t) => !(t.key === key && t.value === value)));
    inputRef.current?.focus();
  };

  const selectKey = (key: string) => {
    setInput(`${key}:`);
    setOpen(true);
    inputRef.current?.focus();
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Backspace' && input === '' && tokens.length > 0) {
      const last = tokens[tokens.length - 1];
      removeToken(last.key, last.value);
    }
    if (e.key === 'Escape') {
      setOpen(false);
      setInput('');
    }
    if (e.key === 'Enter' && mode === 'keys' && input.trim()) {
      addToken('term', input.trim());
    }
    if (e.key === ' ' && mode === 'values' && valuePartRaw.trim()) {
      e.preventDefault();
      addToken(keyPart, valuePartRaw.trim());
    }
  };

  const handlePaste = (e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text');
    const hasKnownKey = text.split(/\s+/).some((p) => {
      const idx = p.indexOf(':');
      return idx > 0 && defs.some((d) => d.key === p.slice(0, idx));
    });
    if (hasKnownKey) {
      e.preventDefault();
      const parsed = parseFilterString(text, defs);
      onTokensChange(mergeTokens(tokens, parsed, defs));
      setInput('');
      setOpen(false);
    }
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(serializeTokens(tokens));
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard unavailable (permissions or insecure context)
    }
  };

  const tokenLabel = (token: FilterToken) => {
    const def = defs.find((d) => d.key === token.key);
    const opt = def?.options.find((o) => o.value === token.value);
    return opt?.label ?? token.value;
  };

  const handleContainerMouseDown = (e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('button')) return;
    e.preventDefault();
    setOpen(true);
    inputRef.current?.focus();
  };

  return (
    <Popover
      open={showPopover}
      onOpenChange={(v) => {
        if (!v) setOpen(false);
      }}
    >
      <PopoverAnchor asChild>
        <div ref={anchorRef} className={cn('flex min-h-9 w-full flex-wrap items-center gap-1.5 rounded-md border border-input bg-background px-3 py-1.5 text-sm', 'focus-within:border-ring focus-within:outline-none', 'cursor-text')} onMouseDown={handleContainerMouseDown}>
          {tokens.map((token) => {
            const def = defs.find((d) => d.key === token.key);
            const isTerm = token.key === 'term';
            return (
              <Badge key={`${token.key}:${token.value}`} variant="secondary" className="gap-1 px-1.5 py-0.5 text-xs font-normal">
                {isTerm ? <Search className="size-3 opacity-60" /> : def?.icon && <span className="opacity-60">{def.icon}</span>}
                {!isTerm && <span className="text-muted-foreground">{def?.label ?? token.key}:</span>}
                <span>{tokenLabel(token)}</span>
                <button
                  type="button"
                  aria-label={t('filterBar.removeToken', { label: tokenLabel(token) })}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    removeToken(token.key, token.value);
                  }}
                  className="ml-0.5 rounded hover:text-foreground"
                >
                  <X className="size-3" />
                </button>
              </Badge>
            );
          })}
          <input
            ref={inputRef}
            value={input}
            onChange={(e) => {
              setInput(e.target.value);
              setOpen(true);
            }}
            onFocus={() => setOpen(true)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            aria-label={placeholder ?? t('common.filter')}
            placeholder={tokens.length === 0 ? (placeholder ?? t('common.filter')) : undefined}
            className="min-w-24 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
          />
          {tokens.length > 0 && (
            <button
              type="button"
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                void handleCopy();
              }}
              aria-label={t('filterBar.copyFilters')}
              title={t('filterBar.copyFilters')}
              className="ml-auto shrink-0 rounded p-0.5 text-muted-foreground hover:text-foreground"
            >
              {copied ? <Check className="size-3.5 text-emerald-500" /> : <Clipboard className="size-3.5" />}
            </button>
          )}
        </div>
      </PopoverAnchor>

      <PopoverContent
        className="w-[var(--radix-popover-anchor-width)] p-1"
        align="start"
        onOpenAutoFocus={(e) => e.preventDefault()}
        onInteractOutside={(e) => {
          if (anchorRef.current?.contains(e.target as Node)) {
            e.preventDefault();
            return;
          }
          setOpen(false);
        }}
      >
        {mode === 'keys' && (
          <>
            {!input && <p className="px-2 pb-1 pt-0.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{t('filterBar.filterBy')}</p>}
            {keySuggestions.length > 0 ? (
              <ul className="flex flex-col gap-0.5">
                {keySuggestions.map((def) => (
                  <li key={def.key}>
                    <button
                      type="button"
                      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-accent"
                      onMouseDown={(e) => {
                        e.preventDefault();
                        selectKey(def.key);
                      }}
                    >
                      {def.icon && <span className="text-muted-foreground">{def.icon}</span>}
                      <span className="font-medium">{def.label}</span>
                      <span className="ml-auto font-mono text-xs text-muted-foreground">{def.key}:</span>
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="px-2 py-1.5 text-xs text-muted-foreground">{t('common.noResults')}</p>
            )}
            {!input && <p className="px-2 pb-0.5 pt-2 text-[11px] text-muted-foreground">{t('filterBar.hint')}</p>}
          </>
        )}
        {mode === 'values' && activeDef && (
          <ul className="flex flex-col gap-0.5">
            {valueSuggestions.length === 0 ? (
              <li className="px-2 py-1.5 text-xs text-muted-foreground">{t('common.noResults')}</li>
            ) : (
              valueSuggestions.map((opt) => (
                <li key={opt.value}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-accent"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      addToken(activeDef.key, opt.value);
                    }}
                  >
                    <span>{opt.label}</span>
                    {opt.label !== opt.value && <span className="ml-auto text-xs text-muted-foreground">{opt.value}</span>}
                  </button>
                </li>
              ))
            )}
          </ul>
        )}
        {mode === 'values' && !activeDef && <p className="px-2 py-1.5 text-xs text-muted-foreground">{t('integrations.repository.unknownFilter', { key: keyPart })}</p>}
      </PopoverContent>
    </Popover>
  );
}
