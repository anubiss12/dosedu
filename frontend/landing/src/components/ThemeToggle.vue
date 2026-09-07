<script setup lang="ts">
import { theme, toggleTheme } from "../composables/useTheme";
</script>

<template>
  <button class="theme-toggle" type="button" @click="toggleTheme" :aria-label="theme === 'dark' ? 'Light mode' : 'Dark mode'">
    <span v-if="theme === 'dark'">☀️</span>
    <span v-else>🌙</span>
  </button>
</template>

<style scoped>
.theme-toggle {
  border: none;
  background: var(--surface-2);
  color: var(--text);
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 50%;
  cursor: pointer;
  font-size: 1.05rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease;
}
.theme-toggle:hover {
  background: var(--surface-3);
}
</style>
