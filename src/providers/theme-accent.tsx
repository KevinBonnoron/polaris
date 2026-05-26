import { type CustomTheme, useTheme } from './appearance';

export { type CustomTheme, useTheme };

export function useThemePreview() {
  const { setPreview } = useTheme();
  return { setPreview };
}
