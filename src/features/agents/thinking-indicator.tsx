import { Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useThinkingStyle } from '@/providers/thinking-style';

function DotsIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-flex gap-1">
      {([0, 0.35, 0.7] as const).map((delay) => (
        <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/70' : 'bg-muted-foreground/50')} style={{ animation: `thinking-dot 1.4s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function SpinnerIndicator({ accent }: { accent: boolean }) {
  return (
    <span className={cn('mt-2 inline-flex items-center gap-2 text-xs', accent ? 'text-primary/80' : 'text-muted-foreground/60')}>
      <Loader2 className="size-3.5 animate-spin" />
      <span>Réflexion en cours</span>
    </span>
  );
}

function PillIndicator({ accent }: { accent: boolean }) {
  return (
    <span className={cn('mt-2 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs', accent ? 'bg-primary/10 text-primary/80' : 'bg-muted/60 text-muted-foreground/70')}>
      <span className={cn('size-1.5 animate-pulse rounded-full', accent ? 'bg-primary' : 'bg-muted-foreground/60')} />
      thinking
    </span>
  );
}

function BarIndicator({ accent }: { accent: boolean }) {
  return (
    <div className={cn('mt-3 h-px w-full overflow-hidden rounded-full', accent ? 'bg-primary/20' : 'bg-border')}>
      <div className={cn('h-full w-1/4 rounded-full', accent ? 'bg-primary/60' : 'bg-muted-foreground/40')} style={{ animation: 'thinking-shimmer 1.6s ease-in-out infinite' }} />
    </div>
  );
}

function WaveIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-flex items-end gap-1">
      {([0, 0.1, 0.2, 0.3, 0.4] as const).map((delay) => (
        <span key={delay} className={cn('block h-2 w-1 rounded-sm', accent ? 'bg-primary/60' : 'bg-muted-foreground/60')} style={{ animation: `thinking-wave 1s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function OrbitIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-block size-4" style={{ animation: 'thinking-orbit 1.2s linear infinite' }}>
      <span className="relative block h-full w-full">
        <span className={cn('absolute left-1/2 top-0 size-1.5 -translate-x-1/2 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} />
        <span className="absolute bottom-0 left-1/2 size-1 -translate-x-1/2 rounded-full bg-muted-foreground/40" />
      </span>
    </span>
  );
}

function TypingIndicator({ accent }: { accent: boolean }) {
  return (
    <span className={cn('mt-2 inline-flex items-center gap-1 text-xs', accent ? 'text-primary/70' : 'text-muted-foreground/70')}>
      <span>Réflexion</span>
      <span className="inline-block h-3 w-px bg-current" style={{ animation: 'thinking-cursor 1s steps(1) infinite' }} />
    </span>
  );
}

function BreathingIndicator({ accent }: { accent: boolean }) {
  return <span className={cn('mt-2 inline-block size-3 rounded-full', accent ? 'bg-primary/70' : 'bg-muted-foreground/50')} style={{ animation: 'thinking-breathing 1.6s ease-in-out infinite' }} />;
}

function SineIndicator({ accent }: { accent: boolean }) {
  return (
    <svg className="mt-2 h-3 w-12" viewBox="0 0 40 12" fill="none" aria-hidden="true">
      <path d="M0 6 Q 5 0 10 6 T 20 6 T 30 6 T 40 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className={accent ? 'text-primary/60' : 'text-muted-foreground/60'} strokeDasharray="40" style={{ animation: 'thinking-sine 1.4s linear infinite' }} />
    </svg>
  );
}

function EllipsisIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-flex gap-1">
      {([0, 0.4, 0.8] as const).map((delay) => (
        <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-ellipsis 1.2s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function RingIndicator({ accent }: { accent: boolean }) {
  return (
    <svg className="mt-2 size-4" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="5.5" strokeWidth="1.5" stroke="currentColor" className={accent ? 'text-primary/20' : 'text-muted-foreground/20'} />
      <circle cx="8" cy="8" r="5.5" strokeWidth="1.5" strokeLinecap="round" stroke="currentColor" className={accent ? 'text-primary/80' : 'text-muted-foreground/60'} strokeDasharray="34.5" strokeDashoffset="26" style={{ animation: 'thinking-ring 1s linear infinite', transformOrigin: '8px 8px' }} />
    </svg>
  );
}

function CascadeIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-flex items-start gap-1">
      {([0, 0.2, 0.4, 0.6] as const).map((delay) => (
        <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-cascade 1.2s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function GradientIndicator({ accent }: { accent: boolean }) {
  return (
    <div
      className="mt-3 h-1 w-full rounded-full"
      style={{
        backgroundImage: accent
          ? 'linear-gradient(90deg, transparent 0%, color-mix(in oklch, var(--primary) 85%, transparent) 50%, transparent 100%)'
          : 'linear-gradient(90deg, transparent 0%, color-mix(in oklch, var(--muted-foreground) 55%, transparent) 50%, transparent 100%)',
        backgroundSize: '200% 100%',
        animation: 'thinking-gradient 2.5s ease-in-out infinite',
      }}
    />
  );
}

function BounceIndicator({ accent }: { accent: boolean }) {
  return (
    <span className="mt-2 inline-flex items-end gap-1">
      {([0, 0.2, 0.4] as const).map((delay) => (
        <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-bounce 0.8s ease-in-out ${delay}s infinite` }} />
      ))}
    </span>
  );
}

function FlickerIndicator({ accent }: { accent: boolean }) {
  const dots = [
    { dur: '0.7s', delay: '0s' },
    { dur: '1.1s', delay: '0.2s' },
    { dur: '0.9s', delay: '0.4s' },
    { dur: '1.3s', delay: '0.1s' },
  ];
  return (
    <span className="mt-2 inline-flex items-center gap-1.5">
      {dots.map(({ dur, delay }, i) => (
        <span key={i} className={cn('size-1 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-flicker ${dur} ease-in-out ${delay} infinite` }} />
      ))}
    </span>
  );
}

export function ThinkingIndicator() {
  const { style, accent } = useThinkingStyle();
  switch (style) {
    case 'spinner': return <SpinnerIndicator accent={accent} />;
    case 'pill': return <PillIndicator accent={accent} />;
    case 'bar': return <BarIndicator accent={accent} />;
    case 'wave': return <WaveIndicator accent={accent} />;
    case 'orbit': return <OrbitIndicator accent={accent} />;
    case 'typing': return <TypingIndicator accent={accent} />;
    case 'breathing': return <BreathingIndicator accent={accent} />;
    case 'sine': return <SineIndicator accent={accent} />;
    case 'ellipsis': return <EllipsisIndicator accent={accent} />;
    case 'ring': return <RingIndicator accent={accent} />;
    case 'cascade': return <CascadeIndicator accent={accent} />;
    case 'gradient': return <GradientIndicator accent={accent} />;
    case 'bounce': return <BounceIndicator accent={accent} />;
    case 'flicker': return <FlickerIndicator accent={accent} />;
    default: return <DotsIndicator accent={accent} />;
  }
}
