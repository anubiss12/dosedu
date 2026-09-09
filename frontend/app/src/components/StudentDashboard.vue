<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{ token: string }>();
const { t } = useI18n();

function authHeaders() {
  return { Authorization: `Bearer ${props.token}` };
}

// --- Profile/progress ---
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

async function loadProfile() {
  const res = await fetch("/api/progress", { headers: authHeaders() });
  if (res.ok) profile.value = await res.json();
}

// --- Daily Log (care_and_prep students — mad/prodlenka groups have no
// tests, just attendance/homework notes from the teacher). ---
type DailyLog = {
  id: string;
  student_id: string;
  log_date: string;
  attendance_status: "present" | "absent" | "excused";
  teacher_note?: string;
  homework_status: "done" | "not_done" | "partial" | "n_a";
  created_at: string;
};
const dailyLogs = ref<DailyLog[]>([]);

async function loadDailyLogs() {
  const res = await fetch("/api/daily-logs", { headers: authHeaders() });
  if (res.ok) dailyLogs.value = (await res.json()).logs ?? [];
}

// --- Practice (unlimited retakes) and official (one-shot per
// assignment) tests, enforced server-side. ---
type Question = { id: string; question: string; option_a: string; option_b: string; option_c?: string; option_d?: string };
type QuizResultItem = { question_id: string; question: string; options: string[]; selected: number; correct: number; is_correct: boolean };
type QuizResult = { id: string; score: number; total: number; results: QuizResultItem[] };
type QuizStatus = "idle" | "loading" | "active" | "submitting" | "result" | "locked" | "empty";

function questionOptions(q: Question): string[] {
  return [q.option_a, q.option_b, q.option_c, q.option_d].filter((o): o is string => !!o);
}

// Practice
const practiceStatus = ref<QuizStatus>("idle");
const practiceQuestions = ref<Question[]>([]);
const practiceIndex = ref(0);
const practiceAnswers = ref<Record<string, number>>({});
const practiceResult = ref<QuizResult | null>(null);
const practiceMessage = ref("");

async function startPractice() {
  practiceStatus.value = "loading";
  practiceResult.value = null;
  practiceAnswers.value = {};
  practiceIndex.value = 0;
  const res = await fetch("/api/practice/questions", { headers: authHeaders() });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    practiceMessage.value = body.message ?? "";
    practiceStatus.value = "locked";
    return;
  }
  if (!res.ok) {
    practiceStatus.value = "idle";
    return;
  }
  const body = await res.json();
  practiceQuestions.value = body.questions ?? [];
  practiceStatus.value = practiceQuestions.value.length ? "active" : "empty";
}

async function pickPracticeAnswer(optionIndex: number) {
  const q = practiceQuestions.value[practiceIndex.value];
  practiceAnswers.value[q.id] = optionIndex;
  if (practiceIndex.value + 1 < practiceQuestions.value.length) {
    practiceIndex.value++;
  } else {
    await submitPractice();
  }
}

async function submitPractice() {
  practiceStatus.value = "submitting";
  const answers = Object.entries(practiceAnswers.value).map(([question_id, selected]) => ({ question_id, selected }));
  const res = await fetch("/api/practice/submit", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ answers }),
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    practiceMessage.value = body.message ?? "";
    practiceStatus.value = "locked";
    return;
  }
  if (!res.ok) {
    practiceStatus.value = "active";
    return;
  }
  practiceResult.value = await res.json();
  practiceStatus.value = "result";
  await loadPracticeHistory();
  await loadProfile(); // streak may have advanced
}

type PracticeAttempt = { id: string; kind: string; subject: string; level: string; score: number; total: number; taken_at: string };
const practiceHistory = ref<PracticeAttempt[]>([]);

async function loadPracticeHistory() {
  const res = await fetch("/api/practice/history", { headers: authHeaders() });
  if (res.ok) practiceHistory.value = (await res.json()).attempts ?? [];
}

