const TINTS: Record<string, string> = {
  bug: 'bg-red-500/15 text-red-400',
  story: 'bg-emerald-500/15 text-emerald-400',
  task: 'bg-blue-500/15 text-blue-400',
  feature: 'bg-blue-500/15 text-blue-400',
  improvement: 'bg-purple-500/15 text-purple-400',
  epic: 'bg-violet-500/15 text-violet-400',
  refactor: 'bg-cyan-500/15 text-cyan-400',
  setup: 'bg-emerald-500/15 text-emerald-400',
  subtask: 'bg-slate-500/15 text-slate-300',
};

const DEFAULT = 'bg-slate-500/15 text-slate-300';

export function tagTint(name: string | undefined): string {
  if (!name) {
    return DEFAULT;
  }
  return TINTS[name.toLowerCase()] ?? DEFAULT;
}
