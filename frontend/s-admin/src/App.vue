<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";

const { t } = useI18n();

const token = ref<string | null>(localStorage.getItem("dosedu_token"));
const identifier = ref("");
const password = ref("");
const loginError = ref(false);

type Tab = "health" | "branches" | "directors" | "logs";
const activeTab = ref<Tab>("health");

type Health = {
  postgres: string;
  redis: string;
  cpu_percent: number;
  ram_percent: number;
  disk_percent: number;
  db_pool: { total: number; idle: number; acquired: number };
  main_site: string;
  app_site: string;
};
const health = ref<Health | null>(null);
let healthTimer: ReturnType<typeof setInterval> | undefined;

function statusColor(status: string): string {
  if (status === "up") return "bg-success";
  if (status === "not_configured") return "bg-ink-muted";
  return "bg-danger";
}

type Branch = { id: string; name: string; address: string; status: string };
const branches = ref<Branch[]>([]);
const newBranchName = ref("");
const newBranchAddress = ref("");

type DirectorAccount = { id: string; branch_id: string; branch_name: string; email: string; full_name: string };
const directors = ref<DirectorAccount[]>([]);
const newDirectorEmail = ref("");
const newDirectorName = ref("");
const newDirectorBranchId = ref("");
const newDirectorCredentials = ref<{ email: string; password: string; notice: string } | null>(null);

type SystemLog = { id: number; level: string; source: string; message: string; created_at: string };
const logs = ref<SystemLog[]>([]);

function authHeaders() {
  return { Authorization: `Bearer ${token.value}` };
}

async function login() {
  loginError.value = false;
  try {
    const res = await fetch("/api/public/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifier: identifier.value, password: password.value, role: "super_admin" }),
    });
    if (!res.ok) throw new Error("invalid");
    const data = await res.json();
    token.value = data.token;
    localStorage.setItem("dosedu_token", data.token);
    activeTab.value = "health";
    await loadAll();
    clearInterval(healthTimer);
    healthTimer = setInterval(loadHealth, 15000);
  } catch {
    loginError.value = true;
  }
}

async function loadHealth() {
  if (!token.value) return;
  const res = await fetch("/api/health", { headers: authHeaders() });
  if (res.ok) health.value = await res.json();
}

async function loadBranches() {
  if (!token.value) return;
  const res = await fetch("/api/branches", { headers: authHeaders() });
  if (res.ok) branches.value = (await res.json()).branches ?? [];
}

async function createBranch() {
  if (!token.value || !newBranchName.value) return;
  const res = await fetch("/api/branches", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name: newBranchName.value, address: newBranchAddress.value }),
  });
  if (res.ok) {
    newBranchName.value = "";
    newBranchAddress.value = "";
    await loadBranches();
  }
}

async function loadDirectors() {
  if (!token.value) return;
  const res = await fetch("/api/directors", { headers: authHeaders() });
  if (res.ok) directors.value = (await res.json()).directors ?? [];
}

async function createDirector() {
  if (!token.value || !newDirectorEmail.value || !newDirectorBranchId.value) return;
  const res = await fetch("/api/directors", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      email: newDirectorEmail.value,
      full_name: newDirectorName.value,
      branch_id: newDirectorBranchId.value,
    }),
  });
  if (res.ok) {
    const data = await res.json();
    newDirectorCredentials.value = {
      email: data.email,
      password: data.temporary_password,
      notice: data.credentials_notice,
    };
    newDirectorEmail.value = "";
    newDirectorName.value = "";
    await loadDirectors();
  }
}

async function loadLogs() {
  if (!token.value) return;
  const res = await fetch("/api/logs", { headers: authHeaders() });
  if (res.ok) logs.value = (await res.json()).logs ?? [];
}

async function loadAll() {
  await Promise.all([loadHealth(), loadBranches(), loadDirectors(), loadLogs()]);
}

function selectTab(tab: Tab) {
  activeTab.value = tab;
  clearInterval(healthTimer);
  if (tab === "health") {
    loadHealth();
    healthTimer = setInterval(loadHealth, 15000);
  }
}

function logout() {
  token.value = null;
  localStorage.removeItem("dosedu_token");
  clearInterval(healthTimer);
}

onMounted(async () => {
  await loadAll();
  if (token.value) healthTimer = setInterval(loadHealth, 15000);
});
onUnmounted(() => clearInterval(healthTimer));
</script>