// Deңгей прогресі: real average from the last N practice attempts —
// no invented vocabulary-percentage, derived from practiceHistory only.
const recentAttempts = computed(() =>
  [...practiceHistory.value].sort((a, b) => new Date(b.taken_at).getTime() - new Date(a.taken_at).getTime()).slice(0, 5)
);
const levelProgressPercent = computed(() => {
  const attempts = recentAttempts.value;
  const totalMax = attempts.reduce((sum, a) => sum + a.total, 0);
  if (!attempts.length || totalMax === 0) return null;
  const totalScore = attempts.reduce((sum, a) => sum + a.score, 0);
  return Math.round((totalScore / totalMax) * 100);
});

// --- Schedule (new: GET /family/schedule via /api/* rewrite) ---
type ScheduleSlot = { id: string; group_id: string; teacher_id: string; room_id: string; weekday: number; start_time: string; end_time: string };
const scheduleSlots = ref<ScheduleSlot[]>([]);
const scheduleLoaded = ref(false);

async function loadSchedule() {
  const res = await fetch("/api/schedule", { headers: authHeaders() });
  if (res.ok) scheduleSlots.value = (await res.json()).schedule ?? [];
  scheduleLoaded.value = true;
}

const sortedSchedule = computed(() =>
  [...scheduleSlots.value].sort((a, b) => a.weekday - b.weekday || a.start_time.localeCompare(b.start_time))
);

function shortTime(hhmmss: string): string {
  return hhmmss.slice(0, 5);
}

// --- QR check-in code (new: GET /family/qr-code via /api/* rewrite) ---
type QrCode = { qr_token: string; qr_image_base64: string };
const qrCode = ref<QrCode | null>(null);
const qrLoaded = ref(false);

async function loadQrCode() {
  const res = await fetch("/api/qr-code", { headers: authHeaders() });
  if (res.ok) qrCode.value = await res.json();
  qrLoaded.value = true;
}

async function loadScheduleTab() {
  scheduleLoaded.value = false;
  qrLoaded.value = false;
  await Promise.all([loadSchedule(), loadQrCode()]);
}

// Official (one-shot per assignment)
type TestAssignment = {
  id: string;
  group_id: string;
  subject: string;
  level: string;
  attempts_allowed: number;
  question_ids: string[];
  created_at: string;
};
const activeAssignments = ref<TestAssignment[]>([]);

async function loadActiveAssignments() {
  const res = await fetch("/api/test-assignments/active", { headers: authHeaders() });
  if (res.ok) activeAssignments.value = (await res.json()).assignments ?? [];
}

const activeAssignmentId = ref<string | null>(null);
const officialStatus = ref<QuizStatus>("idle");
const officialQuestions = ref<Question[]>([]);
const officialIndex = ref(0);
const officialAnswers = ref<Record<string, number>>({});
const officialResult = ref<QuizResult | null>(null);
const officialMessage = ref("");

async function startAssignment(assignment: TestAssignment) {
  activeAssignmentId.value = assignment.id;
  officialStatus.value = "loading";
  officialResult.value = null;
  officialAnswers.value = {};
  officialIndex.value = 0;
  const res = await fetch(`/api/test-assignments/${assignment.id}/questions`, { headers: authHeaders() });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    officialMessage.value = body.message ?? "";
    officialStatus.value = "locked";
    return;
  }
  if (!res.ok) {
    officialStatus.value = "idle";
    return;
  }
  const body = await res.json();
  officialQuestions.value = body.questions ?? [];
  officialStatus.value = officialQuestions.value.length ? "active" : "empty";
}

async function pickOfficialAnswer(optionIndex: number) {
  const q = officialQuestions.value[officialIndex.value];
  officialAnswers.value[q.id] = optionIndex;
  if (officialIndex.value + 1 < officialQuestions.value.length) {
    officialIndex.value++;
  } else {
    await submitAssignment();
  }
}

