<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{ token: string }>();
const { t } = useI18n();

function authHeaders() {
  return { Authorization: `Bearer ${props.token}` };
}

type Subject = "english" | "chinese" | "mad" | "prodlenka";
type CourseType = "language" | "care_and_prep";
type StudentSummary = {
  id: string;
  full_name: string;
  course_type: CourseType;
  subject?: Subject;
  level: string;
  streak_days: number;
  balance: number;
  payment_status: string;
  subscription_expires_at: string;
};
const profile = ref<StudentSummary | null>(null);
const balance = ref<{ balance: number; payment_status: string } | null>(null);
const reportStatus = ref<"idle" | "sent">("idle");
type Certificate = { id: string; level: string; issued_at: string };
const certificates = ref<Certificate[]>([]);

// A student who has never had a subscription period set gets Go's
// zero-value timestamp ("0001-01-01T00:00:00Z") back from the API —
// a non-empty string, so a plain truthy check doesn't catch it. Only
// show a real, meaningfully-set expiry date.
const subscriptionExpiresAt = computed(() => {
  const raw = profile.value?.subscription_expires_at;
  if (!raw) return null;
  const d = new Date(raw);
  return d.getFullYear() > 1970 ? d : null;
});

// --- Daily log (attendance / homework / teacher note for the day) ---
type ChecklistItem = { label: string; done: boolean };
type DailyLog = {
  id: string;
  student_id: string;
  log_date: string;
  attendance_status: "present" | "absent" | "excused";
  teacher_note?: string;
  homework_status: "done" | "not_done" | "partial" | "n_a";
  // Optional, not yet written by any real teacher-side UI — the parent
  // card falls back to homework_status while this is absent/undefined.
  checklist?: ChecklistItem[];
  created_at: string;
};
const dailyLogs = ref<DailyLog[]>([]);

async function loadDailyLogs() {
  const res = await fetch("/api/daily-logs", { headers: authHeaders() });
  if (res.ok) dailyLogs.value = (await res.json()).logs ?? [];
}

// Backend returns the log history ordered most-recent-first, so [0] is
// "today's" (or the last logged day's) featured report.
const featuredLog = computed<DailyLog | null>(() => dailyLogs.value[0] ?? null);

// --- Today's 3-hour session status (care_and_prep only) ---
type TodayStatus = {
  checked_in_at: string | null;
  checked_out_at: string | null;
  expected_end_at: string | null;
};
const todayStatus = ref<TodayStatus | null>(null);

async function loadTodayStatus() {
  const res = await fetch("/api/today-status", { headers: authHeaders() });
  if (res.ok) todayStatus.value = await res.json();
}

// Live clock driving the elapsed/remaining progress bar — ticks every
// 30s and is torn down on unmount so it never leaks across route/tab
// changes.
const now = ref(Date.now());
let clockTimer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  clockTimer = setInterval(() => {
    now.value = Date.now();
  }, 30000);
});
onUnmounted(() => {
  if (clockTimer) clearInterval(clockTimer);
});

type SessionState = "not_checked_in" | "in_progress" | "checked_out";
const sessionState = computed<SessionState | null>(() => {
  const s = todayStatus.value;
  if (!s) return null;
  if (!s.checked_in_at) return "not_checked_in";
  if (!s.checked_out_at) return "in_progress";
  return "checked_out";
});

const sessionProgressPercent = computed(() => {
  const s = todayStatus.value;
  if (!s?.checked_in_at || !s?.expected_end_at) return 0;
  const start = new Date(s.checked_in_at).getTime();
  const end = new Date(s.expected_end_at).getTime();
  const span = end - start;
  if (span <= 0) return 100;
  return Math.min(100, Math.max(0, ((now.value - start) / span) * 100));
});

const sessionRemainingMinutes = computed(() => {
  const s = todayStatus.value;
  if (!s?.expected_end_at) return 0;
  const remainingMs = new Date(s.expected_end_at).getTime() - now.value;
  return Math.max(0, Math.round(remainingMs / 60000));
});

function formatTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

// --- 5-day (Mon-Fri) attendance calendar, built client-side from the
// daily-log history we already load — no new endpoint needed. ---
type WeekDay = { key: string; iso: string; status: DailyLog["attendance_status"] | null };
const weekdayKeys = ["mon", "tue", "wed", "thu", "fri"];

const weekDays = computed<WeekDay[]>(() => {
  const today = new Date();
  const dow = today.getDay(); // 0 = Sunday
  const mondayOffset = dow === 0 ? -6 : 1 - dow;
  const monday = new Date(today);
  monday.setDate(today.getDate() + mondayOffset);
  monday.setHours(0, 0, 0, 0);

  return weekdayKeys.map((key, i) => {
    const d = new Date(monday);
    d.setDate(monday.getDate() + i);
    const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const log = dailyLogs.value.find((l) => l.log_date.slice(0, 10) === iso);
    return { key, iso, status: log?.attendance_status ?? null };
  });
});

