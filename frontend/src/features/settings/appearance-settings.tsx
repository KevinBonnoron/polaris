import { Plus, Trash2, Upload } from 'lucide-react';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { ThemePicker } from '@/features/theme/theme-picker';
import { CARD_ANIMATION_STYLES, type CardAnimationStyleKey } from '@/lib/card-animation-styles';
import { THINKING_STYLES, type ThinkingStyleKey } from '@/lib/thinking-styles';
import { cn } from '@/lib/utils';
import { useCardAnimationStyle } from '@/providers/appearance';
import { type CustomTheme, useStatusBarBlocks, useTheme } from '@/providers/theme-accent';
import { useThinkingStyle } from '@/providers/thinking-style';
import { StatusBarConfigurator } from './status-bar-settings';

const THEME_JSON_EXAMPLE = `{
  "key": "my-theme",
  "name": "My Theme",
  "mode": "dark",
  "colors": {
    "background": "#1a1a1a",
    "foreground": "#f0f0f0",
    "card": "#222",
    "card-foreground": "#f0f0f0",
    "primary": "#7aa2f7",
    "primary-foreground": "#1a1a1a",
    "border": "#333",
    "input": "#333",
    "ring": "#7aa2f7",
    "muted": "#222",
    "muted-foreground": "#9aa0b4",
    "accent": "#2a2a2a",
    "accent-foreground": "#f0f0f0",
    "sidebar": "#141414",
    "sidebar-foreground": "#f0f0f0",
    "sidebar-primary": "#7aa2f7",
    "sidebar-primary-foreground": "#1a1a1a",
    "sidebar-accent": "#222",
    "sidebar-accent-foreground": "#f0f0f0",
    "sidebar-border": "#333",
    "sidebar-ring": "#7aa2f7",
    "destructive": "#f7768e"
  }
}`;

function parseTheme(raw: string): { ok: true; theme: CustomTheme } | { ok: false; missing?: boolean } {
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed.key !== 'string' || typeof parsed.name !== 'string' || (parsed.mode !== 'light' && parsed.mode !== 'dark') || !parsed.colors || typeof parsed.colors !== 'object') {
      return { ok: false, missing: true };
    }
    const safeKey = parsed.key.replace(/[^a-z0-9-]/gi, '-').toLowerCase();
    return {
      ok: true,
      theme: {
        key: safeKey,
        name: parsed.name,
        mode: parsed.mode,
        colors: parsed.colors as Record<string, string>,
      },
    };
  } catch {
    return { ok: false };
  }
}

