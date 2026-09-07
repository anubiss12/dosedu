import { ref } from "vue";

export type Theme = "light" | "dark";

const STORAGE_KEY = "dosedu_theme";

export const theme = ref<Theme>("light");

/**
 * Reads the saved preference (or falls back to the OS setting) and
 * applies it to <html data-theme="..."> before the app mounts, so
 * there's no flash of the wrong theme on load.
 */
export function initTheme(): void {
  const saved = localStorage.getItem(STORAGE_KEY) as Theme | null;
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  theme.value = saved ?? (prefersDark ? "dark" : "light");
  applyTheme(theme.value);
}

export function toggleTheme(): void {
  theme.value = theme.value === "light" ? "dark" : "light";
  localStorage.setItem(STORAGE_KEY, theme.value);
  applyTheme(theme.value);
}

function applyTheme(t: Theme): void {
  document.documentElement.setAttribute("data-theme", t);
}
