import { createI18n } from "vue-i18n";
import kk from "./locales/kk.json";
import en from "./locales/en.json";

const STORAGE_KEY = "dosedu_locale";
const SUPPORTED = ["kk", "en"] as const;

function detectLocale(): string {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved && SUPPORTED.includes(saved as (typeof SUPPORTED)[number])) return saved;

  const browser = navigator.language.slice(0, 2);
  if (SUPPORTED.includes(browser as (typeof SUPPORTED)[number])) return browser;

  return "kk";
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: "kk",
  messages: { kk, en },
});
