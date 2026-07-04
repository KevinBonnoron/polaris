import 'highlight.js/styles/github-dark.css';
import { Check, ChevronDown, ChevronRight, Copy } from 'lucide-react';
import { Fragment, memo, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import type { polaris } from '@/wailsjs/go/models';
import { BrowserOpenURL } from '@/wailsjs/runtime/runtime';
import { UserMessageContent } from './user-message';

type StreamEvent = polaris.StreamEvent;

const TIMESTAMP_RE = /^(\[\d{2}:\d{2}:\d{2}\])\s?/;

// Emitted by the backend (runner_status.go) as a system event when the user
// kills the run; used to fail any tool calls / sub-agents left in flight.
const STOP_MARKER = '(stopped by user)';

function isStructuredLine(rest: string): boolean {
  const t = rest.trimStart();
  return t.startsWith('→ ') || t.startsWith('← ') || t.startsWith('(thinking) ') || t.startsWith('[system]') || t.startsWith('[result]') || t.startsWith('>') || /^(✓|✔|✅|✗|✘|❌)/.test(t);
}

function classifyLogLine(line: string): string {
  const trimmed = line.trimStart();
  if (!trimmed) {
    return '';
  }
  if (trimmed.startsWith('→ ')) {
    return 'text-violet-400';
  }
  if (trimmed.startsWith('← ')) {
    return 'text-cyan-400';
  }
  if (trimmed.startsWith('>')) {
    return 'text-purple-400';
  }
  if (/^(✓|✔|✅)/.test(trimmed)) {
    return 'text-emerald-400';
  }
  if (/^(✗|✘|❌|error|err|failed|fatal)/i.test(trimmed)) {
    return 'text-red-400';
  }
  if (/^(warn|warning)/i.test(trimmed)) {
    return 'text-amber-400';
  }
  if (/^\[(result|system)\]/.test(trimmed)) {
    return 'text-muted-foreground';
  }
  if (/^(starting|running|refactoring|analyzing|building|installing|fetching|loading)/i.test(trimmed)) {
    return 'text-blue-400';
  }
  return 'text-foreground/85';
}

type LogLineParts = { stamp: string | null; rest: string };

function splitLogLine(line: string): LogLineParts {
  const match = line.match(TIMESTAMP_RE);
  if (!match) {
    return { stamp: null, rest: line };
  }
  return { stamp: match[1], rest: line.slice(match[0].length) };
}

type LogLineBlock = {
  type: 'line';
  stamp: string | null;
  rest: string;
  key: string;
  toolStatus?: 'success' | 'error' | 'pending';
  toolResultLines?: string[];
  toolId?: string;
};

type ThinkingBlock = {
  type: 'thinking';
  lines: string[];
  key: string;
};

type TextBlock = {
  type: 'text';
  content: string;
  key: string;
};

type ToolResultBlock = {
  type: 'tool-result';
  lines: string[];
  key: string;
  toolId?: string;
  status?: 'success' | 'error';
};

type UserMessageBlock = {
  type: 'user-message';
  content: string;
  key: string;
};

type AgentGroupBlock = {
  type: 'agent-group';
  description: string;
  subagentType?: string;
  children: LogBlock[];
  key: string;
  toolStatus: 'pending' | 'success' | 'error';
  toolId?: string;
  resultLines?: string[];
};

type CompactBlock = {
  type: 'compact';
  summary: string;
  key: string;
};

export type LogBlock = LogLineBlock | ThinkingBlock | TextBlock | ToolResultBlock | UserMessageBlock | AgentGroupBlock | CompactBlock;

function stripXmlTags(text: string): string {
  return text.replace(/<\/?[a-z_]+>/gi, '').trim();
}

// Like stripXmlTags but keeps leading whitespace so code/file indentation in
// tool-result previews is preserved; only surrounding blank lines are dropped.
function cleanResultLines(lines: string[]): string[] {
  const stripped = lines.map((l) => l.replace(/<\/?[a-z_]+>/gi, ''));
  let start = 0;
  let end = stripped.length;
  while (start < end && stripped[start].trim() === '') {
    start++;
  }
  while (end > start && stripped[end - 1].trim() === '') {
    end--;
  }
  return stripped.slice(start, end);
}

// Tracks whether a single-line, CSS-truncated element is actually clipping its
// text, so callers can offer an expand affordance only when there's more to show.
function useOverflow<T extends HTMLElement>(dep: unknown): [React.RefObject<T | null>, boolean] {
  const ref = useRef<T>(null);
  const [clipped, setClipped] = useState(false);
  // biome-ignore lint/correctness/useExhaustiveDependencies: dep is a re-measure trigger (its value drives the rendered text, read via ref)
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) {
      setClipped(false);
      return;
    }
    const measure = () => setClipped(el.scrollWidth > el.clientWidth + 1);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [dep]);
  return [ref, clipped];
}

