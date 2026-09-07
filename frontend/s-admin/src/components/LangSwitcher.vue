<script setup lang="ts">
import { useI18n } from "vue-i18n";

const { locale } = useI18n();

const languages = [
  { code: "kk", label: "Қазақша" },
  { code: "en", label: "English" },
];

function setLocale(code: string) {
  locale.value = code;
  localStorage.setItem("dosedu_locale", code);
}
</script>

<template>
  <select class="lang-switcher" :value="locale" @change="setLocale(($event.target as HTMLSelectElement).value)">
    <option v-for="lang in languages" :key="lang.code" :value="lang.code">
      {{ lang.label }}
    </option>
  </select>
</template>

<style scoped>
.lang-switcher {
  border-radius: 8px;
  padding: 0.35rem 0.6rem;
  cursor: pointer;
}
</style>
