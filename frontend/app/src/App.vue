<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";
import StudentDashboard from "./components/StudentDashboard.vue";
import ParentPortal from "./components/ParentPortal.vue";

const { t } = useI18n();

const token = ref<string | null>(localStorage.getItem("dosedu_token"));
const loggedInRole = ref<"student" | "parent" | null>(
  (localStorage.getItem("dosedu_role") as "student" | "parent" | null) ?? null
);
const identifier = ref("");
const password = ref("");
const loginError = ref(false);
const showForgotPassword = ref(false);

function detectRole(value: string): "student" | "parent" {
  const looksLikePhone = /^\+?\d[\d\s()-]{6,}$/.test(value.trim());
  return looksLikePhone ? "parent" : "student";
}

async function login() {
  loginError.value = false;
  const role = detectRole(identifier.value);
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifier: identifier.value, password: password.value, role }),
    });
    if (!res.ok) throw new Error("invalid");
    const data = await res.json();
    token.value = data.token;
    loggedInRole.value = role;
    localStorage.setItem("dosedu_token", data.token);
    localStorage.setItem("dosedu_role", role);
  } catch {
    loginError.value = true;
  }
}

function logout() {
  token.value = null;
  loggedInRole.value = null;
  localStorage.removeItem("dosedu_token");
  localStorage.removeItem("dosedu_role");
}

// Accept a token handed off from the landing page's login (e.g.
// https://app.dosedu.kz/#token=...&role=student), so the small landing
// modal only needs to authenticate, then the person lands here on this
// full page for everything else.
function consumeHandoff() {
  const hash = window.location.hash.startsWith("#") ? window.location.hash.slice(1) : "";
  if (!hash) return;
  const params = new URLSearchParams(hash);
  const handoffToken = params.get("token");
  const handoffRole = params.get("role") as "student" | "parent" | null;
  if (handoffToken && handoffRole) {
    token.value = handoffToken;
    loggedInRole.value = handoffRole;
    localStorage.setItem("dosedu_token", handoffToken);
    localStorage.setItem("dosedu_role", handoffRole);
    history.replaceState(null, "", window.location.pathname + window.location.search);
  }
}

onMounted(() => {
  consumeHandoff();
});
</script>

<template>
  <header class="flex items-center justify-between border-b border-line px-4 py-3 sm:px-6">
    <div class="flex items-center gap-3">
      <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
      <span class="font-display text-base font-bold text-ink sm:text-lg">{{ t("app.title") }}</span>
    </div>
    <div class="flex items-center gap-2 sm:gap-3">
      <button v-if="token" type="button" class="rounded-lg bg-accent px-3 py-1.5 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="logout">↩</button>
      <LangSwitcher />
      <ThemeToggle />
    </div>
  </header>

  <main class="mx-auto max-w-3xl px-4 py-6 sm:px-6">
    <form
      v-if="!token"
      class="mx-auto max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 sm:p-6"
      @submit.prevent="login"
    >
      <h2 class="font-display text-lg font-bold text-ink">{{ t("login.title") }}</h2>
      <label class="block text-sm text-ink-muted">
        {{ t("login.identifier") }}
        <input v-model="identifier" required class="mt-1" />
      </label>
      <label class="block text-sm text-ink-muted">
        {{ t("login.password") }}
        <input v-model="password" type="password" inputmode="numeric" required minlength="4" maxlength="4" class="mt-1" />
      </label>
      <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90">
        {{ t("login.submit") }}
      </button>
      <p v-if="loginError" class="text-center text-sm text-danger">{{ t("login.error") }}</p>
      <button type="button" class="block w-full text-center text-xs text-ink-muted underline" @click="showForgotPassword = !showForgotPassword">
        {{ t("login.forgotPassword") }}
      </button>
      <p v-if="showForgotPassword" class="text-center text-xs text-ink-muted">{{ t("login.forgotPasswordHint") }}</p>
    </form>

    <ParentPortal v-else-if="loggedInRole === 'parent' && token" :token="token" />
    <StudentDashboard v-else-if="token" :token="token" />
  </main>
</template>