async function submitAssignment() {
  if (!activeAssignmentId.value) return;
  officialStatus.value = "submitting";
  const answers = Object.entries(officialAnswers.value).map(([question_id, selected]) => ({ question_id, selected }));
  const res = await fetch(`/api/test-assignments/${activeAssignmentId.value}/submit`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ answers }),
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    officialMessage.value = body.message ?? "";
    officialStatus.value = "locked";
    await loadActiveAssignments();
    return;
  }
  if (!res.ok) {
    officialStatus.value = "active";
    return;
  }
  officialResult.value = await res.json();
  officialStatus.value = "result";
  await loadActiveAssignments();
}

function backToAssignments() {
  activeAssignmentId.value = null;
  officialStatus.value = "idle";
  loadActiveAssignments();
}

// --- AI: "explain my mistake", in whichever language the student picks ---
const explainLang = ref<"kk" | "ru" | "en" | "zh">("kk");
const explanations = ref<Record<string, string>>({});
const explainLoading = ref<Record<string, boolean>>({});

async function explainMistake(item: QuizResultItem) {
  if (explanations.value[item.question_id] || !profile.value?.subject) return;
  explainLoading.value[item.question_id] = true;
  const res = await fetch("/api/tests/explain", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      question: item.question,
      chosen_answer: item.options[item.selected] ?? "",
      correct_answer: item.options[item.correct],
      subject: profile.value.subject,
      explain_language: explainLang.value,
    }),
  });
  explainLoading.value[item.question_id] = false;
  if (res.ok) {
    const body = await res.json();
    explanations.value[item.question_id] = body.explanation;
  }
}

type StudentTab = "practice" | "official" | "results" | "schedule";
const studentTab = ref<StudentTab>("practice");

onMounted(async () => {
  await loadProfile();
  if (profile.value?.course_type === "care_and_prep") {
    await loadDailyLogs();
  } else if (profile.value?.course_type === "language" && profile.value.subject && profile.value.level) {
    await Promise.all([loadPracticeHistory(), loadActiveAssignments()]);
  }
});
</script>

