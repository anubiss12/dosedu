<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";

const { t } = useI18n();

const token = ref<string | null>(localStorage.getItem("dosedu_token"));
const identifier = ref("");
const password = ref("");
const loginError = ref(false);

type Tab = "schedule" | "gradebook" | "payment" | "tests" | "certificate";
const activeTab = ref<Tab>("schedule");

function authHeaders() {
  return { Authorization: `Bearer ${token.value}` };
}

// --- JWT claims (client-side decode, display-only — the server is the
// real authority on language_scope enforcement) ---
type Claims = { uid: string; role: string; branch_id?: string; language_scope?: string };
function decodeToken(t: string | null): Claims | null {
  if (!t) return null;
  try {
    const payload = t.split(".")[1];
    return JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/")));
  } catch {
    return null;
  }
}
const claims = computed(() => decodeToken(token.value));
const languageScope = computed(() => claims.value?.language_scope ?? "");

// A mad/prodlenka teacher (no language_scope) has attendance-only
// access — no test-upload or certificate tabs at all.
const visibleTabs = computed<Tab[]>(() =>
  languageScope.value
    ? ["schedule", "gradebook", "payment", "tests", "certificate"]
    : ["schedule", "gradebook", "payment"],
);

const englishLevels = ["Beginner", "Elementary", "Intermediate", "Upper-Intermediate", "Advanced"];
const chineseLevels = ["HSK1", "HSK2", "HSK3", "HSK4", "HSK5", "HSK6"];
const availableLevels = computed(() => (languageScope.value === "zh" ? chineseLevels : englishLevels));
const weekdayNames = ["", "Дс", "Сс", "Ср", "Бс", "Жм", "Сб", "Жс"];

// --- Schedule ---
type ScheduleSlot = { id: string; group_id: string; room_id: string; weekday: number; start_time: string; end_time: string };
const mySlots = ref<ScheduleSlot[]>([]);
const myGroupIds = computed(() => [...new Set(mySlots.value.map((s) => s.group_id))]);

async function loadSchedule() {
  if (!token.value) return;
  const res = await fetch("/api/schedule", { headers: authHeaders() });
  if (res.ok) mySlots.value = (await res.json()).schedule ?? [];
}

// --- Gradebook ---
type GradebookRow = { student_id: string; full_name: string; status: string; grade: number | null };
const gradebookGroupId = ref("");
const gradebookDate = ref(new Date().toISOString().slice(0, 10));
const gradebookRows = ref<GradebookRow[]>([]);

async function loadGradebook() {
  if (!token.value || !gradebookGroupId.value) return;
  const res = await fetch(`/api/groups/${gradebookGroupId.value}/gradebook?date=${gradebookDate.value}`, { headers: authHeaders() });
  if (res.ok) gradebookRows.value = (await res.json()).students ?? [];
}

async function markStudent(row: GradebookRow, status: string) {
  if (!token.value) return;
  await fetch("/api/attendance", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ student_id: row.student_id, status, grade: row.grade, date: gradebookDate.value }),
  });
  row.status = status;
}

async function saveGrade(row: GradebookRow) {
  if (!token.value) return;
  await fetch("/api/attendance", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ student_id: row.student_id, status: row.status || "present", grade: row.grade, date: gradebookDate.value }),
  });
}

// --- Payment confirmation ---
const paymentStudentId = ref("");
const paymentAmount = ref<number | null>(null);
const paymentStart = ref(new Date().toISOString().slice(0, 10));
const paymentEnd = ref(new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10));
const paymentStatus = ref<"idle" | "success" | "error">("idle");

