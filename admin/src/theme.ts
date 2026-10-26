import { ref } from 'vue';

/**
 * The panel's theme, per browser: Auto follows prefers-color-scheme (no data-theme on <html>), Light / Dark
 * set data-theme. public/theme.js applies the saved choice before the first paint.
 */
export type Theme = 'system' | 'light' | 'dark';
const KEY = 'dialcatz-den-theme';

function saved(): Theme {
  try {
    const t = localStorage.getItem(KEY);
    return t === 'light' || t === 'dark' ? t : 'system';
  } catch {
    return 'system';
  }
}

export const theme = ref<Theme>(saved());

export function setTheme(t: Theme) {
  theme.value = t;
  if (t === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', t);
  try {
    if (t === 'system') localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, t);
  } catch {
    // Storage blocked (private window): the choice lasts until the page is closed.
  }
}