<template>
  <header class="flex items-center justify-between border-b border-line px-4 py-3 sm:px-6">
    <div class="flex items-center gap-3">
      <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
      <span class="font-display text-base font-bold text-ink sm:text-lg">{{ t("app.title") }}</span>
    </div>
    <div class="flex items-center gap-2 sm:gap-3">
      <button
        v-if="token"
        type="button"
        class="rounded-lg bg-accent px-3 py-1.5 text-sm font-semibold text-accent-contrast hover:opacity-90"
        @click="logout"
      >
        ↩
      </button>
      <LangSwitcher />
      <ThemeToggle />
    </div>
  </header>

  <main class="px-4 py-6 sm:px-6">
    <form
      v-if="!token"
      class="mx-auto max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 shadow-sm sm:p-6"
      @submit.prevent="login"
    >
      <h2 class="font-display text-lg font-bold text-ink">{{ t("login.title") }}</h2>
      <label class="block text-sm text-ink-muted">
        {{ t("login.identifier") }}
        <input v-model="identifier" type="email" required class="mt-1" />
      </label>
      <label class="block text-sm text-ink-muted">
        {{ t("login.password") }}
        <input v-model="password" type="password" required minlength="10" maxlength="10" class="mt-1" />
      </label>
      <button
        type="submit"
        class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90"
      >
        {{ t("login.submit") }}
      </button>
      <p v-if="loginError" class="text-center text-sm text-danger">{{ t("login.error") }}</p>
    </form>

    <div v-else class="mx-auto max-w-4xl">
      <!-- Tabs -->
      <div class="flex gap-1 overflow-x-auto border-b border-line">
        <button
          v-for="tab in (['health', 'branches', 'directors', 'logs'] as Tab[])"
          :key="tab"
          type="button"
          class="font-accent shrink-0 border-b-2 px-4 py-2 text-sm font-medium"
          :class="activeTab === tab ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="selectTab(tab)"
        >
          {{ t(`nav.${tab}`) }}
        </button>
      </div>

      <!-- Health -->
      <section v-if="activeTab === 'health'" class="mt-4 space-y-4">
        <div v-if="health" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="rounded-xl border border-line bg-surface2 p-4">
            <div class="flex items-center gap-2 text-sm text-ink-muted">
              <span class="h-2 w-2 shrink-0 rounded-full" :class="statusColor(health.postgres)"></span>
              {{ t("health.postgres") }}
            </div>
            <p class="font-numeric mt-1 text-lg font-semibold text-ink">{{ health.postgres }}</p>
          </div>
          <div class="rounded-xl border border-line bg-surface2 p-4">
            <div class="flex items-center gap-2 text-sm text-ink-muted">
              <span class="h-2 w-2 shrink-0 rounded-full" :class="statusColor(health.redis)"></span>
              {{ t("health.redis") }}
            </div>
            <p class="font-numeric mt-1 text-lg font-semibold text-ink">{{ health.redis }}</p>
          </div>
          <div class="rounded-xl border border-line bg-surface2 p-4">
            <div class="flex items-center gap-2 text-sm text-ink-muted">
              <span class="h-2 w-2 shrink-0 rounded-full" :class="statusColor(health.main_site)"></span>
              {{ t("health.mainSite") }}
            </div>
            <p class="font-numeric mt-1 text-lg font-semibold text-ink">{{ health.main_site }}</p>
          </div>
          <div class="rounded-xl border border-line bg-surface2 p-4">
            <div class="flex items-center gap-2 text-sm text-ink-muted">
              <span class="h-2 w-2 shrink-0 rounded-full" :class="statusColor(health.app_site)"></span>
              {{ t("health.appSite") }}
            </div>
            <p class="font-numeric mt-1 text-lg font-semibold text-ink">{{ health.app_site }}</p>
          </div>
        </div>

        <div v-if="health" class="rounded-xl border border-line bg-surface2 p-5 shadow-sm">
          <h3 class="font-display mb-3 text-sm font-semibold text-ink">{{ t("health.title") }}</h3>
          <div class="space-y-3">
            <div v-for="metric in ([
              { label: t('health.cpu'), value: health.cpu_percent },
              { label: t('health.ram'), value: health.ram_percent },
              { label: t('health.disk'), value: health.disk_percent },
            ])" :key="metric.label">
              <div class="mb-1 flex items-center justify-between text-xs text-ink-muted">
                <span>{{ metric.label }}</span>
                <span class="font-numeric">{{ metric.value }}%</span>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-surface3">
                <div
                  class="h-full rounded-full bg-accent transition-all"
                  :style="{ width: Math.min(metric.value, 100) + '%' }"
                ></div>
              </div>
            </div>
          </div>
          <p class="font-numeric mt-4 text-xs text-ink-muted">
            {{ t("health.dbPool") }}: {{ health.db_pool.acquired }}/{{ health.db_pool.total }}
            ({{ t("health.idle") }}: {{ health.db_pool.idle }})
          </p>
        </div>
        <p v-else class="text-sm text-ink-muted">—</p>
      </section>

      <!-- Branches -->
      <section v-if="activeTab === 'branches'" class="mt-4 grid gap-4 sm:grid-cols-2">
        <form class="space-y-3 rounded-xl border border-line bg-surface2 p-5 shadow-sm" @submit.prevent="createBranch">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("branches.add") }}</h3>
          <label class="block text-sm text-ink-muted">
            {{ t("branches.name") }}
            <input v-model="newBranchName" required class="mt-1" />
          </label>
          <label class="block text-sm text-ink-muted">
            {{ t("branches.address") }}
            <input v-model="newBranchAddress" class="mt-1" />
          </label>
          <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast">
            {{ t("branches.create") }}
          </button>
        </form>

        <div class="space-y-2 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("branches.list") }}</h3>
          <div v-for="b in branches" :key="b.id" class="rounded-lg border border-line bg-surface1 p-3 text-sm">
            <p class="font-semibold text-ink">{{ b.name }}</p>
            <p class="text-ink-muted">{{ b.address }}</p>
            <span
              class="mt-1 inline-block rounded-full px-2 py-0.5 text-xs font-semibold"
              :class="b.status === 'active' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'"
            >
              {{ b.status }}
            </span>
          </div>
          <p v-if="!branches.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>

      <!-- Directors -->
      <section v-if="activeTab === 'directors'" class="mt-4 grid gap-4 sm:grid-cols-2">
        <div>
          <form class="space-y-3 rounded-xl border border-line bg-surface2 p-5 shadow-sm" @submit.prevent="createDirector">
            <h3 class="font-display text-sm font-semibold text-ink">{{ t("directors.add") }}</h3>
            <label class="block text-sm text-ink-muted">
              {{ t("directors.email") }}
              <input v-model="newDirectorEmail" type="email" required class="mt-1" />
            </label>
            <label class="block text-sm text-ink-muted">
              {{ t("directors.fullName") }}
              <input v-model="newDirectorName" required class="mt-1" />
            </label>
            <label class="block text-sm text-ink-muted">
              {{ t("directors.branch") }}
              <select v-model="newDirectorBranchId" required class="mt-1">
                <option value="" disabled>—</option>
                <option v-for="b in branches" :key="b.id" :value="b.id">{{ b.name }}</option>
              </select>
            </label>
            <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast">
              {{ t("directors.create") }}
            </button>
          </form>

          <div v-if="newDirectorCredentials" class="mt-3 rounded-xl border border-accent bg-surface2 p-4 text-sm">
            <p class="font-semibold text-ink">
              {{ newDirectorCredentials.email }} ·
              <code class="rounded bg-surface3 px-1.5 py-0.5">{{ newDirectorCredentials.password }}</code>
            </p>
            <p class="mt-1 text-ink-muted">{{ newDirectorCredentials.notice }}</p>
          </div>
        </div>

        <div class="space-y-2 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("directors.list") }}</h3>
          <div v-for="dir in directors" :key="dir.id" class="rounded-lg border border-line bg-surface1 p-3 text-sm">
            <p class="font-semibold text-ink">{{ dir.full_name || dir.email }}</p>
            <p class="text-ink-muted">{{ dir.email }}</p>
            <p class="text-xs text-ink-muted">{{ dir.branch_name }}</p>
          </div>
          <p v-if="!directors.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>

      <!-- Logs -->
      <section v-if="activeTab === 'logs'" class="mt-4 space-y-2 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
        <div v-for="log in logs" :key="log.id" class="border-b border-line py-2 text-sm last:border-0">
          <span
            class="mr-2 rounded px-1.5 py-0.5 text-xs font-semibold"
            :class="log.level === 'security' ? 'bg-danger/15 text-danger' : 'bg-surface3 text-ink-muted'"
          >
            {{ log.level }}
          </span>
          <span class="text-ink">{{ log.message }}</span>
          <span class="ml-2 text-xs text-ink-muted">{{ new Date(log.created_at).toLocaleString() }}</span>
        </div>
        <p v-if="!logs.length" class="text-sm text-ink-muted">—</p>
      </section>
    </div>
  </main>
</template>
