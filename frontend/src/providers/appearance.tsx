import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from 'react';
import { DEFAULT_THEME_KEY, LIGHT_THEME_KEYS, THEMES } from '@/lib/themes';
import { CARD_ANIMATION_STYLES, type CardAnimationStyleKey, DEFAULT_CARD_ANIMATION_STYLE } from '@/lib/card-animation-styles';
import { DEFAULT_THINKING_STYLE, THINKING_STYLES, type ThinkingStyleKey } from '@/lib/thinking-styles';
import { GetAppearanceSettings, UpdateAppearanceSettings } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';

const THEME_CLASS_PREFIX = 'theme-';
const CUSTOM_THEME_STYLE_ID = 'polaris-custom-themes';

export type CustomTheme = polaris.CustomTheme;
type AppearancePayload = polaris.AppearanceSettings;

const DEFAULT_STATUS_BAR_BLOCKS = ['model', 'tools', 'tokens', 'tools-used', 'cost'];

function injectCustomThemeStyles(themes: CustomTheme[]) {
  const existing = document.getElementById(CUSTOM_THEME_STYLE_ID);
  const css = themes
    .map((t) => {
      const body = Object.entries(t.colors || {})
        .map(([k, v]) => `  --${k}: ${v};`)
        .join('\n');
      return `.theme-${t.key} {\n${body}\n}`;
    })
    .join('\n\n');
  if (existing) {
    existing.textContent = css;
    return;
  }
  const el = document.createElement('style');
  el.id = CUSTOM_THEME_STYLE_ID;
  el.textContent = css;
  document.head.appendChild(el);
}

function applyThemeToRoot(key: string, lightKeys: string[]) {
  const root = document.documentElement;
  for (const cls of Array.from(root.classList)) {
    if (cls.startsWith(THEME_CLASS_PREFIX)) {
      root.classList.remove(cls);
    }
  }
  root.classList.add(`${THEME_CLASS_PREFIX}${key}`);
  if (lightKeys.includes(key)) {
    root.classList.remove('dark');
  } else {
    root.classList.add('dark');
  }
}

function isValidThemeKey(key: string, customs: CustomTheme[]): boolean {
  if (THEMES.some((t) => t.key === key)) {
    return true;
  }
  return customs.some((t) => t.key === key);
}

function validTheme(key: string, customs: CustomTheme[]): string {
  return isValidThemeKey(key, customs) ? key : DEFAULT_THEME_KEY;
}

function validThinkingStyle(key: string): ThinkingStyleKey {
  return THINKING_STYLES.some((s) => s.key === key) ? (key as ThinkingStyleKey) : DEFAULT_THINKING_STYLE;
}

function validCardAnimationStyle(key: string): CardAnimationStyleKey {
  return CARD_ANIMATION_STYLES.some((s) => s.key === key) ? (key as CardAnimationStyleKey) : DEFAULT_CARD_ANIMATION_STYLE;
}

function lightKeysWithCustom(customs: CustomTheme[]): string[] {
  return [...LIGHT_THEME_KEYS, ...customs.filter((t) => t.mode === 'light').map((t) => t.key)];
}

function savePayload(payload: { theme: string; thinkingStyle: ThinkingStyleKey; thinkingStyleAccent: boolean; cardAnimationStyle: CardAnimationStyleKey; customThemes: CustomTheme[]; statusBarBlocks: string[]; gitChangesViewMode: string }) {
  UpdateAppearanceSettings(payload as AppearancePayload).catch(() => {});
}

interface AppearanceState {
  theme: string;
  thinkingStyle: ThinkingStyleKey;
  thinkingStyleAccent: boolean;
  cardAnimationStyle: CardAnimationStyleKey;
  customThemes: CustomTheme[];
  statusBarBlocks: string[];
  gitChangesViewMode: 'list' | 'tree';
  setTheme: (key: string) => void;
  setThinkingStyle: (key: ThinkingStyleKey) => void;
  setThinkingStyleAccent: (accent: boolean) => void;
  setCardAnimationStyle: (key: CardAnimationStyleKey) => void;
  setPreview: (key: string) => void;
  addCustomTheme: (theme: CustomTheme) => void;
  removeCustomTheme: (key: string) => void;
  setStatusBarBlocks: (blocks: string[]) => void;
  setGitChangesViewMode: (mode: 'list' | 'tree') => void;
}

const AppearanceContext = createContext<AppearanceState | null>(null);

