import { Loader2 } from 'lucide-react';
import { useThinkingStyle } from '@/providers/thinking-style';

function DotsIndicator() {
  return (
    <span className="mt-2 inline-flex gap-1">
      {([0, 0.35, 0.7] as const).map((delay) => (
        <span key={delay} className="size-1.5 rounded-full bg-muted-foreground/50" style={{ animation: `thinking-dot 1.4s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function SpinnerIndicator() {
  return (
    <span className="mt-2 inline-flex items-center gap-2 text-xs text-muted-foreground/60">
      <Loader2 className="size-3.5 animate-spin" />
      <span>Réflexion en cours</span>
    </span>
  );
}

function PillIndicator() {
  return (
    <span className="mt-2 inline-flex items-center gap-1.5 rounded-full bg-muted/60 px-2.5 py-1 text-xs text-muted-foreground/70">
      <span className="size-1.5 animate-pulse rounded-full bg-blue-400" />
      thinking
    </span>
  );
}

function BarIndicator() {
  return (
    <div className="mt-3 h-px w-full overflow-hidden rounded-full bg-border">
      <div className="h-full w-1/4 rounded-full bg-muted-foreground/40" style={{ animation: 'thinking-shimmer 1.6s ease-in-out infinite' }} />
    </div>
  );
}

function WaveIndicator() {
  return (
    <span className="mt-2 inline-flex items-end gap-1">
      {([0, 0.1, 0.2, 0.3, 0.4] as const).map((delay) => (
        <span key={delay} className="block h-2 w-1 rounded-sm bg-muted-foreground/60" style={{ animation: `thinking-wave 1s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function OrbitIndicator() {
  return (
    <span className="mt-2 inline-block size-4" style={{ animation: 'thinking-orbit 1.2s linear infinite' }}>
      <span className="relative block h-full w-full">
        <span className="absolute left-1/2 top-0 size-1.5 -translate-x-1/2 rounded-full bg-primary/80" />
        <span className="absolute bottom-0 left-1/2 size-1 -translate-x-1/2 rounded-full bg-muted-foreground/40" />
      </span>
    </span>
  );
}

function TypingIndicator() {
  return (
    <span className="mt-2 inline-flex items-center gap-1 text-xs text-muted-foreground/70">
      <span>Réflexion</span>
      <span className="inline-block h-3 w-px bg-muted-foreground/70" style={{ animation: 'thinking-cursor 1s steps(1) infinite' }} />
    </span>
  );
}

function BreathingIndicator() {
  return <span className="mt-2 inline-block size-3 rounded-full bg-primary/70" style={{ animation: 'thinking-breathing 1.6s ease-in-out infinite' }} />;
}

function SineIndicator() {
  return (
    <svg className="mt-2 h-3 w-12" viewBox="0 0 40 12" fill="none" aria-hidden="true">
      <path d="M0 6 Q 5 0 10 6 T 20 6 T 30 6 T 40 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className="text-muted-foreground/60" strokeDasharray="40" style={{ animation: 'thinking-sine 1.4s linear infinite' }} />
    </svg>
  );
}

export function ThinkingIndicator() {
  const { style } = useThinkingStyle();
  switch (style) {
    case 'spinner':
      return <SpinnerIndicator />;
    case 'pill':
      return <PillIndicator />;
    case 'bar':
      return <BarIndicator />;
    case 'wave':
      return <WaveIndicator />;
    case 'orbit':
      return <OrbitIndicator />;
    case 'typing':
      return <TypingIndicator />;
    case 'breathing':
      return <BreathingIndicator />;
    case 'sine':
      return <SineIndicator />;
    default:
      return <DotsIndicator />;
  }
}