function calendarCellClass(status: DailyLog["attendance_status"] | null): string {
  switch (status) {
    case "present":
      return "bg-badge3 text-badge3-fg";
    case "absent":
      return "bg-danger/15 text-danger";
    case "excused":
      return "bg-badge4 text-badge4-fg";
    default:
      return "bg-surface3 text-ink-muted";
  }
}

async function loadParentData() {
  const [p, b, certs] = await Promise.all([
    fetch("/api/progress", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/balance", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/certificates", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
  ]);
  profile.value = p;
  balance.value = b;
  certificates.value = certs?.certificates ?? [];
  if (profile.value?.course_type === "care_and_prep") {
    await Promise.all([loadDailyLogs(), loadTodayStatus()]);
  }
}

const downloadError = ref("");

function downloadReport() {
  downloadError.value = "";
  fetch("/api/report/pdf", { headers: authHeaders() })
    .then(async (r) => {
      if (!r.ok) {
        const body = await r.json().catch(() => ({}));
        throw new Error(body.error ?? `HTTP ${r.status}`);
      }
      return r.blob();
    })
    .then((blob) => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "report.pdf";
      a.click();
      URL.revokeObjectURL(url);
    })
    .catch((err) => {
      downloadError.value = t("report.downloadError") + " (" + err.message + ")";
    });
}

function downloadCertificate(id: string) {
  downloadError.value = "";
  fetch(`/api/certificates/${id}/pdf`, { headers: authHeaders() })
    .then(async (r) => {
      if (!r.ok) {
        const body = await r.json().catch(() => ({}));
        throw new Error(body.error ?? `HTTP ${r.status}`);
      }
      return r.blob();
    })
    .then((blob) => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "certificate.pdf";
      a.click();
      URL.revokeObjectURL(url);
    })
    .catch((err) => {
      downloadError.value = t("report.downloadError") + " (" + err.message + ")";
    });
}

async function sendReportTelegram() {
  const res = await fetch("/api/report/telegram", { method: "POST", headers: authHeaders() });
  if (res.ok) reportStatus.value = "sent";
}

onMounted(loadParentData);
</script>