async function confirmPayment() {
  if (!token.value || !paymentStudentId.value || !paymentAmount.value) return;
  paymentStatus.value = "idle";
  const res = await fetch(`/api/payments/${paymentStudentId.value}/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ amount: paymentAmount.value, period_start: paymentStart.value, period_end: paymentEnd.value }),
  });
  paymentStatus.value = res.ok ? "success" : "error";
  if (res.ok) {
    paymentStudentId.value = "";
    paymentAmount.value = null;
  }
}

// --- Test upload: real question bank, scoped to the teacher's own
// language, with an official (one-time level-advancement) flag ---
const level = ref(availableLevels.value[0]);
const isOfficial = ref(false);
const file = ref<File | null>(null);
const uploading = ref(false);
type UploadError = { row: number; message: string };
const errorLog = ref<UploadError[]>([]);
const uploadStatus = ref<"idle" | "ok" | "failed">("idle");
const questionsAdded = ref(0);

type UploadHistoryItem = {
  id: string;
  level: string;
  file_name: string;
  status: string;
  error_log: UploadError[] | null;
  created_at: string;
};
const uploadHistory = ref<UploadHistoryItem[]>([]);

async function loadUploadHistory() {
  if (!token.value) return;
  const res = await fetch("/api/tests/uploads", { headers: authHeaders() });
  if (res.ok) uploadHistory.value = (await res.json()).uploads ?? [];
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement;
  file.value = input.files?.[0] ?? null;
}

async function uploadTest() {
  if (!file.value || !token.value) return;
  uploading.value = true;
  errorLog.value = [];
  questionsAdded.value = 0;
  uploadStatus.value = "idle";
  const form = new FormData();
  form.append("file", file.value);
  try {
    const res = await fetch(`/api/tests/${encodeURIComponent(level.value)}/upload?official=${isOfficial.value}`, {
      method: "POST",
      headers: authHeaders(),
      body: form,
    });
    const data = await res.json();
    uploadStatus.value = data.status === "ok" ? "ok" : "failed";
    errorLog.value = data.error_log ?? [];
    questionsAdded.value = data.questions_added ?? 0;
    await loadUploadHistory();
  } catch {
    uploadStatus.value = "failed";
  } finally {
    uploading.value = false;
  }
}

// --- Certificate (existing) ---
const certStudentId = ref("");
const certLevel = ref(availableLevels.value[0]);
const certStatus = ref<"idle" | "success" | "error">("idle");
const issuingCert = ref(false);

async function issueCertificate() {
  if (!token.value || !certStudentId.value) return;
  issuingCert.value = true;
  certStatus.value = "idle";
  try {
    const res = await fetch("/api/certificates", {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify({ student_id: certStudentId.value, level: certLevel.value }),
    });
    certStatus.value = res.ok ? "success" : "error";
    if (res.ok) certStudentId.value = "";
  } catch {
    certStatus.value = "error";
  } finally {
    issuingCert.value = false;
  }
}

async function selectTab(tab: Tab) {
  activeTab.value = tab;
  if (tab === "schedule" || tab === "gradebook") await loadSchedule();
  if (tab === "tests") await loadUploadHistory();
}

async function login() {
  loginError.value = false;
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifier: identifier.value, password: password.value, role: "teacher" }),
    });
    if (!res.ok) throw new Error("invalid");
    const data = await res.json();
    token.value = data.token;
    localStorage.setItem("dosedu_token", data.token);
    level.value = availableLevels.value[0];
    certLevel.value = availableLevels.value[0];
    activeTab.value = "schedule"; // reset in case the previous session left it on a tab this account can't see
    await loadSchedule();
  } catch {
    loginError.value = true;
  }
}

function logout() {
  token.value = null;
  localStorage.removeItem("dosedu_token");
}

onMounted(loadSchedule);
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
    <form v-if="!token" class="mx-auto max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 shadow-sm sm:p-6" @submit.prevent="login">
      <h2 class="font-display text-lg font-bold text-ink">{{ t("login.title") }}</h2>
      <label class="block text-sm text-ink-muted">
        {{ t("login.identifier") }}
        <input v-model="identifier" type="email" required class="mt-1" />
      </label>
      <label class="block text-sm text-ink-muted">
        {{ t("login.password") }}
        <input v-model="password" type="password" required minlength="6" class="mt-1" />
      </label>
      <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90">
        {{ t("login.submit") }}
      </button>
      <p v-if="loginError" class="text-center text-sm text-danger">{{ t("login.error") }}</p>
    </form>

    <div v-else>
      <div class="flex gap-1 overflow-x-auto border-b border-line">
        <button
          v-for="tab in visibleTabs"
          :key="tab"
          type="button"
          class="font-accent shrink-0 border-b-2 px-4 py-2 text-sm font-medium"
          :class="activeTab === tab ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="selectTab(tab)"
        >
          {{ t(`tabs.${tab}`) }}
        </button>
      </div>

      <section v-if="activeTab === 'schedule'" class="mt-4 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
        <h2 class="font-display text-sm font-semibold text-ink">{{ t("nav.schedule") }}</h2>
        <div v-for="slot in mySlots" :key="slot.id" class="mt-2 border-b border-line py-2 text-sm last:border-0">
          <span class="font-semibold text-ink">{{ weekdayNames[slot.weekday] }} {{ slot.start_time }}–{{ slot.end_time }}</span>
          <span class="text-ink-muted"> · {{ t("schedule.room") }}: {{ slot.room_id }}</span>
        </div>
        <p v-if="!mySlots.length" class="mt-2 text-sm text-ink-muted">—</p>
      </section>

      <section v-if="activeTab === 'gradebook'" class="mt-4 space-y-4">
        <div class="flex flex-wrap gap-2 rounded-xl border border-line bg-surface2 p-4">
          <select v-model="gradebookGroupId" class="text-sm">
            <option value="" disabled>{{ t("gradebook.group") }}</option>
            <option v-for="gid in myGroupIds" :key="gid" :value="gid">{{ gid.slice(0, 8) }}…</option>
          </select>
          <input v-model="gradebookDate" type="date" class="text-sm" />
          <button type="button" class="rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-contrast" @click="loadGradebook">
            {{ t("gradebook.load") }}
          </button>
        </div>

        <div class="space-y-2">
          <div v-for="row in gradebookRows" :key="row.student_id" class="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-line bg-surface1 p-3">
            <span class="text-sm font-medium text-ink">{{ row.full_name }}</span>
            <div class="flex items-center gap-2">
              <select :value="row.status" class="text-xs" @change="markStudent(row, ($event.target as HTMLSelectElement).value)">
                <option value="">—</option>
                <option value="present">{{ t("gradebook.present") }}</option>
                <option value="absent">{{ t("gradebook.absent") }}</option>
                <option value="excused">{{ t("gradebook.excused") }}</option>
              </select>
              <input v-model.number="row.grade" type="number" min="0" max="100" class="w-16 text-xs" :placeholder="t('gradebook.grade')" @blur="saveGrade(row)" />
            </div>
          </div>
          <p v-if="!gradebookRows.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>

      <section v-if="activeTab === 'payment'" class="mt-4 max-w-sm space-y-3 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
        <h2 class="font-display text-sm font-semibold text-ink">{{ t("payment.title") }}</h2>
        <label class="block text-sm text-ink-muted">{{ t("payment.studentId") }}<input v-model="paymentStudentId" placeholder="UUID" class="mt-1" /></label>
        <label class="block text-sm text-ink-muted">{{ t("payment.amount") }}<input v-model.number="paymentAmount" type="number" class="mt-1" /></label>
        <div class="grid grid-cols-2 gap-2">
          <label class="block text-xs text-ink-muted">{{ t("payment.periodStart") }}<input v-model="paymentStart" type="date" class="mt-1" /></label>
          <label class="block text-xs text-ink-muted">{{ t("payment.periodEnd") }}<input v-model="paymentEnd" type="date" class="mt-1" /></label>
        </div>
        <button type="button" class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast" @click="confirmPayment">
          {{ t("payment.confirm") }}
        </button>
        <p v-if="paymentStatus === 'success'" class="text-sm text-success">✅ {{ t("payment.success") }}</p>
        <p v-if="paymentStatus === 'error'" class="text-sm text-danger">{{ t("payment.error") }}</p>
      </section>

      <section v-if="activeTab === 'tests'" class="mt-4 max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
        <div class="flex items-center justify-between">
          <h2 class="font-display text-sm font-semibold text-ink">{{ t("testUpload.title") }}</h2>
          <span class="font-accent rounded-full bg-badge1 px-2.5 py-0.5 text-xs font-semibold text-badge1-fg">
            {{ languageScope === "zh" ? "中文" : "EN" }}
          </span>
        </div>
        <label class="block text-sm text-ink-muted">
          {{ t("testUpload.level") }}
          <select v-model="level" class="mt-1">
            <option v-for="lvl in availableLevels" :key="lvl" :value="lvl">{{ lvl }}</option>
          </select>
        </label>
        <label class="block text-sm text-ink-muted">
          {{ t("testUpload.chooseFile") }}
          <input type="file" accept=".csv,.xlsx" class="mt-1" @change="onFileChange" />
        </label>
        <label class="flex items-center gap-2 text-sm text-ink-muted">
          <input v-model="isOfficial" type="checkbox" class="!w-auto h-4 w-4 shrink-0" />
          {{ t("testUpload.official") }}
        </label>
        <button
          type="button"
          :disabled="!file || uploading"
          class="font-accent w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-60"
          @click="uploadTest"
        >
          {{ t("testUpload.upload") }}
        </button>
        <div v-if="uploadStatus === 'failed' && errorLog.length">
          <h3 class="text-sm font-semibold text-ink">{{ t("testUpload.errorLog") }}</h3>
          <ul class="mt-1 list-disc space-y-1 pl-5 text-xs text-danger">
            <li v-for="err in errorLog" :key="err.row">{{ t("testUpload.row") }} {{ err.row }}: {{ err.message }}</li>
          </ul>
        </div>
        <p v-else-if="uploadStatus === 'ok'" class="text-sm text-success">
          ✅ {{ t("testUpload.success") }} ({{ t("testUpload.questionsAdded") }}: {{ questionsAdded }})
        </p>

        <div v-if="uploadHistory.length" class="border-t border-line pt-3">
          <h3 class="text-sm font-semibold text-ink">{{ t("testUpload.history") }}</h3>
          <div v-for="u in uploadHistory" :key="u.id" class="mt-2 rounded-lg border border-line bg-surface1 p-2.5 text-xs">
            <div class="flex items-center justify-between">
              <span class="font-medium text-ink">{{ u.file_name }} — {{ u.level }}</span>
              <span
                class="font-accent rounded-full px-2 py-0.5 font-semibold"
                :class="u.status === 'ok' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'"
              >
                {{ u.status === "ok" ? "OK" : t("testUpload.errorLog") }}
              </span>
            </div>
            <ul v-if="u.error_log && u.error_log.length" class="mt-1 list-disc space-y-0.5 pl-4 text-danger">
              <li v-for="err in u.error_log" :key="err.row">{{ t("testUpload.row") }} {{ err.row }}: {{ err.message }}</li>
            </ul>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'certificate'" class="mt-4 max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 shadow-sm">
        <h2 class="font-display text-sm font-semibold text-ink">{{ t("certificate.title") }}</h2>
        <label class="block text-sm text-ink-muted">{{ t("certificate.studentId") }}<input v-model="certStudentId" placeholder="UUID" class="mt-1" /></label>
        <label class="block text-sm text-ink-muted">
          {{ t("testUpload.level") }}
          <select v-model="certLevel" class="mt-1">
            <option v-for="lvl in availableLevels" :key="lvl" :value="lvl">{{ lvl }}</option>
          </select>
        </label>
        <button
          type="button"
          :disabled="!certStudentId || issuingCert"
          class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-60"
          @click="issueCertificate"
        >
          {{ t("certificate.issue") }}
        </button>
        <p v-if="certStatus === 'success'" class="text-sm text-success">{{ t("certificate.success") }}</p>
        <p v-if="certStatus === 'error'" class="text-sm text-danger">{{ t("certificate.error") }}</p>
      </section>
    </div>
  </main>
</template>