export function AppearanceSettings() {
  const { t } = useTranslation();
  const { theme, setTheme, customThemes, addCustomTheme, removeCustomTheme } = useTheme();
  const { style, setStyle, accent, setAccent } = useThinkingStyle();
  const { style: cardStyle, setStyle: setCardStyle } = useCardAnimationStyle();
  const { blocks, setBlocks } = useStatusBarBlocks();
  const [jsonInput, setJsonInput] = useState('');
  const [feedback, setFeedback] = useState<{ kind: 'error' | 'success'; message: string } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleImport = (raw: string) => {
    const result = parseTheme(raw);
    if (!result.ok) {
      setFeedback({
        kind: 'error',
        message: result.missing ? t('settings.appearance.importErrorMissing') : t('settings.appearance.importError'),
      });
      return;
    }
    addCustomTheme(result.theme);
    setFeedback({ kind: 'success', message: t('settings.appearance.importSuccess', { name: result.theme.name }) });
    setJsonInput('');
  };

  const handleFile = (file: File) => {
    file
      .text()
      .then(handleImport)
      .catch(() => setFeedback({ kind: 'error', message: t('settings.appearance.importError') }));
  };

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <div className="flex flex-col gap-1">
          <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.appearance.theme')}</h4>
          <p className="text-xs text-muted-foreground">{t('settings.appearance.themeHint')}</p>
        </div>
        <Dialog
          onOpenChange={(open) => {
            if (!open) {
              setJsonInput('');
              setFeedback(null);
            }
          }}
        >
          <ThemePicker value={theme} onChange={setTheme}>
            <DialogTrigger asChild>
              <button type="button" className="inline-flex items-center gap-2 rounded-full border border-dashed px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground">
                <Plus className="size-2.5" />
                {customThemes.length > 0 ? `${t('settings.appearance.customThemes')} · ${customThemes.length}` : t('settings.appearance.customThemes')}
              </button>
            </DialogTrigger>
          </ThemePicker>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('settings.appearance.customThemes')}</DialogTitle>
            </DialogHeader>
            <div className="flex flex-col gap-4">
              <p className="text-sm text-muted-foreground">{t('settings.appearance.customThemesHint')}</p>

              {customThemes.length > 0 && (
                <div className="flex flex-col gap-1.5">
                  {customThemes.map((ct) => (
                    <div key={ct.key} className={cn('flex items-center justify-between gap-3 rounded-md border bg-card px-3 py-2 text-sm')}>
                      <div className="flex items-center gap-2">
                        <span className={`theme-${ct.key} inline-block size-3 rounded-full`} style={{ background: 'var(--primary)' }} />
                        <span className="font-medium">{ct.name}</span>
                        <span className="text-xs text-muted-foreground">{ct.mode}</span>
                      </div>
                      <Button variant="ghost" size="xs" onClick={() => removeCustomTheme(ct.key)} aria-label={t('settings.appearance.removeTheme')}>
                        <Trash2 />
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              <Textarea
                value={jsonInput}
                onChange={(e) => {
                  setJsonInput(e.target.value);
                  setFeedback(null);
                }}
                placeholder={t('settings.appearance.pastePlaceholder')}
                rows={6}
                className="font-mono text-xs"
              />

              <div className="flex flex-wrap items-center gap-2">
                <Button size="sm" onClick={() => handleImport(jsonInput)} disabled={!jsonInput.trim()}>
                  {t('settings.appearance.importTheme')}
                </Button>
                <Button size="sm" variant="outline" onClick={() => fileInputRef.current?.click()}>
                  <Upload />
                  {t('settings.appearance.importFromFile')}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setJsonInput(THEME_JSON_EXAMPLE)}>
                  {t('settings.appearance.themeJsonExample')}
                </Button>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="application/json,.json"
                  className="hidden"
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    if (file) {
                      handleFile(file);
                    }
                    e.target.value = '';
                  }}
                />
              </div>

              {feedback && <p className={cn('text-xs', feedback.kind === 'error' ? 'text-destructive' : 'text-muted-foreground')}>{feedback.message}</p>}
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <div className="flex flex-col gap-3">
        <div className="flex items-start justify-between gap-4">
          <div className="flex flex-col gap-1">
            <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.appearance.thinkingStyle')}</h4>
            <p className="text-xs text-muted-foreground">{t('settings.appearance.thinkingStyleHint')}</p>
          </div>
          <div className="flex shrink-0 overflow-hidden rounded-md border text-xs">
            <button type="button" onClick={() => setAccent(false)} className={cn('px-2.5 py-1 transition-colors', !accent ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}>
              {t('settings.appearance.thinkingStyleNeutral')}
            </button>
            <button type="button" onClick={() => setAccent(true)} className={cn('px-2.5 py-1 transition-colors', accent ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}>
              {t('settings.appearance.thinkingStyleThemed')}
            </button>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
          {THINKING_STYLES.map((s) => (
            <button
              key={s.key}
              type="button"
              onClick={() => setStyle(s.key as ThinkingStyleKey)}
              className={cn('flex flex-col items-center justify-between gap-3 rounded-lg border p-3 text-xs transition-colors', style === s.key ? 'border-primary bg-primary/5 text-foreground' : 'border-border text-muted-foreground hover:border-border/80 hover:bg-muted/40')}
            >
              <div className="flex h-8 w-full items-center justify-center">
                <ThinkingStylePreview styleKey={s.key as ThinkingStyleKey} accent={accent} />
              </div>
              <span>{t(`settings.appearance.thinkingStyles.${s.key}`)}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.appearance.cardAnimation')}</h4>
          <p className="text-xs text-muted-foreground">{t('settings.appearance.cardAnimationHint')}</p>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
          {CARD_ANIMATION_STYLES.map((s) => (
            <button
              key={s.key}
              type="button"
              onClick={() => setCardStyle(s.key as CardAnimationStyleKey)}
              className={cn('flex flex-col items-center justify-between gap-3 rounded-lg border p-3 text-xs transition-colors', cardStyle === s.key ? 'border-primary bg-primary/5 text-foreground' : 'border-border text-muted-foreground hover:border-border/80 hover:bg-muted/40')}
            >
              <div className="flex h-8 w-full items-center justify-center">
                <CardAnimationPreview styleKey={s.key as CardAnimationStyleKey} />
              </div>
              <span>{t(`settings.appearance.cardStyles.${s.key}`)}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('settings.appearance.statusBar')}</h4>
          <p className="text-xs text-muted-foreground">{t('settings.appearance.statusBarHint')}</p>
        </div>
        <StatusBarConfigurator blocks={blocks} onChange={setBlocks} />
      </div>
    </section>
  );
}

function CardAnimationPreview({ styleKey }: { styleKey: CardAnimationStyleKey }) {
  const base = 'relative overflow-hidden h-8 w-full rounded border border-border/50 bg-card';
  switch (styleKey) {
    case 'pulse':
      return (
        <div className={base}>
          <span className="absolute inset-0 animate-pulse rounded ring-1 ring-inset ring-primary/50" />
        </div>
      );
    case 'shimmer':
      return (
        <div className={base}>
          <span className="absolute inset-y-0 w-1/3 bg-gradient-to-r from-transparent via-foreground/10 to-transparent" style={{ animation: 'card-shimmer 2s ease-in-out infinite' }} />
        </div>
      );
    case 'progress':
      return (
        <div className={base}>
          <span className="absolute top-0 left-0 h-0.5 w-1/3 rounded-full bg-primary/70" style={{ animation: 'card-progress 1.8s ease-in-out infinite' }} />
        </div>
      );
    case 'glow':
      return <div className={cn(base, 'animate-pulse ring-2 ring-primary/30')} />;
    case 'none':
      return <div className={base} />;
  }
}

function ThinkingStylePreview({ styleKey, accent }: { styleKey: ThinkingStyleKey; accent: boolean }) {
  const { t } = useTranslation();
  const label = t('settings.appearance.thinkingPreview');
  switch (styleKey) {
    case 'spinner':
      return (
        <span className={cn('inline-flex items-center gap-1.5 text-xs', accent ? 'text-primary/80' : 'text-muted-foreground/60')}>
          <svg className="size-3.5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
            <path d="M21 12a9 9 0 1 1-6.219-8.56" />
          </svg>
          <span>{label}</span>
        </span>
      );
    case 'pill':
      return (
        <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs', accent ? 'bg-primary/10 text-primary/80' : 'bg-muted/60 text-muted-foreground/70')}>
          <span className={cn('size-1.5 animate-pulse rounded-full', accent ? 'bg-primary' : 'bg-muted-foreground/60')} />
          {label}
        </span>
      );
    case 'bar':
      return (
        <div className={cn('h-px w-full overflow-hidden rounded-full', accent ? 'bg-primary/20' : 'bg-border')}>
          <div className={cn('h-full w-1/4 rounded-full', accent ? 'bg-primary/60' : 'bg-muted-foreground/40')} style={{ animation: 'thinking-shimmer 1.6s ease-in-out infinite' }} />
        </div>
      );
    case 'wave':
      return (
        <span className="inline-flex items-end gap-1">
          {([0, 0.1, 0.2, 0.3, 0.4] as const).map((delay) => (
            <span key={delay} className={cn('block h-2 w-1 rounded-sm', accent ? 'bg-primary/60' : 'bg-muted-foreground/60')} style={{ animation: `thinking-wave 1s ease-in-out ${delay}s infinite` }} />
          ))}
        </span>
      );
    case 'orbit':
      return (
        <span className="inline-block size-4" style={{ animation: 'thinking-orbit 1.2s linear infinite' }}>
          <span className="relative block h-full w-full">
            <span className={cn('absolute left-1/2 top-0 size-1.5 -translate-x-1/2 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} />
            <span className="absolute bottom-0 left-1/2 size-1 -translate-x-1/2 rounded-full bg-muted-foreground/40" />
          </span>
        </span>
      );
    case 'typing':
      return (
        <span className={cn('inline-flex items-center gap-1 text-xs', accent ? 'text-primary/70' : 'text-muted-foreground/70')}>
          <span>{label}</span>
          <span className="inline-block h-3 w-px bg-current" style={{ animation: 'thinking-cursor 1s steps(1) infinite' }} />
        </span>
      );
    case 'breathing':
      return <span className={cn('inline-block size-3 rounded-full', accent ? 'bg-primary/70' : 'bg-muted-foreground/50')} style={{ animation: 'thinking-breathing 1.6s ease-in-out infinite' }} />;
    case 'sine':
      return (
        <svg className="h-3 w-12" viewBox="0 0 40 12" fill="none" aria-hidden="true">
          <path d="M0 6 Q 5 0 10 6 T 20 6 T 30 6 T 40 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className={accent ? 'text-primary/60' : 'text-muted-foreground/60'} strokeDasharray="40" style={{ animation: 'thinking-sine 1.4s linear infinite' }} />
        </svg>
      );
    case 'ellipsis':
      return (
        <span className="inline-flex gap-1">
          {([0, 0.4, 0.8] as const).map((delay) => (
            <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-ellipsis 1.2s ease-in-out ${delay}s infinite` }} />
          ))}
        </span>
      );
    case 'ring':
      return (
        <svg className="size-4" viewBox="0 0 16 16" fill="none" aria-hidden="true">
          <circle cx="8" cy="8" r="5.5" strokeWidth="1.5" stroke="currentColor" className={accent ? 'text-primary/20' : 'text-muted-foreground/20'} />
          <circle cx="8" cy="8" r="5.5" strokeWidth="1.5" strokeLinecap="round" stroke="currentColor" className={accent ? 'text-primary/80' : 'text-muted-foreground/60'} strokeDasharray="34.5" strokeDashoffset="26" style={{ animation: 'thinking-ring 1s linear infinite', transformOrigin: '8px 8px' }} />
        </svg>
      );
    case 'cascade':
      return (
        <span className="inline-flex items-start gap-1">
          {([0, 0.2, 0.4, 0.6] as const).map((delay) => (
            <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-cascade 1.2s ease-in-out ${delay}s infinite` }} />
          ))}
        </span>
      );
    case 'gradient':
      return (
        <div
          className="h-1 w-full rounded-full"
          style={{
            backgroundImage: accent ? 'linear-gradient(90deg, transparent 0%, color-mix(in oklch, var(--primary) 85%, transparent) 50%, transparent 100%)' : 'linear-gradient(90deg, transparent 0%, color-mix(in oklch, var(--muted-foreground) 55%, transparent) 50%, transparent 100%)',
            backgroundSize: '200% 100%',
            animation: 'thinking-gradient 2.5s ease-in-out infinite',
          }}
        />
      );
    case 'bounce':
      return (
        <span className="inline-flex items-end gap-1">
          {([0, 0.2, 0.4] as const).map((delay) => (
            <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-bounce 0.8s ease-in-out ${delay}s infinite` }} />
          ))}
        </span>
      );
    case 'flicker':
      return (
        <span className="inline-flex items-center gap-1.5">
          {(
            [
              { dur: '0.7s', delay: '0s' },
              { dur: '1.1s', delay: '0.2s' },
              { dur: '0.9s', delay: '0.4s' },
              { dur: '1.3s', delay: '0.1s' },
            ] as const
          ).map(({ dur, delay }, i) => (
            <span key={i} className={cn('size-1 rounded-full', accent ? 'bg-primary/80' : 'bg-muted-foreground/60')} style={{ animation: `thinking-flicker ${dur} ease-in-out ${delay} infinite` }} />
          ))}
        </span>
      );
    default:
      return (
        <span className="inline-flex gap-1">
          {([0, 0.35, 0.7] as const).map((delay) => (
            <span key={delay} className={cn('size-1.5 rounded-full', accent ? 'bg-primary/70' : 'bg-muted-foreground/50')} style={{ animation: `thinking-dot 1.4s ease-in-out ${delay}s infinite` }} />
          ))}
        </span>
      );
  }
}
