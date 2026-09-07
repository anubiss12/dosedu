/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  theme: {
    extend: {
      colors: {
        // Theme-aware tokens — these read the CSS custom properties that
        // useTheme.ts toggles via <html data-theme="light|dark">, so
        // Tailwind utilities like `bg-surface1` stay in sync with the
        // existing dark-mode system without a separate Tailwind dark:
        // variant.
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
        // Marketing-specific accents (literal, same in both themes —
        // kept for hero mockup/section variety alongside the shared
        // --accent purple; no longer used for primary CTAs).
        indigo: {
          DEFAULT: "#2B3E70",
          light: "#6C86C4",
        },
        marigold: {
          DEFAULT: "#E8A33D",
          light: "#F0B45C",
        },
        teal: {
          DEFAULT: "#3E8574",
          light: "#57A895",
        },
      },
      fontFamily: {
        display: ["font-display", "Manrope", "sans-serif"],
        body: ["font-body", "Inter", "sans-serif"],
        accent: ["font-accent", "Manrope", "sans-serif"],
        numeric: ["font-numeric", "Manrope", "sans-serif"],
      },
      maxWidth: {
        prose: "42rem",
      },
    },
  },
  plugins: [],
};