export function AppearanceProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState(DEFAULT_THEME_KEY);
  const [thinkingStyle, setThinkingStyleState] = useState<ThinkingStyleKey>(DEFAULT_THINKING_STYLE);
  const [thinkingStyleAccent, setThinkingStyleAccentState] = useState<boolean>(false);
  const [cardAnimationStyle, setCardAnimationStyleState] = useState<CardAnimationStyleKey>(DEFAULT_CARD_ANIMATION_STYLE);
  const [customThemes, setCustomThemes] = useState<CustomTheme[]>([]);
  const [statusBarBlocks, setStatusBarBlocksState] = useState<string[]>(DEFAULT_STATUS_BAR_BLOCKS);
  const [gitChangesViewMode, setGitChangesViewModeState] = useState<'list' | 'tree'>('list');

  useEffect(() => {
    GetAppearanceSettings()
      .then((s) => {
        const customs = (s.customThemes || []) as CustomTheme[];
        injectCustomThemeStyles(customs);
        const t = validTheme(s.theme, customs);
        const ts = validThinkingStyle(s.thinkingStyle);
        const cas = validCardAnimationStyle(s.cardAnimationStyle ?? '');
        setCustomThemes(customs);
        setThemeState(t);
        setThinkingStyleState(ts);
        setThinkingStyleAccentState(s.thinkingStyleAccent ?? false);
        setCardAnimationStyleState(cas);
        setStatusBarBlocksState(s.statusBarBlocks?.length ? s.statusBarBlocks : DEFAULT_STATUS_BAR_BLOCKS);
        setGitChangesViewModeState(s.gitChangesViewMode === 'tree' ? 'tree' : 'list');
        applyThemeToRoot(t, lightKeysWithCustom(customs));
      })
      .catch(() => {
        applyThemeToRoot(DEFAULT_THEME_KEY, LIGHT_THEME_KEYS);
      });
  }, []);

  const save = useCallback(() => {
    setThemeState((t) => {
      setThinkingStyleState((ts) => {
        setThinkingStyleAccentState((tsa) => {
          setCardAnimationStyleState((cas) => {
            setCustomThemes((cts) => {
              setStatusBarBlocksState((sbb) => {
                setGitChangesViewModeState((vm) => {
                  savePayload({ theme: t, thinkingStyle: ts, thinkingStyleAccent: tsa, cardAnimationStyle: cas, customThemes: cts, statusBarBlocks: sbb, gitChangesViewMode: vm });
                  return vm;
                });
                return sbb;
              });
              return cts;
            });
            return cas;
          });
          return tsa;
        });
        return ts;
      });
      return t;
    });
  }, []);

  const setTheme = useCallback(
    (key: string) => {
      setCustomThemes((customs) => {
        const t = validTheme(key, customs);
        applyThemeToRoot(t, lightKeysWithCustom(customs));
        setThemeState(t);
        save();
        return customs;
      });
    },
    [save],
  );

  const setThinkingStyle = useCallback(
    (key: ThinkingStyleKey) => {
      const ts = validThinkingStyle(key);
      setThinkingStyleState(ts);
      save();
    },
    [save],
  );

  const setThinkingStyleAccent = useCallback(
    (accent: boolean) => {
      setThinkingStyleAccentState(accent);
      save();
    },
    [save],
  );

  const setCardAnimationStyle = useCallback(
    (key: CardAnimationStyleKey) => {
      setCardAnimationStyleState(validCardAnimationStyle(key));
      save();
    },
    [save],
  );

  const setPreview = useCallback((key: string) => {
    setCustomThemes((customs) => {
      applyThemeToRoot(key, lightKeysWithCustom(customs));
      return customs;
    });
  }, []);

  const addCustomTheme = useCallback(
    (incoming: CustomTheme) => {
      setCustomThemes((cts) => {
        const next = [...cts.filter((c) => c.key !== incoming.key), incoming];
        injectCustomThemeStyles(next);
        save();
        return next;
      });
    },
    [save],
  );

  const removeCustomTheme = useCallback(
    (key: string) => {
      setCustomThemes((cts) => {
        const next = cts.filter((c) => c.key !== key);
        injectCustomThemeStyles(next);
        setThemeState((t) => {
          const nextTheme = isValidThemeKey(t, next) ? t : DEFAULT_THEME_KEY;
          if (nextTheme !== t) {
            applyThemeToRoot(nextTheme, lightKeysWithCustom(next));
          }
          return nextTheme;
        });
        save();
        return next;
      });
    },
    [save],
  );

  const setStatusBarBlocks = useCallback(
    (blocks: string[]) => {
      setStatusBarBlocksState(blocks);
      save();
    },
    [save],
  );

  const setGitChangesViewMode = useCallback(
    (mode: 'list' | 'tree') => {
      setGitChangesViewModeState(mode);
      save();
    },
    [save],
  );

  return (
    <AppearanceContext.Provider
      value={{
        theme,
        thinkingStyle,
        thinkingStyleAccent,
        cardAnimationStyle,
        customThemes,
        statusBarBlocks,
        gitChangesViewMode,
        setTheme,
        setThinkingStyle,
        setThinkingStyleAccent,
        setCardAnimationStyle,
        setPreview,
        addCustomTheme,
        removeCustomTheme,
        setStatusBarBlocks,
        setGitChangesViewMode,
      }}
    >
      {children}
    </AppearanceContext.Provider>
  );
}

function useAppearance(): AppearanceState {
  const ctx = useContext(AppearanceContext);
  if (!ctx) {
    throw new Error('useAppearance must be used inside <AppearanceProvider>');
  }
  return ctx;
}

export function useTheme() {
  const { theme, setTheme, setPreview, customThemes, addCustomTheme, removeCustomTheme } = useAppearance();
  return { theme, setTheme, setPreview, customThemes, addCustomTheme, removeCustomTheme };
}

export function useThinkingStyle() {
  const { thinkingStyle, setThinkingStyle, thinkingStyleAccent, setThinkingStyleAccent } = useAppearance();
  return { style: thinkingStyle, setStyle: setThinkingStyle, accent: thinkingStyleAccent, setAccent: setThinkingStyleAccent };
}

export function useCardAnimationStyle() {
  const { cardAnimationStyle, setCardAnimationStyle } = useAppearance();
  return { style: cardAnimationStyle, setStyle: setCardAnimationStyle };
}

export function useStatusBarBlocks() {
  const { statusBarBlocks, setStatusBarBlocks } = useAppearance();
  return { blocks: statusBarBlocks, setBlocks: setStatusBarBlocks };
}

export function useGitChangesViewMode() {
  const { gitChangesViewMode, setGitChangesViewMode } = useAppearance();
  return { viewMode: gitChangesViewMode, setViewMode: setGitChangesViewMode };
}
