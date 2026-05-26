import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from 'react';
import { DEFAULT_THEME_KEY, LIGHT_THEME_KEYS, THEMES } from '@/lib/themes';
import { DEFAULT_THINKING_STYLE, THINKING_STYLES, type ThinkingStyleKey } from '@/lib/thinking-styles';
import { GetAppearanceSettings, UpdateAppearanceSettings } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';

const THEME_CLASS_PREFIX = 'theme-';
const CUSTOM_THEME_STYLE_ID = 'polaris-custom-themes';

export type CustomTheme = polaris.CustomTheme;
type AppearancePayload = polaris.AppearanceSettings;

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

function lightKeysWithCustom(customs: CustomTheme[]): string[] {
  return [...LIGHT_THEME_KEYS, ...customs.filter((t) => t.mode === 'light').map((t) => t.key)];
}

function savePayload(payload: { theme: string; thinkingStyle: ThinkingStyleKey; customThemes: CustomTheme[] }) {
  UpdateAppearanceSettings(payload as AppearancePayload).catch(() => {});
}

interface AppearanceState {
  theme: string;
  thinkingStyle: ThinkingStyleKey;
  customThemes: CustomTheme[];
  setTheme: (key: string) => void;
  setThinkingStyle: (key: ThinkingStyleKey) => void;
  setPreview: (key: string) => void;
  addCustomTheme: (theme: CustomTheme) => void;
  removeCustomTheme: (key: string) => void;
}

const AppearanceContext = createContext<AppearanceState | null>(null);

export function AppearanceProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState(DEFAULT_THEME_KEY);
  const [thinkingStyle, setThinkingStyleState] = useState<ThinkingStyleKey>(DEFAULT_THINKING_STYLE);
  const [customThemes, setCustomThemes] = useState<CustomTheme[]>([]);

  useEffect(() => {
    GetAppearanceSettings()
      .then((s) => {
        const customs = (s.customThemes || []) as CustomTheme[];
        injectCustomThemeStyles(customs);
        const t = validTheme(s.theme, customs);
        const ts = validThinkingStyle(s.thinkingStyle);
        setCustomThemes(customs);
        setThemeState(t);
        setThinkingStyleState(ts);
        applyThemeToRoot(t, lightKeysWithCustom(customs));
      })
      .catch(() => {
        applyThemeToRoot(DEFAULT_THEME_KEY, LIGHT_THEME_KEYS);
      });
  }, []);

  const setTheme = useCallback((key: string) => {
    setCustomThemes((customs) => {
      const t = validTheme(key, customs);
      applyThemeToRoot(t, lightKeysWithCustom(customs));
      setThemeState(t);
      setThinkingStyleState((ts) => {
        savePayload({ theme: t, thinkingStyle: ts, customThemes: customs });
        return ts;
      });
      return customs;
    });
  }, []);

  const setThinkingStyle = useCallback((key: ThinkingStyleKey) => {
    const ts = validThinkingStyle(key);
    setThinkingStyleState(ts);
    setThemeState((t) => {
      setCustomThemes((cts) => {
        savePayload({ theme: t, thinkingStyle: ts, customThemes: cts });
        return cts;
      });
      return t;
    });
  }, []);

  const setPreview = useCallback((key: string) => {
    setCustomThemes((customs) => {
      applyThemeToRoot(key, lightKeysWithCustom(customs));
      return customs;
    });
  }, []);

  const addCustomTheme = useCallback((incoming: CustomTheme) => {
    setCustomThemes((cts) => {
      const next = [...cts.filter((c) => c.key !== incoming.key), incoming];
      injectCustomThemeStyles(next);
      setThemeState((t) => {
        setThinkingStyleState((ts) => {
          savePayload({ theme: t, thinkingStyle: ts, customThemes: next });
          return ts;
        });
        return t;
      });
      return next;
    });
  }, []);

  const removeCustomTheme = useCallback((key: string) => {
    setCustomThemes((cts) => {
      const next = cts.filter((c) => c.key !== key);
      injectCustomThemeStyles(next);
      setThemeState((t) => {
        const nextTheme = isValidThemeKey(t, next) ? t : DEFAULT_THEME_KEY;
        if (nextTheme !== t) {
          applyThemeToRoot(nextTheme, lightKeysWithCustom(next));
        }
        setThinkingStyleState((ts) => {
          savePayload({ theme: nextTheme, thinkingStyle: ts, customThemes: next });
          return ts;
        });
        return nextTheme;
      });
      return next;
    });
  }, []);

  return (
    <AppearanceContext.Provider
      value={{
        theme,
        thinkingStyle,
        customThemes,
        setTheme,
        setThinkingStyle,
        setPreview,
        addCustomTheme,
        removeCustomTheme,
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
  const { thinkingStyle, setThinkingStyle } = useAppearance();
  return { style: thinkingStyle, setStyle: setThinkingStyle };
}