<template>
  <div class="grid gap-4 sm:grid-cols-2">
    <section v-if="profile" class="rounded-xl border border-line bg-surface2 p-5">
      <h2 class="font-display text-base font-bold text-ink">{{ t("progress.title") }}</h2>
      <p class="mt-2 text-sm font-semibold text-ink">{{ profile.full_name }}</p>
      <template v-if="profile.course_type === 'language'">
        <p v-if="profile.subject" class="text-sm text-ink-muted">{{ t("progress.subject") }}: {{ t(`subject.${profile.subject}`) }}</p>
        <p v-if="profile.level" class="text-sm text-ink-muted">{{ t("progress.level") }}: {{ profile.level }}</p>
      </template>
      <template v-else>
        <p v-if="profile.subject" class="text-sm text-ink-muted">{{ t("progress.subject") }}: {{ t(`subject.${profile.subject}`) }}</p>
      </template>
      <p class="text-sm text-ink-muted">🔥 {{ t("progress.streak") }}: {{ profile.streak_days }}</p>
    </section>

    <!-- Payment / subscription -->
    <section v-if="balance" class="rounded-xl border border-line bg-surface2 p-5">
      <h2 class="font-display text-base font-bold text-ink">{{ t("payment.title") }}</h2>
      <p class="font-numeric mt-2 text-2xl font-bold text-ink">{{ balance.balance }} ₸</p>
      <span
        class="mt-2 inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold"
        :class="
          balance.payment_status === 'paid'
            ? 'bg-badge3 text-badge3-fg'
            : balance.payment_status === 'pending'
              ? 'bg-badge4 text-badge4-fg'
              : 'bg-danger/15 text-danger'
        "
      >
        {{ t(`balance.${balance.payment_status}`) }}
      </span>
      <p v-if="subscriptionExpiresAt" class="mt-3 text-sm text-ink-muted">
        {{ t("payment.expiresAt") }}: {{ subscriptionExpiresAt.toLocaleDateString() }}
      </p>
    </section>

    <!-- 3-hour session status (care_and_prep only — a language parent has no such concept) -->
    <section v-if="profile?.course_type === 'care_and_prep'" class="rounded-xl border border-line bg-surface2 p-5 sm:col-span-2">
      <h2 class="font-display text-base font-bold text-ink">{{ t("todayStatus.title") }}</h2>

      <div v-if="sessionState === 'not_checked_in'" class="mt-3 flex items-center gap-2 text-sm text-ink-muted">
        <span class="h-2.5 w-2.5 rounded-full bg-surface3"></span>
        {{ t("todayStatus.notCheckedIn") }}
      </div>

      <div v-else-if="sessionState === 'in_progress'" class="mt-3">
        <div class="flex items-center gap-2 text-sm font-semibold text-ink">
          <span class="h-2.5 w-2.5 animate-pulse rounded-full bg-success"></span>
          {{ t("todayStatus.inProgress") }}
        </div>
        <p class="mt-1 text-sm text-ink-muted">
          {{ t("todayStatus.arrivedAt") }}: <span class="font-numeric text-ink">{{ formatTime(todayStatus?.checked_in_at) }}</span>
          · {{ t("todayStatus.expectedEnd") }}: <span class="font-numeric text-ink">{{ formatTime(todayStatus?.expected_end_at) }}</span>
        </p>
        <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-surface3">
          <div class="h-full rounded-full bg-accent transition-all duration-500" :style="{ width: sessionProgressPercent + '%' }"></div>
        </div>
        <p class="mt-1.5 text-xs text-ink-muted">{{ t("todayStatus.remaining") }}: {{ sessionRemainingMinutes }} {{ t("todayStatus.minutesShort") }}</p>
      </div>

      <div v-else-if="sessionState === 'checked_out'" class="mt-3 flex items-center gap-2 text-sm text-ink">
        <span class="h-2.5 w-2.5 rounded-full bg-accent-secondary"></span>
        {{ t("todayStatus.checkedOut") }}: <span class="font-numeric">{{ formatTime(todayStatus?.checked_out_at) }}</span>
      </div>
    </section>

    <!-- Daily progress: featured (most recent) report + compact history -->
    <section v-if="profile?.course_type === 'care_and_prep'" class="rounded-xl border border-line bg-surface2 p-5 sm:col-span-2">
      <h2 class="font-display text-base font-bold text-ink">{{ t("dailyLog.title") }}</h2>

      <div v-if="featuredLog" class="mt-3 rounded-lg border border-line bg-surface1 p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="font-numeric text-sm font-semibold text-ink">{{ featuredLog.log_date }}</p>
          <span
            class="rounded-full px-2.5 py-0.5 text-xs font-semibold"
            :class="
              featuredLog.attendance_status === 'present'
                ? 'bg-badge3 text-badge3-fg'
                : featuredLog.attendance_status === 'excused'
                  ? 'bg-badge4 text-badge4-fg'
                  : 'bg-danger/15 text-danger'
            "
          >
            {{ t(`dailyLog.attendance.${featuredLog.attendance_status}`) }}
          </span>
        </div>

        <!-- Checklist, when the teacher-side entry has written one; otherwise fall back to the single homework badge. -->
        <ul v-if="featuredLog.checklist && featuredLog.checklist.length" class="mt-3 space-y-1.5">
          <li v-for="(item, idx) in featuredLog.checklist" :key="idx" class="flex items-center gap-2 text-sm">
            <span
              class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-xs"
              :class="item.done ? 'bg-badge3 text-badge3-fg' : 'bg-surface3 text-ink-muted'"
            >
              {{ item.done ? "✓" : "" }}
            </span>
            <span :class="item.done ? 'text-ink' : 'text-ink-muted'">{{ item.label }}</span>
          </li>
        </ul>
        <div v-else class="mt-3">
          <span class="text-xs font-medium text-ink-muted">{{ t("dailyLog.homeworkLabel") }}:</span>
          <span
            class="ml-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold"
            :class="featuredLog.homework_status === 'done' ? 'bg-badge3 text-badge3-fg' : 'bg-badge4 text-badge4-fg'"
          >
            {{ t(`dailyLog.homework.${featuredLog.homework_status}`) }}
          </span>
        </div>

        <p v-if="featuredLog.teacher_note" class="mt-3 border-l-2 border-accent bg-surface2 py-2 pl-3 text-sm italic text-ink-muted">
          “{{ featuredLog.teacher_note }}”
        </p>
      </div>
      <p v-else class="mt-3 text-sm text-ink-muted">{{ t("dailyLog.empty") }}</p>

      <details v-if="dailyLogs.length > 1" class="mt-4">
        <summary class="cursor-pointer text-xs font-medium text-ink-muted hover:text-ink">{{ t("dailyLog.historyTitle") }}</summary>
        <div class="mt-2 overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-line text-xs text-ink-muted">
                <th class="py-1.5 pr-3 font-medium">{{ t("dailyLog.date") }}</th>
                <th class="py-1.5 pr-3 font-medium">{{ t("dailyLog.attendanceLabel") }}</th>
                <th class="py-1.5 pr-3 font-medium">{{ t("dailyLog.note") }}</th>
                <th class="py-1.5 font-medium">{{ t("dailyLog.homeworkLabel") }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="log in dailyLogs" :key="log.id" class="border-b border-line last:border-0">
                <td class="py-1.5 pr-3 text-ink-muted">{{ log.log_date }}</td>
                <td class="py-1.5 pr-3 text-ink">{{ t(`dailyLog.attendance.${log.attendance_status}`) }}</td>
                <td class="py-1.5 pr-3 text-ink-muted">{{ log.teacher_note || "—" }}</td>
                <td class="py-1.5 text-ink">{{ t(`dailyLog.homework.${log.homework_status}`) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </details>
    </section>

    <!-- 5-day (Mon-Fri) attendance calendar, from the same daily-log history -->
    <section v-if="profile?.course_type === 'care_and_prep'" class="rounded-xl border border-line bg-surface2 p-5">
      <h2 class="font-display text-base font-bold text-ink">{{ t("calendar.title") }}</h2>
      <div class="mt-3 grid grid-cols-5 gap-1.5 sm:gap-2">
        <div v-for="day in weekDays" :key="day.iso" class="flex flex-col items-center gap-1">
          <span class="text-[11px] font-medium text-ink-muted">{{ t(`calendar.weekday.${day.key}`) }}</span>
          <div class="flex h-9 w-full items-center justify-center rounded-lg text-xs font-semibold" :class="calendarCellClass(day.status)">
            <span v-if="day.status === 'present'">✓</span>
            <span v-else-if="day.status === 'absent'">✕</span>
            <span v-else-if="day.status === 'excused'">!</span>
            <span v-else>·</span>
          </div>
        </div>
      </div>
      <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-ink-muted">
        <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-badge3"></span>{{ t("calendar.legend.present") }}</span>
        <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-danger/40"></span>{{ t("calendar.legend.absent") }}</span>
        <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-badge4"></span>{{ t("calendar.legend.excused") }}</span>
        <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-surface3"></span>{{ t("calendar.legend.noData") }}</span>
      </div>
    </section>

    <!-- Pickup & Safety — no backend yet (deliberately deferred), polished coming-soon placeholder -->
    <section v-if="profile?.course_type === 'care_and_prep'" class="relative overflow-hidden rounded-xl border border-line bg-surface2 p-5 opacity-80">
      <span class="absolute right-4 top-4 rounded-full bg-badge1 px-2.5 py-0.5 text-[11px] font-semibold text-badge1-fg">{{ t("pickup.comingSoon") }}</span>
      <h2 class="font-display pr-24 text-base font-bold text-ink">{{ t("pickup.title") }}</h2>
      <p class="mt-2 text-sm text-ink-muted">{{ t("pickup.description") }}</p>
      <ul class="mt-3 space-y-1.5 text-sm text-ink-muted">
        <li class="flex items-start gap-2"><span class="mt-0.5">•</span>{{ t("pickup.featureWarning") }}</li>
        <li class="flex items-start gap-2"><span class="mt-0.5">•</span>{{ t("pickup.featureAuthorizedList") }}</li>
      </ul>
      <button type="button" disabled class="mt-4 w-full cursor-not-allowed rounded-lg bg-surface3 px-4 py-2 text-sm font-semibold text-ink-muted">
        {{ t("pickup.comingSoon") }}
      </button>
    </section>

    <section class="rounded-xl border border-line bg-surface2 p-5 sm:col-span-2">
      <h2 class="font-display text-base font-bold text-ink">{{ t("report.title") }}</h2>
      <div class="mt-3 flex flex-wrap gap-2">
        <button type="button" class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="downloadReport">
          {{ t("report.download") }}
        </button>
        <button type="button" class="rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="sendReportTelegram">
          {{ t("report.sendTelegram") }}
        </button>
      </div>
      <p v-if="reportStatus === 'sent'" class="mt-2 text-sm text-success">{{ t("report.sent") }}</p>
      <p v-if="downloadError" class="mt-2 text-sm text-danger">{{ downloadError }}</p>
    </section>

    <section v-if="certificates.length" class="rounded-xl border border-line bg-surface2 p-5 sm:col-span-2">
      <h2 class="font-display text-base font-bold text-ink">{{ t("portfolio.title") }}</h2>
      <div class="mt-3 grid gap-2 sm:grid-cols-2">
        <div v-for="cert in certificates" :key="cert.id" class="flex items-center justify-between rounded-lg border border-line bg-surface1 px-3 py-2">
          <div>
            <p class="text-sm font-semibold text-ink">{{ cert.level }}</p>
            <p class="text-xs text-ink-muted">{{ new Date(cert.issued_at).toLocaleDateString() }}</p>
          </div>
          <button type="button" class="rounded-md bg-accent px-2.5 py-1 text-xs font-semibold text-accent-contrast hover:opacity-90" @click="downloadCertificate(cert.id)">
            PDF
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
