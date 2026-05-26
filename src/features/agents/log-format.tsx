import 'highlight.js/styles/github-dark.css';
import { Check, ChevronDown, ChevronRight, Copy } from 'lucide-react';
import { Fragment, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { highlightSegments } from './file-mentions';

const TIMESTAMP_RE = /^(\[\d{2}:\d{2}:\d{2}\])\s?/;

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

export type LogBlock = LogLineBlock | ThinkingBlock | TextBlock | ToolResultBlock | UserMessageBlock;

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

const TOOL_ID_RE = /^\[#([^\]]+)\]\s?/;

function extractToolId(rest: string): { id: string | null; rest: string } {
  const m = rest.match(TOOL_ID_RE);
  if (!m) {
    return { id: null, rest };
  }
  return { id: m[1], rest: rest.slice(m[0].length) };
}

export function buildLogBlocks(lines: string[]): LogBlock[] {
  const blocks: LogBlock[] = [];
  let thinkingLines: string[] = [];
  let thinkingKey: string | null = null;
  let textLines: string[] = [];
  let textKey: string | null = null;
  let toolResultLines: string[] = [];
  let toolResultKey: string | null = null;
  let prevStamp: string | null = null;
  let blockIndex = 0;

  const flushThinking = () => {
    if (thinkingLines.length > 0 && thinkingKey !== null) {
      blocks.push({ type: 'thinking', lines: thinkingLines, key: thinkingKey });
      thinkingLines = [];
      thinkingKey = null;
    }
  };

  const flushText = () => {
    if (textLines.length > 0 && textKey !== null) {
      const content = textLines.join('\n').trim();
      if (content) {
        blocks.push({ type: 'text', content, key: textKey });
      }
      textLines = [];
      textKey = null;
    }
  };

  let toolResultCurrentId: string | null = null;
  let toolResultCurrentStatus: 'success' | 'error' = 'success';
  const flushToolResult = () => {
    if (toolResultLines.length > 0 && toolResultKey !== null) {
      blocks.push({ type: 'tool-result', lines: toolResultLines, key: toolResultKey, toolId: toolResultCurrentId ?? undefined, status: toolResultCurrentStatus });
      toolResultLines = [];
      toolResultKey = null;
      toolResultCurrentId = null;
      toolResultCurrentStatus = 'success';
    }
  };

  let userMsgLines: string[] = [];
  let userMsgKey: string | null = null;

  const flushUserMsg = () => {
    if (userMsgLines.length > 0 && userMsgKey !== null) {
      blocks.push({ type: 'user-message', content: userMsgLines.join('\n'), key: userMsgKey });
      userMsgLines = [];
      userMsgKey = null;
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const { stamp, rest } = splitLogLine(lines[i]);

    if (rest.startsWith('(thinking) ')) {
      flushText();
      flushToolResult();
      flushUserMsg();
      if (thinkingKey === null) {
        thinkingKey = `thinking-${blockIndex++}-${i}`;
      }
      thinkingLines.push(rest.slice('(thinking) '.length));
      if (stamp) {
        prevStamp = stamp;
      }
    } else if (rest.startsWith('> ') || rest === '>') {
      flushThinking();
      flushText();
      flushToolResult();
      if (userMsgKey === null) {
        userMsgKey = `user-msg-${blockIndex++}-${i}`;
      }
      userMsgLines.push(rest.startsWith('> ') ? rest.slice('> '.length) : '');
      if (stamp) {
        prevStamp = stamp;
      }
    } else if (stamp === null && userMsgLines.length > 0) {
      userMsgLines.push(rest);
    } else if (rest.startsWith('← ') || (rest.startsWith('✗ ') && TOOL_ID_RE.test(rest.slice('✗ '.length)))) {
      // A tagged error result (`✗ [#id] …`) opens an absorbing block just like a
      // success result (`← `), so its (often multi-line) output attaches to the
      // tool call instead of spilling into orphan text blocks. Untagged `✗` lines
      // (e.g. connection errors) fall through to the structured-line branch.
      flushThinking();
      flushText();
      flushToolResult();
      flushUserMsg();
      toolResultKey = `tool-result-${blockIndex++}-${i}`;
      const isError = rest.startsWith('✗ ');
      const { id, rest: body } = extractToolId(rest.slice(2));
      toolResultCurrentId = id;
      toolResultCurrentStatus = isError ? 'error' : 'success';
      toolResultLines.push(body);
      if (stamp) {
        prevStamp = stamp;
      }
    } else if (stamp === null && toolResultLines.length > 0) {
      toolResultLines.push(rest);
    } else if (isStructuredLine(rest)) {
      flushThinking();
      flushText();
      flushToolResult();
      flushUserMsg();
      if (stamp) {
        prevStamp = stamp;
      }
      if (rest.startsWith('[result]') || rest.startsWith('[system]')) {
        continue;
      }
      const show = stamp && stamp !== prevStamp ? stamp : null;
      let lineRest = rest;
      let toolId: string | undefined;
      if (rest.startsWith('→ ') || rest.startsWith('✗ ')) {
        const prefix = rest.slice(0, 2);
        const { id, rest: tail } = extractToolId(rest.slice(2));
        if (id) {
          toolId = id;
          lineRest = prefix + tail;
        }
      }
      blocks.push({ type: 'line', stamp: show, rest: lineRest, key: `${blockIndex++}-${i}-${lines[i].slice(0, 20)}`, toolId });
    } else {
      flushThinking();
      flushToolResult();
      flushUserMsg();
      if (textKey === null) {
        textKey = `text-${blockIndex++}-${i}`;
      }
      textLines.push(rest);
      if (stamp) {
        prevStamp = stamp;
      }
    }
  }

  flushThinking();
  flushText();
  flushToolResult();
  flushUserMsg();

  // Pair tool calls with their results. New logs carry a `[#id]` tag that
  // matches `tool_use.id` ↔ `tool_result.tool_use_id` (handles parallel calls
  // returning out of order). Older logs without IDs fall back to FIFO order.
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
      block.toolStatus = 'pending';
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
          // AskUserQuestion / ExitPlanMode aren't answered by a tool_result but by the
          // panel, so the agent's error result here only means "the turn ended before
          // you replied". Keep them pending instead of flagging a red failure.
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
    } else if (block.type === 'line' && block.rest.startsWith('✗ ')) {
      let call: LogLineBlock | undefined;
      if (block.toolId && callsById.has(block.toolId)) {
        call = callsById.get(block.toolId);
        callsById.delete(block.toolId);
      } else {
        call = pendingFifo.shift();
      }
      if (call) {
        // AskUserQuestion / ExitPlanMode aren't answered by a tool_result but by the
        // panel, so the agent's error result here only means "the turn ended before
        // you replied". Keep them pending instead of flagging a red failure.
        if (call.rest.startsWith('→ AskUserQuestion') || call.rest.startsWith('→ ExitPlanMode')) {
          absorbed.add(i);
        } else {
          attachResult(call, 'error', [stripXmlTags(block.rest.slice('✗ '.length))]);
          absorbed.add(i);
        }
      }
    }
  }

  return blocks.filter((_, i) => !absorbed.has(i));
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
  // For file paths, show only the basename
  const detail = raw.startsWith('/') ? (raw.split('/').pop() ?? raw) : raw;
  return { name, detail };
}

// Grep uses `<n>:` (ripgrep style); Read uses `<n>→` (cat -n with arrow).
const LINE_NUMBER_RE = /^\s*\d+(?:→|:)/;

const TODO_RE = /^\[([ x~])\] /;

function TodoResultPanel({ lines }: { lines: string[] }) {
  return (
    <ScrollArea className="mt-1 max-h-64 rounded border border-border/60 bg-muted/30 text-muted-foreground/80">
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
    <ScrollArea className="mt-1 max-h-64 rounded border border-border/60 bg-muted/30 text-muted-foreground/80">
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
  const dotClass = toolStatus === 'success' ? 'bg-emerald-400' : toolStatus === 'error' ? 'bg-red-400' : 'bg-blue-400 animate-pulse';
  const canExpand = toolResultLines && toolResultLines.length > 0;
  return (
    <Fragment>
      <span className="self-start text-muted-foreground/70 tabular-nums">{stamp ?? ' '}</span>
      <span className="flex min-w-0 flex-col">
        <button type="button" disabled={!canExpand} onClick={() => canExpand && setOpen((v) => !v)} className={cn('flex w-full min-w-0 items-start gap-1.5 text-left', canExpand && 'cursor-pointer')}>
          <span className={cn('mt-1.5 size-1.5 shrink-0 rounded-full', dotClass)} />
          <span className="min-w-0 flex-1 break-words">
            {parsed ? (
              <>
                <span className="text-violet-400">{parsed.name}</span>
                {parsed.detail && <span className="ml-1.5 text-muted-foreground/50">{parsed.detail}</span>}
              </>
            ) : (
              <span className="text-violet-400">{rest.slice('→ '.length)}</span>
            )}
          </span>
          {canExpand && (open ? <ChevronDown className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" /> : <ChevronRight className="mt-0.5 size-3 shrink-0 text-muted-foreground/40" />)}
        </button>
        {open && canExpand && <ToolResultPanel lines={toolResultLines} toolName={parsed?.name} />}
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
      <button
        type="button"
        onClick={copy}
        aria-label="Copy"
        className="absolute right-1.5 top-1.5 rounded bg-white/5 p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-white/10 hover:text-foreground group-hover:opacity-100"
      >
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
  ol: ({ children }) => <ol className="mb-2 list-decimal pl-4 last:mb-0 [&>li]:mb-0.5">{children}</ol>,
  li: ({ children }) => <li className="text-foreground/85">{children}</li>,
  a: ({ href, children }) => (
    <a href={href} className="text-blue-400 underline" target="_blank" rel="noopener noreferrer">
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

export function Markdown({ children }: { children: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={mdComponents}>
      {children}
    </ReactMarkdown>
  );
}

interface LogBlocksGridProps {
  blocks: LogBlock[];
  restClassName?: string;
  preserveWhitespace?: boolean;
}

export function LogBlocksGrid({ blocks, restClassName, preserveWhitespace = true }: LogBlocksGridProps) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 font-mono text-xs leading-relaxed">
      {blocks.map((block) => {
        if (block.type === 'user-message') {
          return (
            <div key={block.key} className="col-span-2 my-1 whitespace-pre-wrap break-words rounded-md bg-muted/50 px-3 py-2 text-sm text-foreground/80">
              {highlightSegments(block.content, { requireAt: true })}
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
        if (block.rest.startsWith('→ ')) {
          return <ToolCallLine key={block.key} stamp={block.stamp} rest={block.rest} toolStatus={block.toolStatus} toolResultLines={block.toolResultLines} />;
        }
        return (
          <Fragment key={block.key}>
            <span className="self-start text-muted-foreground/70 tabular-nums">{block.stamp ?? ' '}</span>
            <span className={cn(preserveWhitespace && 'whitespace-pre-wrap break-words', classifyLogLine(block.rest), restClassName)}>{block.rest}</span>
          </Fragment>
        );
      })}
    </div>
  );
}