const TOOL_ID_RE = /^\[#([^\]]+)\]\s?/;

function extractToolId(rest: string): { id: string | null; rest: string } {
  const m = rest.match(TOOL_ID_RE);
  if (!m) {
    return { id: null, rest };
  }
  return { id: m[1], rest: rest.slice(m[0].length) };
}

export function buildLogBlocks(events: StreamEvent[]): LogBlock[] {
  const out: LogBlock[] = [];
  let blockIndex = 0;
  let textLines: string[] = [];
  let textKey: string | null = null;
  let thinkingLines: string[] = [];
  let thinkingKey: string | null = null;

  // Stack for nesting sub-agent events under their Agent tool call
  const agentStack: Array<{ id: string; block: AgentGroupBlock }> = [];
  // Set when the run was killed: in-flight tool calls / sub-agents get no
  // result event, so they'd otherwise stay stuck "pending" (blue) forever.
  let stopped = false;

  function target(): LogBlock[] {
    return agentStack.length > 0 ? agentStack[agentStack.length - 1].block.children : out;
  }

  const flushText = () => {
    if (!textKey || !textLines.length) {
      return;
    }
    const content = textLines.join('\n').trim();
    if (content) {
      target().push({ type: 'text', content, key: textKey });
    }
    textLines = [];
    textKey = null;
  };
  const flushThinking = () => {
    if (!thinkingKey || !thinkingLines.length) {
      return;
    }
    target().push({ type: 'thinking', lines: thinkingLines, key: thinkingKey });
    thinkingLines = [];
    thinkingKey = null;
  };

  for (let i = 0; i < events.length; i++) {
    const evt = events[i];
    switch (evt.type) {
      case 'text':
        flushThinking();
        if (!textKey) {
          textKey = `text-${blockIndex++}-${i}`;
        }
        textLines.push(evt.content ?? '');
        break;
      case 'thinking':
        flushText();
        if (!thinkingKey) {
          thinkingKey = `thinking-${blockIndex++}-${i}`;
        }
        thinkingLines.push(evt.content ?? '');
        break;
      case 'tool_call': {
        flushText();
        flushThinking();
        if (evt.name === 'Agent' && evt.id) {
          const description = typeof evt.input?.description === 'string' ? evt.input.description : '';
          const agentBlock: AgentGroupBlock = {
            type: 'agent-group',
            description,
            subagentType: typeof evt.input?.subagent_type === 'string' ? evt.input.subagent_type : undefined,
            children: [],
            key: `agent-group-${blockIndex++}-${i}`,
            toolStatus: 'pending',
            toolId: evt.id,
          };
          target().push(agentBlock);
          agentStack.push({ id: evt.id, block: agentBlock });
        } else {
          const rest = '→ ' + (evt.name ?? 'Tool') + (evt.content ? ' · ' + evt.content : '');
          target().push({ type: 'line', stamp: evt.ts ?? null, rest, key: `tool-${blockIndex++}-${i}`, toolId: evt.id, toolStatus: 'pending' });
        }
        break;
      }
      case 'tool_result': {
        flushText();
        flushThinking();
        // Check if this closes an active agent group
        if (evt.id) {
          const agentIdx = agentStack.findIndex((a) => a.id === evt.id);
          if (agentIdx !== -1) {
            const agent = agentStack[agentIdx];
            agent.block.toolStatus = evt.error ? 'error' : 'success';
            const resultText = evt.rendered_content || evt.content || '';
            if (resultText) {
              agent.block.resultLines = resultText.split('\n');
            }
            agentStack.splice(agentIdx, 1);
            break;
          }
        }
        const text = evt.rendered_content || evt.content || '';
        target().push({ type: 'tool-result', lines: text ? text.split('\n') : [], key: `result-${blockIndex++}-${i}`, toolId: evt.id ?? undefined, status: evt.error ? 'error' : 'success' });
        break;
      }
      case 'user_message':
        flushText();
        flushThinking();
        // A human message interrupts any in-flight sub-agents: mark them killed
        // and stop nesting so the message lands at the top level.
        for (const a of agentStack) {
          a.block.toolStatus = 'error';
        }
        agentStack.length = 0;
        if (evt.content) {
          out.push({ type: 'user-message', content: evt.content, key: `user-${blockIndex++}-${i}` });
        }
        break;
      case 'system': {
        flushText();
        flushThinking();
        const content = evt.content ?? '';
        if (content.includes(STOP_MARKER)) {
          // Killed run: fail any open sub-agents and stop nesting so the notice
          // lands at the top level (mirrors the user_message interrupt).
          stopped = true;
          for (const a of agentStack) {
            a.block.toolStatus = 'error';
          }
          agentStack.length = 0;
        }
        if (content) {
          target().push({ type: 'line', stamp: evt.ts ?? null, rest: content, key: `sys-${blockIndex++}-${i}` });
        }
        break;
      }
      case 'compact': {
        flushText();
        flushThinking();
        const summary = evt.content ?? '';
        if (summary) {
          out.push({ type: 'compact', summary, key: `compact-${blockIndex++}-${i}` });
        }
        break;
      }
      // turn_end: skip
    }
  }

  flushText();
  flushThinking();

  // Pair tool_call events with tool_result events by ID, recursively within agent groups.
  function pairToolResults(blocks: LogBlock[]): LogBlock[] {
    const absorbed = new Set<number>();
    const callsById = new Map<string, LogLineBlock>();
    const pendingFifo: LogLineBlock[] = [];

    const attachResult = (call: LogLineBlock, status: 'success' | 'error', resultLines: string[]) => {
      call.toolStatus = status;
      call.toolResultLines = resultLines;
    };

    for (let i = 0; i < blocks.length; i++) {
      const block = blocks[i];
      if (block.type === 'line' && block.rest.startsWith('→ ')) {
        if (block.toolId) {
          callsById.set(block.toolId, block);
        } else {
          pendingFifo.push(block);
        }
      } else if (block.type === 'tool-result') {
        let call: LogLineBlock | undefined;
        if (block.toolId && callsById.has(block.toolId)) {
          call = callsById.get(block.toolId);
          callsById.delete(block.toolId);
        } else {
          call = pendingFifo.shift();
        }
        if (call) {
          if (block.status === 'error') {
            // AskUserQuestion / ExitPlanMode get no tool_result from the agent (subprocess
            // was stopped to wait for user input), so an error result here means "turn
            // ended before you answered". Keep them pending rather than flagging failure.
            if (call.rest.startsWith('→ AskUserQuestion') || call.rest.startsWith('→ ExitPlanMode')) {
              absorbed.add(i);
            } else {
              attachResult(call, 'error', cleanResultLines(block.lines));
              absorbed.add(i);
            }
          } else {
            attachResult(call, 'success', cleanResultLines(block.lines));
            absorbed.add(i);
          }
        }
      } else if (block.type === 'agent-group') {
        block.children = pairToolResults(block.children);
      }
    }

    return blocks.filter((_, i) => !absorbed.has(i));
  }

  const paired = pairToolResults(out);
  if (stopped) {
    failPendingBlocks(paired);
  }
  return paired;
}

// After a killed run, any block still "pending" never received its result;
// flag it as an error, recursing into sub-agent groups. AskUserQuestion /
// ExitPlanMode stay pending — those legitimately wait on the user.
function failPendingBlocks(blocks: LogBlock[]): void {
  for (const b of blocks) {
    if (b.type === 'agent-group') {
      if (b.toolStatus === 'pending') {
        b.toolStatus = 'error';
      }
      failPendingBlocks(b.children);
    } else if (b.type === 'line' && b.toolStatus === 'pending' && b.rest.startsWith('→ ') && !b.rest.startsWith('→ AskUserQuestion') && !b.rest.startsWith('→ ExitPlanMode')) {
      b.toolStatus = 'error';
    }
  }
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Sub-agent task descriptions usually restate the agent type as their leading
// word (e.g. "Explore …" for an Explore agent). Drop that echo so the type,
// shown as a tag, isn't repeated in the description prose right after it.
function stripLeadingType(description: string, type?: string): string {
  if (!type) {
    return description;
  }
  const stripped = description.replace(new RegExp(`^${escapeRegExp(type)}\\b[\\s:—-]*`, 'i'), '').trim();
  return stripped || description;
}

function AgentGroup({ description, subagentType, children, resultLines, toolStatus }: Omit<AgentGroupBlock, 'type' | 'key'>) {
  const [open, setOpen] = useState(false);
  const type = subagentType?.trim();
  const desc = stripLeadingType(description, type);
  const [descRef, descClipped] = useOverflow<HTMLSpanElement>(desc);

  const dotClass = toolStatus === 'success' ? 'bg-emerald-400' : toolStatus === 'error' ? 'bg-red-400' : 'bg-blue-400 animate-pulse';
  const hasChildren = children.length > 0;
  const hasResult = (resultLines?.length ?? 0) > 0;
  const canExpand = hasChildren || hasResult || descClipped;

  return (
    <div className="col-start-2 my-0.5 min-w-0">
      <button type="button" onClick={() => canExpand && setOpen((v) => !v)} className={cn('flex w-full min-w-0 items-start gap-1.5 text-left', canExpand && 'cursor-pointer')}>
        <span className={cn('mt-1.5 size-1.5 shrink-0 rounded-full', dotClass)} />
        <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
          {type ? <span className="shrink-0 rounded-sm bg-violet-400/10 px-1 text-violet-300">{type}</span> : <span className="shrink-0 text-violet-400">Agent</span>}
          {desc && (
            <span ref={descRef} className="min-w-0 truncate text-muted-foreground/50">
              {desc}
            </span>
          )}
        </span>
        {canExpand && (open ? <ChevronDown className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" /> : <ChevronRight className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" />)}
      </button>
      {open && canExpand && (
        <div className="ml-1.5 mt-0.5 border-l border-border/30 pl-2">
          {descClipped && <div className="mb-0.5 whitespace-pre-wrap break-words text-muted-foreground/70">{desc}</div>}
          {hasChildren && <LogBlocksGrid blocks={children} showTimestamps={false} compact />}
          {hasResult && <ToolResultPanel lines={cleanResultLines(resultLines!)} toolName="Agent" />}
        </div>
      )}
    </div>
  );
}

function ThinkingGroup({ lines }: { lines: string[] }) {
  const [open, setOpen] = useState(false);
  const content = lines.join('\n');
  return (
    <div className="col-span-2 my-0.5">
      <button type="button" onClick={() => setOpen((v) => !v)} className="flex items-center gap-1 text-muted-foreground/60 transition-colors hover:text-muted-foreground">
        {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        <span className="italic">{open ? 'Thinking' : `Thinking (${lines.length} block${lines.length > 1 ? 's' : ''})`}</span>
      </button>
      {open && (
        <div className="mt-1 border-l border-border pl-3 text-sm italic text-muted-foreground/70 [&_*]:font-sans [&_code]:font-mono [&_pre]:not-italic [&_pre_*]:not-italic">
          <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={mdComponents}>
            {content}
          </ReactMarkdown>
        </div>
      )}
    </div>
  );
}

function parseToolCall(rest: string): { name: string; detail: string } | null {
  if (!rest.startsWith('→ ')) {
    return null;
  }
  const body = rest.slice('→ '.length);
  const dotIdx = body.indexOf(' · ');
  if (dotIdx === -1) {
    return { name: body, detail: '' };
  }
  const name = body.slice(0, dotIdx);
  const raw = body.slice(dotIdx + 3);
  // For absolute file paths (no spaces), show only the basename
  const detail = raw.startsWith('/') && !raw.includes(' ') ? (raw.split('/').pop() ?? raw) : raw;
  return { name, detail };
}

// Grep uses `<n>:` (ripgrep style); Read uses `<n>→` (cat -n with arrow).
const LINE_NUMBER_RE = /^\s*\d+(?:→|:)/;

const TODO_RE = /^\[([ x~])\] /;

function TodoResultPanel({ lines }: { lines: string[] }) {
  return (
    <ScrollArea className="mt-1 rounded border border-border/60 bg-muted/30 text-muted-foreground/80" viewportProps={{ className: 'max-h-64' }}>
      <div className="flex flex-col gap-0.5 px-2 py-1.5 text-[11px] leading-relaxed">
        {lines.map((l, i) => {
          const m = l.match(TODO_RE);
          if (!m) {
            return <span key={i}>{l}</span>;
          }

          const marker = m[1];
          const text = l.slice(m[0].length);
          return (
            // biome-ignore lint/suspicious/noArrayIndexKey: todo lines have no stable id
            <span key={i} className="flex items-start gap-1.5">
              <span className={cn('mt-px size-3 shrink-0 rounded-sm border', marker === 'x' && 'border-emerald-500 bg-emerald-500/20', marker === '~' && 'border-blue-400 bg-blue-400/20 animate-pulse', marker === ' ' && 'border-muted-foreground/40')} />
              <span className={cn(marker === 'x' && 'text-muted-foreground/50 line-through', marker === '~' && 'text-blue-400', marker === ' ' && 'text-muted-foreground/70')}>{text}</span>
            </span>
          );
        })}
      </div>
    </ScrollArea>
  );
}

function ToolResultPanel({ lines, toolName }: { lines: string[]; toolName?: string }) {
  const isDiff = lines.some((l) => l.startsWith('+ ') || l.startsWith('- '));
  const isTodo = toolName === 'TodoWrite' && lines.some((l) => TODO_RE.test(l));
  const stripLineNumbers = toolName === 'Grep' || toolName === 'Read';
  const display = stripLineNumbers ? lines.map((l) => l.replace(LINE_NUMBER_RE, '')) : lines;

  if (isTodo) {
    return <TodoResultPanel lines={lines} />;
  }

  return (
    <ScrollArea className="mt-1 rounded border border-border/60 bg-muted/30 text-muted-foreground/80" viewportProps={{ className: 'max-h-64' }}>
      <pre className="whitespace-pre-wrap break-words px-2 py-1.5 font-mono text-[11px] leading-relaxed">
        {isDiff
          ? display.map((l, i) => (
              <span
                // biome-ignore lint/suspicious/noArrayIndexKey: diff lines have no stable id
                key={i}
                className={cn(l.startsWith('+ ') && 'text-emerald-400', l.startsWith('- ') && 'text-red-400')}
              >
                {l}
                {'\n'}
              </span>
            ))
          : display.join('\n')}
      </pre>
    </ScrollArea>
  );
}

function ToolCallLine({ stamp, rest, toolStatus, toolResultLines }: { stamp: string | null; rest: string; toolStatus?: 'success' | 'error' | 'pending'; toolResultLines?: string[] }) {
  const [open, setOpen] = useState(false);
  const parsed = parseToolCall(rest);
  const detail = parsed ? parsed.detail : rest.slice('→ '.length);
  // The detail carries the full, backend-translated text; the collapsed line
  // clips it with a CSS ellipsis, so only offer expansion when it's truncated.
  const [detailRef, detailClipped] = useOverflow<HTMLSpanElement>(detail);
  const dotClass = toolStatus === 'success' ? 'bg-emerald-400' : toolStatus === 'error' ? 'bg-red-400' : 'bg-blue-400 animate-pulse';
  const hasResult = (toolResultLines?.length ?? 0) > 0;
  const canExpand = hasResult || detailClipped;
  return (
    <Fragment>
      <span className="self-start text-muted-foreground/70 tabular-nums">{stamp ?? ' '}</span>
      <span className="flex min-w-0 flex-col">
        <button type="button" disabled={!canExpand} onClick={() => canExpand && setOpen((v) => !v)} className={cn('flex w-full min-w-0 items-start gap-1.5 text-left', canExpand && 'cursor-pointer')}>
          <span className={cn('mt-1.5 size-1.5 shrink-0 rounded-full', dotClass)} />
          <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
            {parsed ? (
              <>
                <span className="shrink-0 text-violet-400">{parsed.name}</span>
                {detail && (
                  <span ref={detailRef} className="min-w-0 truncate text-muted-foreground/50">
                    {detail}
                  </span>
                )}
              </>
            ) : (
              <span ref={detailRef} className="min-w-0 truncate text-violet-400">
                {detail}
              </span>
            )}
          </span>
          {canExpand && (open ? <ChevronDown className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" /> : <ChevronRight className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" />)}
        </button>
        {open && detailClipped && (
          <ScrollArea className="mt-1 rounded border border-border/60 bg-muted/30 text-muted-foreground/80" viewportProps={{ className: 'max-h-64' }}>
            <pre className="whitespace-pre-wrap break-words px-2 py-1.5 font-mono text-[11px] leading-relaxed text-foreground/70">{detail}</pre>
          </ScrollArea>
        )}
        {open && toolResultLines && toolResultLines.length > 0 && <ToolResultPanel lines={toolResultLines} toolName={parsed?.name} />}
      </span>
    </Fragment>
  );
}

function CodeBlock({ children }: { children: React.ReactNode }) {
  const ref = useRef<HTMLPreElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = () => {
    const text = ref.current?.textContent ?? '';
    if (!text) {
      return;
    }
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <div className="group relative mb-2 last:mb-0">
      <pre ref={ref} className="overflow-x-auto rounded bg-[#0d1117] p-3 font-mono text-xs">
        {children}
      </pre>
      <button type="button" onClick={copy} aria-label="Copy" className="absolute right-1.5 top-1.5 rounded bg-white/5 p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-white/10 hover:text-foreground group-hover:opacity-100">
        {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
      </button>
    </div>
  );
}

const mdComponents: React.ComponentProps<typeof ReactMarkdown>['components'] = {
  p: ({ children }) => <p className="mb-2 last:mb-0 text-foreground/85">{children}</p>,
  h1: ({ children }) => <h1 className="mb-2 text-base font-semibold text-foreground">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-1.5 text-sm font-semibold text-foreground">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1 text-sm font-semibold text-foreground">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1 text-xs font-semibold text-foreground">{children}</h4>,
  h5: ({ children }) => <h5 className="mb-1 text-xs font-semibold text-foreground">{children}</h5>,
  h6: ({ children }) => <h6 className="mb-1 text-xs font-semibold text-foreground">{children}</h6>,
  ul: ({ children }) => <ul className="mb-2 list-disc pl-4 last:mb-0 [&>li]:mb-0.5">{children}</ul>,
  ol: ({ children }) => <ol className="mb-2 list-decimal pl-6 last:mb-0 [&>li]:mb-0.5">{children}</ol>,
  li: ({ children }) => <li className="text-foreground/85">{children}</li>,
  a: ({ href, children }) => (
    <a
      href={href}
      className="text-blue-400 underline"
      target="_blank"
      rel="noopener noreferrer"
      onClick={(e) => {
        if (href) {
          e.preventDefault();
          BrowserOpenURL(href);
        }
      }}
    >
      {children}
    </a>
  ),
  code: ({ className, children, ...props }) => {
    const isBlock = className?.includes('language-') || className?.includes('hljs');
    if (isBlock) {
      return (
        <code className={cn('font-mono text-xs', className)} {...props}>
          {children}
        </code>
      );
    }
    return (
      <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground/90" {...props}>
        {children}
      </code>
    );
  },
  pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
  blockquote: ({ children }) => <blockquote className="mb-2 border-l-2 border-border pl-3 text-muted-foreground last:mb-0">{children}</blockquote>,
  hr: () => <hr className="my-2 border-border" />,
  strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
  em: ({ children }) => <em className="italic">{children}</em>,
};

export const Markdown = memo(function Markdown({ children }: { children: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={mdComponents}>
      {children}
    </ReactMarkdown>
  );
});

interface LogBlocksGridProps {
  blocks: LogBlock[];
  restClassName?: string;
  preserveWhitespace?: boolean;
  showTimestamps?: boolean;
  compact?: boolean;
}

export function LogBlocksGrid({ blocks, restClassName, preserveWhitespace = true, showTimestamps = false, compact = false }: LogBlocksGridProps) {
  const { t } = useTranslation();
  return (
    <div className={cn('grid grid-cols-[auto_minmax(0,1fr)] font-mono text-xs leading-relaxed', compact ? 'gap-x-1.5' : 'gap-x-3')}>
      {blocks.map((block) => {
        if (block.type === 'agent-group') {
          return <AgentGroup key={block.key} description={block.description} subagentType={block.subagentType} children={block.children} toolStatus={block.toolStatus} toolId={block.toolId} resultLines={block.resultLines} />;
        }
        if (block.type === 'user-message') {
          return <UserMessageContent key={block.key} content={block.content} />;
        }
        if (block.type === 'compact') {
          return (
            <div key={block.key} className="col-span-2 my-3 rounded-md border border-border/50 bg-muted/30 px-3 py-2">
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{t('agents.detail.compactedLabel')}</p>
              <p className="whitespace-pre-wrap text-xs text-foreground/80">{block.summary}</p>
            </div>
          );
        }
        if (block.type === 'thinking') {
          return <ThinkingGroup key={block.key} lines={block.lines} />;
        }
        if (block.type === 'tool-result') {
          return null;
        }
        if (block.type === 'text') {
          return (
            <div key={block.key} className="col-span-2 my-1 text-sm leading-relaxed [&_*]:font-sans [&_code]:font-mono">
              <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={mdComponents}>
                {block.content}
              </ReactMarkdown>
            </div>
          );
        }
        const stamp = showTimestamps ? (block.stamp ?? null) : null;
        if (block.rest.startsWith('→ ')) {
          return <ToolCallLine key={block.key} stamp={stamp} rest={block.rest} toolStatus={block.toolStatus} toolResultLines={block.toolResultLines} />;
        }
        return (
          <Fragment key={block.key}>
            <span className="self-start text-muted-foreground/70 tabular-nums">{stamp ?? ' '}</span>
            <span className={cn(preserveWhitespace && 'whitespace-pre-wrap break-words', classifyLogLine(block.rest), restClassName)}>{block.rest}</span>
          </Fragment>
        );
      })}
    </div>
  );
}
