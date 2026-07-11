export const STATUS_ORDER = ['draft', 'waiting', 'sleeping', 'error', 'working', 'completed', 'stopped', 'idle', 'archived'] as const;
export type AgentStatus = (typeof STATUS_ORDER)[number];

const KNOWN_STATUSES = new Set<string>(STATUS_ORDER);

// Any status the backend hasn't taught the UI about is shown as "idle", so an
// unexpected value still renders instead of silently falling out of every status
// bucket. Shared so the list page and the list item can't drift apart.
export const normalizeAgentStatus = (status: string): AgentStatus => (KNOWN_STATUSES.has(status) ? (status as AgentStatus) : 'idle');
