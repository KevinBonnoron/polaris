import { describe, expect, it } from 'bun:test';
import { buildLogBlocks } from './log-format';

type StreamEvent = {
  type: string;
  ts?: string;
  content?: string;
  id?: string;
  name?: string;
  input?: Record<string, unknown>;
  error?: boolean;
  rendered_content?: string;
  status?: string;
  tokens?: number;
  cost_usd?: number;
};

const evt = (type: string, partial: Partial<StreamEvent> = {}): StreamEvent => ({
  type,
  ts: '12:00:00',
  ...partial,
});

describe('buildLogBlocks', () => {
  it('text event → TextBlock', () => {
    const blocks = buildLogBlocks([evt('text', { content: 'hello world' })] as any);
    const text = blocks.find((b) => b.type === 'text');
    expect(text).toBeDefined();
    expect((text as any).content).toBe('hello world');
  });

  it('thinking event → ThinkingBlock', () => {
    const blocks = buildLogBlocks([evt('thinking', { content: 'reasoning...' })] as any);
    const thinking = blocks.find((b) => b.type === 'thinking');
    expect(thinking).toBeDefined();
    expect((thinking as any).lines).toContain('reasoning...');
  });

  it('consecutive thinking events merge into one ThinkingBlock', () => {
    const blocks = buildLogBlocks([
      evt('thinking', { content: 'step 1' }),
      evt('thinking', { content: 'step 2' }),
    ] as any);
    const thinkings = blocks.filter((b) => b.type === 'thinking');
    expect(thinkings.length).toBe(1);
    expect((thinkings[0] as any).lines).toEqual(['step 1', 'step 2']);
  });

  it('tool_call + tool_result → ToolCallLine with result attached', () => {
    const blocks = buildLogBlocks([
      evt('tool_call', { id: 'id1', name: 'Edit', content: 'foo.go' }),
      evt('tool_result', { id: 'id1', content: 'ok' }),
    ] as any);
    const call = blocks.find((b) => b.type === 'line' && (b as any).rest.startsWith('→ Edit'));
    expect(call).toBeDefined();
    expect((call as any).toolStatus).toBe('success');
    expect((call as any).toolResultLines).toBeDefined();
    const absorbed = blocks.find((b) => b.type === 'tool-result');
    expect(absorbed).toBeUndefined();
  });

  it('parallel tool calls paired by ID not FIFO', () => {
    const blocks = buildLogBlocks([
      evt('tool_call', { id: 'a', name: 'Read', content: 'a.go' }),
      evt('tool_call', { id: 'b', name: 'Read', content: 'b.go' }),
      evt('tool_result', { id: 'b', content: 'content b' }),
      evt('tool_result', { id: 'a', content: 'content a' }),
    ] as any);
    const calls = blocks.filter((b) => b.type === 'line' && (b as any).rest.startsWith('→ Read'));
    expect(calls.length).toBe(2);
    for (const c of calls) {
      expect((c as any).toolStatus).toBe('success');
    }
  });

  it('error tool_result → toolStatus error', () => {
    const blocks = buildLogBlocks([
      evt('tool_call', { id: 'err1', name: 'Bash', content: 'rm -rf' }),
      evt('tool_result', { id: 'err1', content: 'permission denied', error: true }),
    ] as any);
    const call = blocks.find((b) => b.type === 'line' && (b as any).toolId === 'err1');
    expect((call as any).toolStatus).toBe('error');
  });

  it('AskUserQuestion stays pending on error result', () => {
    const blocks = buildLogBlocks([
      evt('tool_call', { id: 'ask1', name: 'AskUserQuestion', content: 'Proceed?' }),
      evt('tool_result', { id: 'ask1', content: '', error: true }),
    ] as any);
    const call = blocks.find((b) => b.type === 'line' && (b as any).rest.startsWith('→ AskUserQuestion'));
    expect((call as any).toolStatus).toBe('pending');
  });

  it('user_message → UserMessageBlock', () => {
    const blocks = buildLogBlocks([evt('user_message', { content: 'fix the bug' })] as any);
    const msg = blocks.find((b) => b.type === 'user-message');
    expect(msg).toBeDefined();
    expect((msg as any).content).toBe('fix the bug');
  });

  it('turn_end is skipped', () => {
    const blocks = buildLogBlocks([
      evt('text', { content: 'done' }),
      evt('turn_end', { status: 'success', tokens: 100 }),
    ] as any);
    const turnEnd = blocks.find((b) => (b as any).type === 'turn_end');
    expect(turnEnd).toBeUndefined();
  });

  it('system event shown as line block', () => {
    const blocks = buildLogBlocks([evt('system', { content: '✗ API error' })] as any);
    const line = blocks.find((b) => b.type === 'line');
    expect(line).toBeDefined();
    expect((line as any).rest).toBe('✗ API error');
  });

  it('rendered_content used over content for tool result', () => {
    const blocks = buildLogBlocks([
      evt('tool_call', { id: 'rc1', name: 'Edit', content: 'f.go' }),
      evt('tool_result', { id: 'rc1', content: 'raw', rendered_content: '- old\n+ new' }),
    ] as any);
    const call = blocks.find((b) => (b as any).toolId === 'rc1') as any;
    expect(call?.toolResultLines).toContain('- old');
    expect(call?.toolResultLines).toContain('+ new');
  });
});