<template>
  <div v-if="profile">
    <!-- care_and_prep (mad/prodlenka): no tests — attendance/homework log instead -->
    <section v-if="profile.course_type === 'care_and_prep'" class="rounded-xl border border-line bg-surface2 p-5">
      <h2 class="font-display text-base font-bold text-ink">{{ t("dailyLog.title") }}</h2>
      <div v-if="dailyLogs.length" class="mt-3 overflow-x-auto">
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
      <p v-else class="mt-3 text-sm text-ink-muted">{{ t("dailyLog.empty") }}</p>
    </section>

    <!-- language: not yet assigned by a teacher/director -->
    <section v-else-if="!profile.subject || !profile.level" class="mx-auto max-w-sm rounded-xl border border-line bg-surface2 p-5 text-center">
      <h2 class="font-display text-base font-bold text-ink">{{ t("assignment.notAssignedTitle") }}</h2>
      <p class="mt-1 text-xs text-ink-muted">{{ t("assignment.notAssignedHint") }}</p>
    </section>

    <!-- language: assigned — practice / official / results -->
    <template v-else>
      <!-- Streak & daily goal banner -->
      <section class="mb-4 overflow-hidden rounded-xl border border-line bg-gradient-to-br from-badge4/40 via-surface2 to-surface2 p-5">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div class="flex items-center gap-3">
            <span class="font-numeric text-4xl leading-none">🔥</span>
            <div>
              <p class="font-display text-lg font-bold text-ink">{{ t("streakBanner.message", { days: profile.streak_days }) }}</p>
            </div>
          </div>
          <button
            type="button"
            class="shrink-0 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90"
            @click="studentTab = 'practice'; if (practiceStatus === 'idle') startPractice();"
          >
            {{ t("streakBanner.cta") }}
          </button>
        </div>
      </section>

      <div class="flex flex-wrap gap-1 border-b border-line">
        <button
          type="button"
          class="border-b-2 px-4 py-2 text-sm font-medium"
          :class="studentTab === 'practice' ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="studentTab = 'practice'"
        >
          {{ t("practice.title") }}
        </button>
        <button
          type="button"
          class="border-b-2 px-4 py-2 text-sm font-medium"
          :class="studentTab === 'official' ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="studentTab = 'official'; loadActiveAssignments()"
        >
          {{ t("official.title") }}
        </button>
        <button
          type="button"
          class="border-b-2 px-4 py-2 text-sm font-medium"
          :class="studentTab === 'results' ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="studentTab = 'results'; loadPracticeHistory()"
        >
          {{ t("results.title") }}
        </button>
        <button
          type="button"
          class="border-b-2 px-4 py-2 text-sm font-medium"
          :class="studentTab === 'schedule' ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="studentTab = 'schedule'; loadScheduleTab()"
        >
          {{ t("scheduleTab.title") }}
        </button>
      </div>

      <!-- Practice: unlimited retakes -->
      <div v-if="studentTab === 'practice'" class="mt-4">
        <section class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("practice.title") }}</h2>

          <template v-if="practiceStatus === 'idle'">
            <button type="button" class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="startPractice">
              {{ t("quiz.startPractice") }}
            </button>
          </template>
          <template v-else-if="practiceStatus === 'loading' || practiceStatus === 'submitting'">
            <p class="mt-3 text-sm text-ink-muted">…</p>
          </template>
          <template v-else-if="practiceStatus === 'empty'">
            <p class="mt-3 text-sm text-ink-muted">{{ t("quiz.empty") }}</p>
          </template>
          <template v-else-if="practiceStatus === 'locked'">
            <p class="mt-3 text-sm text-ink-muted">{{ practiceMessage || t("quiz.locked") }}</p>
          </template>
          <template v-else-if="practiceStatus === 'active'">
            <p class="mb-1 mt-3 text-xs text-ink-muted">{{ t("quiz.question") }} {{ practiceIndex + 1 }} / {{ practiceQuestions.length }}</p>
            <div class="mb-3 h-2 w-full overflow-hidden rounded-full bg-surface3">
              <div
                class="h-full rounded-full bg-accent transition-all duration-300 ease-out"
                :style="{ width: (practiceIndex / practiceQuestions.length) * 100 + '%' }"
              ></div>
            </div>
            <Transition name="q-fade" mode="out-in">
              <div :key="practiceIndex">
                <p class="mb-3 text-sm font-medium text-ink">{{ practiceQuestions[practiceIndex].question }}</p>
                <div class="space-y-2">
                  <button
                    v-for="(opt, i) in questionOptions(practiceQuestions[practiceIndex])"
                    :key="i"
                    type="button"
                    class="block w-full rounded-lg border border-line bg-surface1 px-3 py-2 text-left text-sm text-ink hover:border-accent"
                    @click="pickPracticeAnswer(i)"
                  >
                    {{ opt }}
                  </button>
                </div>
              </div>
            </Transition>
          </template>
          <template v-else-if="practiceStatus === 'result' && practiceResult">
            <p class="mt-3 text-sm font-semibold text-ink">{{ t("quiz.score") }}: {{ practiceResult.score }} / {{ practiceResult.total }}</p>
            <p class="mt-1 mb-2 text-xs font-medium text-ink-muted">{{ t("quiz.overview") }}</p>
            <div class="mb-3 flex flex-wrap gap-1.5">
              <span
                v-for="item in practiceResult.results"
                :key="item.question_id"
                class="flex h-7 w-7 items-center justify-center rounded-md text-xs font-bold"
                :class="item.is_correct ? 'bg-badge3 text-badge3-fg' : 'bg-badge2 text-badge2-fg'"
                :title="item.question"
              >
                {{ item.is_correct ? "✅" : "❌" }}
              </span>
            </div>
            <div v-if="practiceResult.results.some((r) => !r.is_correct)" class="mt-3 space-y-2">
              <div v-for="item in practiceResult.results.filter((r) => !r.is_correct)" :key="item.question_id" class="rounded-lg border border-danger/30 bg-surface1 p-3 text-xs">
                <p class="font-medium text-ink">{{ item.question }}</p>
                <p class="mt-1 text-danger">{{ t("quiz.yourAnswer") }}: {{ item.options[item.selected] ?? "—" }}</p>
                <p class="text-success">{{ t("quiz.correctAnswer") }}: {{ item.options[item.correct] }}</p>
                <button
                  v-if="!explanations[item.question_id]"
                  type="button"
                  class="mt-2 rounded-md bg-accent px-2.5 py-1 text-xs font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-60"
                  :disabled="explainLoading[item.question_id]"
                  @click="explainMistake(item)"
                >
                  {{ explainLoading[item.question_id] ? t("quiz.explainLoading") : t("quiz.explain") }}
                </button>
                <p v-else class="mt-2 rounded-md bg-surface2 p-2 text-ink">{{ explanations[item.question_id] }}</p>
              </div>
              <label class="block text-xs text-ink-muted">
                {{ t("quiz.explainLangLabel") }}
                <select v-model="explainLang" class="mt-1">
                  <option value="kk">Қазақша</option>
                  <option value="ru">Русский</option>
                  <option value="en">English</option>
                  <option value="zh">中文</option>
                </select>
              </label>
            </div>
            <button type="button" class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="startPractice">
              {{ t("quiz.retake") }}
            </button>
          </template>
        </section>
      </div>

      <!-- Official: one-shot per assignment -->
      <div v-else-if="studentTab === 'official'" class="mt-4">
        <section v-if="!activeAssignmentId" class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("official.title") }}</h2>
          <p class="mt-1 text-sm text-ink-muted">{{ t("official.notice") }}</p>
          <div v-if="activeAssignments.length" class="mt-3 space-y-2">
            <div v-for="a in activeAssignments" :key="a.id" class="flex items-center justify-between rounded-lg border border-line bg-surface1 px-3 py-2">
              <div>
                <p class="text-sm font-semibold text-ink">{{ t(`subject.${a.subject}`) }} — {{ a.level }}</p>
                <p class="text-xs text-ink-muted">{{ t("official.attemptsAllowed") }}: {{ a.attempts_allowed }}</p>
              </div>
              <button type="button" class="rounded-md bg-accent px-2.5 py-1 text-xs font-semibold text-accent-contrast hover:opacity-90" @click="startAssignment(a)">
                {{ t("official.start") }}
              </button>
            </div>
          </div>
          <p v-else class="mt-3 text-sm text-ink-muted">{{ t("official.empty") }}</p>
        </section>

        <section v-else class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("official.title") }}</h2>

          <template v-if="officialStatus === 'loading' || officialStatus === 'submitting'">
            <p class="mt-3 text-sm text-ink-muted">…</p>
          </template>
          <template v-else-if="officialStatus === 'empty'">
            <p class="mt-3 text-sm text-ink-muted">{{ t("quiz.empty") }}</p>
          </template>
          <template v-else-if="officialStatus === 'locked'">
            <p class="mt-3 text-sm text-ink-muted">{{ officialMessage || t("official.alreadyTaken") }}</p>
          </template>
          <template v-else-if="officialStatus === 'active'">
            <p class="mb-1 mt-3 text-xs text-ink-muted">{{ t("quiz.question") }} {{ officialIndex + 1 }} / {{ officialQuestions.length }}</p>
            <div class="mb-3 h-2 w-full overflow-hidden rounded-full bg-surface3">
              <div
                class="h-full rounded-full bg-accent transition-all duration-300 ease-out"
                :style="{ width: (officialIndex / officialQuestions.length) * 100 + '%' }"
              ></div>
            </div>
            <Transition name="q-fade" mode="out-in">
              <div :key="officialIndex">
                <p class="mb-3 text-sm font-medium text-ink">{{ officialQuestions[officialIndex].question }}</p>
                <div class="space-y-2">
                  <button
                    v-for="(opt, i) in questionOptions(officialQuestions[officialIndex])"
                    :key="i"
                    type="button"
                    class="block w-full rounded-lg border border-line bg-surface1 px-3 py-2 text-left text-sm text-ink hover:border-accent"
                    @click="pickOfficialAnswer(i)"
                  >
                    {{ opt }}
                  </button>
                </div>
              </div>
            </Transition>
          </template>
          <template v-else-if="officialStatus === 'result' && officialResult">
            <p class="mt-3 text-sm font-semibold text-ink">{{ t("quiz.score") }}: {{ officialResult.score }} / {{ officialResult.total }}</p>
            <p class="mt-1 mb-2 text-xs font-medium text-ink-muted">{{ t("quiz.overview") }}</p>
            <div class="mb-3 flex flex-wrap gap-1.5">
              <span
                v-for="item in officialResult.results"
                :key="item.question_id"
                class="flex h-7 w-7 items-center justify-center rounded-md text-xs font-bold"
                :class="item.is_correct ? 'bg-badge3 text-badge3-fg' : 'bg-badge2 text-badge2-fg'"
                :title="item.question"
              >
                {{ item.is_correct ? "✅" : "❌" }}
              </span>
            </div>
            <div v-if="officialResult.results.some((r) => !r.is_correct)" class="mt-3 space-y-2">
              <div v-for="item in officialResult.results.filter((r) => !r.is_correct)" :key="item.question_id" class="rounded-lg border border-danger/30 bg-surface1 p-3 text-xs">
                <p class="font-medium text-ink">{{ item.question }}</p>
                <p class="mt-1 text-danger">{{ t("quiz.yourAnswer") }}: {{ item.options[item.selected] ?? "—" }}</p>
                <p class="text-success">{{ t("quiz.correctAnswer") }}: {{ item.options[item.correct] }}</p>
                <button
                  v-if="!explanations[item.question_id]"
                  type="button"
                  class="mt-2 rounded-md bg-accent px-2.5 py-1 text-xs font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-60"
                  :disabled="explainLoading[item.question_id]"
                  @click="explainMistake(item)"
                >
                  {{ explainLoading[item.question_id] ? t("quiz.explainLoading") : t("quiz.explain") }}
                </button>
                <p v-else class="mt-2 rounded-md bg-surface2 p-2 text-ink">{{ explanations[item.question_id] }}</p>
              </div>
              <label class="block text-xs text-ink-muted">
                {{ t("quiz.explainLangLabel") }}
                <select v-model="explainLang" class="mt-1">
                  <option value="kk">Қазақша</option>
                  <option value="ru">Русский</option>
                  <option value="en">English</option>
                  <option value="zh">中文</option>
                </select>
              </label>
            </div>
          </template>

          <button
            v-if="officialStatus === 'result' || officialStatus === 'locked' || officialStatus === 'empty'"
            type="button"
            class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90"
            @click="backToAssignments"
          >
            {{ t("official.backToList") }}
          </button>
        </section>
      </div>

      <!-- Results: practice attempt history -->
      <div v-else-if="studentTab === 'results'" class="mt-4 space-y-4">
        <!-- Academic progress: real average from recent practiceHistory attempts -->
        <section class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("progressLevel.title") }}</h2>
          <template v-if="levelProgressPercent !== null">
            <p class="mt-1 text-xs text-ink-muted">{{ t("progressLevel.recentAverage", { count: recentAttempts.length }) }}</p>
            <div class="mt-2 flex items-center gap-3">
              <div class="h-3 flex-1 overflow-hidden rounded-full bg-surface3">
                <div class="h-full rounded-full bg-accent transition-all duration-500 ease-out" :style="{ width: levelProgressPercent + '%' }"></div>
              </div>
              <span class="font-numeric text-lg font-bold text-ink">{{ levelProgressPercent }}%</span>
            </div>
          </template>
          <p v-else class="mt-2 text-sm text-ink-muted">{{ t("progressLevel.empty") }}</p>
        </section>

        <div class="rounded-xl border border-line bg-surface2 p-5">
          <div v-for="attempt in practiceHistory" :key="attempt.id" class="flex items-center justify-between border-b border-line py-2 text-sm last:border-0">
            <span class="font-medium text-ink">{{ t(`subject.${attempt.subject}`) }} · {{ attempt.level }}</span>
            <span class="text-ink-muted">{{ attempt.score }} / {{ attempt.total }}</span>
            <span class="text-xs text-ink-muted">{{ new Date(attempt.taken_at).toLocaleString() }}</span>
          </div>
          <p v-if="!practiceHistory.length" class="text-sm text-ink-muted">—</p>
        </div>
      </div>

      <!-- Schedule + QR check-in + Speaking Club placeholder -->
      <div v-else class="mt-4 space-y-4">
        <section class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("scheduleTab.scheduleTitle") }}</h2>
          <template v-if="scheduleLoaded">
            <ul v-if="sortedSchedule.length" class="mt-3 grid gap-2 sm:grid-cols-2">
              <li v-for="slot in sortedSchedule" :key="slot.id" class="rounded-lg border border-line bg-surface1 px-3 py-2 text-sm text-ink">
                <span class="font-semibold text-accent">{{ t(`scheduleTab.weekdays.${slot.weekday}`) }}</span>
                {{ shortTime(slot.start_time) }}–{{ shortTime(slot.end_time) }}
              </li>
            </ul>
            <p v-else class="mt-3 text-sm text-ink-muted">{{ t("scheduleTab.scheduleEmpty") }}</p>
          </template>
          <p v-else class="mt-3 text-sm text-ink-muted">…</p>
        </section>

        <section class="rounded-xl border border-line bg-surface2 p-5">
          <h2 class="font-display text-base font-bold text-ink">{{ t("scheduleTab.qrTitle") }}</h2>
          <p class="mt-1 text-xs text-ink-muted">{{ t("scheduleTab.qrHint") }}</p>
          <div class="mt-3 flex justify-center">
            <img
              v-if="qrLoaded && qrCode"
              :src="qrCode.qr_image_base64"
              alt="QR"
              class="h-40 w-40 rounded-lg border border-line bg-surface1 p-2"
            />
            <p v-else-if="qrLoaded" class="text-sm text-ink-muted">—</p>
            <p v-else class="text-sm text-ink-muted">{{ t("scheduleTab.qrLoading") }}</p>
          </div>
        </section>

        <!-- Speaking Club: no backend yet — polished empty state, no fake booking -->
        <section class="rounded-xl border border-line bg-surface2 p-5">
          <div class="flex flex-col items-center gap-3 text-center sm:flex-row sm:text-left">
            <div class="flex h-16 w-16 shrink-0 items-center justify-center rounded-full bg-surface3 text-ink-muted">
              <svg viewBox="0 0 24 24" fill="none" class="h-9 w-9" aria-hidden="true">
                <circle cx="12" cy="8" r="4" stroke="currentColor" stroke-width="1.6" />
                <path d="M4 20c0-4.4 3.6-7 8-7s8 2.6 8 7" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
              </svg>
            </div>
            <div class="flex-1">
              <div class="flex flex-wrap items-center justify-center gap-2 sm:justify-start">
                <h2 class="font-display text-base font-bold text-ink">{{ t("speakingClub.title") }}</h2>
                <span class="rounded-full bg-badge4 px-2 py-0.5 text-xs font-semibold text-badge4-fg">{{ t("speakingClub.comingSoon") }}</span>
              </div>
              <p class="mt-1 text-sm text-ink-muted">{{ t("speakingClub.description") }}</p>
              <button type="button" disabled class="mt-3 cursor-not-allowed rounded-lg border border-line bg-surface3 px-4 py-2 text-sm font-semibold text-ink-muted">
                {{ t("speakingClub.waitlistButton") }}
              </button>
            </div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.q-fade-enter-active,
.q-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}
.q-fade-enter-from {
  opacity: 0;
  transform: translateX(8px);
}
.q-fade-leave-to {
  opacity: 0;
  transform: translateX(-8px);
}
</style>
