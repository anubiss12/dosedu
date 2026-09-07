/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  theme: {
    extend: {
      colors: {
        // Theme-aware tokens reading the CSS custom properties toggled
        // by useTheme.ts via <html data-theme="light|dark">. `accent`
        // is this app's own distinct brand color (set in style.css),
        // so `bg-accent` automatically differs per cabinet.
        surface1: "var(--surface-1)",
        surface2: "var(--surface-2)",
        surface3: "var(--surface-3)",
        ink: "var(--text)",
        "ink-muted": "var(--text-muted)",
        line: "var(--border)",
        danger: "var(--danger)",
        success: "var(--success)",
        accent: "var(--accent)",
        "accent-contrast": "var(--accent-contrast)",
        "accent-secondary": "var(--accent-secondary)",
        badge1: { DEFAULT: "var(--badge-1-bg)", fg: "var(--badge-1-fg)" },
        badge2: { DEFAULT: "var(--badge-2-bg)", fg: "var(--badge-2-fg)" },
        badge3: { DEFAULT: "var(--badge-3-bg)", fg: "var(--badge-3-fg)" },
        badge4: { DEFAULT: "var(--badge-4-bg)", fg: "var(--badge-4-fg)" },
      },
      fontFamily: {
        display: ["font-display", "Manrope", "sans-serif"],
        body: ["font-body", "Inter", "sans-serif"],
        accent: ["font-accent", "Manrope", "sans-serif"],
        numeric: ["font-numeric", "Manrope", "sans-serif"],
      },
    },
  },
  plugins: [],
};
