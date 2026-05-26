import { type CustomTheme, useStatusBarBlocks, useTheme } from './appearance';

export { type CustomTheme, useStatusBarBlocks, useTheme };

export function useThemePreview() {
  const { setPreview } = useTheme();
  return { setPreview };
}
