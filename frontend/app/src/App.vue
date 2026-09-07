<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";

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

function authHeaders() {
  return { Authorization: `Bearer ${token.value}` };
}

// --- Parent view: progress / balance / report / certificates ---
const progress = ref<{ full_name: string; level: string; streak_days: number } | null>(null);
const balance = ref<{ balance: number; payment_status: string } | null>(null);
const reportStatus = ref<"idle" | "sent">("idle");
type Certificate = { id: string; level: string; issued_at: string };
const certificates = ref<Certificate[]>([]);

async function loadParentData() {
  if (!token.value) return;
  const [p, b, certs] = await Promise.all([
    fetch("/api/progress", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/balance", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/certificates", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
  ]);
  progress.value = p;
  balance.value = b;
  certificates.value = certs?.certificates ?? [];
}

const downloadError = ref("");

function downloadReport() {
  if (!token.value) return;
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
  if (!token.value) return;
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
  if (!token.value) return;
  const res = await fetch("/api/report/telegram", { method: "POST", headers: authHeaders() });
  if (res.ok) reportStatus.value = "sent";
}

// --- Student view: real question bank (practice = unlimited retakes,
// official = one-time level test), enforced server-side. ---
type StudentProfile = { full_name: string; level: string; language: "en" | "zh" | null; streak_days: number };
const studentProfile = ref<StudentProfile | null>(null);

async function loadStudentProfile() {
  if (!token.value) return;
  const res = await fetch("/api/progress", { headers: authHeaders() });
  if (res.ok) studentProfile.value = await res.json();
}

async function chooseLanguage(lang: "en" | "zh") {
  const res = await fetch("/api/profile/language", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ language: lang }),
  });
  if (res.ok) await loadStudentProfile();
}

function defaultLevel(lang: "en" | "zh" | null | undefined): string {
  return lang === "zh" ? "HSK1" : "Beginner";
}

type QuizKind = "practice" | "official";
type Question = { id: string; question: string; option_a: string; option_b: string; option_c?: string; option_d?: string };
type QuizResultItem = { question_id: string; question: string; options: string[]; selected: number; correct: number; is_correct: boolean };
type QuizStatus = "idle" | "loading" | "active" | "submitting" | "result" | "locked" | "empty";

const quizKind = ref<QuizKind | null>(null);
const quizStatus = ref<QuizStatus>("idle");
const quizQuestions = ref<Question[]>([]);
const quizIndex = ref(0);
const quizAnswers = ref<Record<string, number>>({});
const quizResult = ref<{ score: number; total: number; results: QuizResultItem[] } | null>(null);
const quizLockedMessage = ref("");

function questionOptions(q: Question): string[] {
  return [q.option_a, q.option_b, q.option_c, q.option_d].filter((o): o is string => !!o);
}

async function startQuiz(kind: QuizKind) {
  if (!studentProfile.value?.language) return;
  quizKind.value = kind;
  quizStatus.value = "loading";
  quizResult.value = null;
  quizAnswers.value = {};
  quizIndex.value = 0;

  const language = studentProfile.value.language;
  const level = studentProfile.value.level || defaultLevel(language);
  const res = await fetch(`/api/questions?language=${language}&level=${encodeURIComponent(level)}&kind=${kind}`, {
    headers: authHeaders(),
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    quizLockedMessage.value = body.message ?? "";
    quizStatus.value = "locked";
    return;
  }
  if (!res.ok) {
    quizStatus.value = "idle";
    return;
  }
  const body = await res.json();
  quizQuestions.value = body.questions ?? [];
  quizStatus.value = quizQuestions.value.length ? "active" : "empty";
}

async function pickAnswer(optionIndex: number) {
  const q = quizQuestions.value[quizIndex.value];
  quizAnswers.value[q.id] = optionIndex;
  if (quizIndex.value + 1 < quizQuestions.value.length) {
    quizIndex.value++;
  } else {
    await submitQuiz();
  }
}

async function submitQuiz() {
  if (!studentProfile.value?.language || !quizKind.value) return;
  quizStatus.value = "submitting";
  const answers = Object.entries(quizAnswers.value).map(([question_id, selected]) => ({ question_id, selected }));
  const res = await fetch("/api/quiz/submit", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ language: studentProfile.value.language, kind: quizKind.value, answers }),
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    quizLockedMessage.value = body.message ?? "";
    quizStatus.value = "locked";
    return;
  }
  if (!res.ok) {
    quizStatus.value = "active";
    return;
  }
  quizResult.value = await res.json();
  quizStatus.value = "result";
  await loadResults();
}

// --- AI: "explain my mistake", in whichever language the student picks ---
const explainLang = ref<"kk" | "ru" | "en" | "zh">("kk");
const explanations = ref<Record<string, string>>({});
const explainLoading = ref<Record<string, boolean>>({});

async function explainMistake(item: QuizResultItem) {
  if (explanations.value[item.question_id] || !studentProfile.value?.language) return;
  explainLoading.value[item.question_id] = true;
  const res = await fetch("/api/quiz/explain", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      question: item.question,
      chosen_answer: item.options[item.selected] ?? "",
      correct_answer: item.options[item.correct],
      language: studentProfile.value.language,
      explain_language: explainLang.value,
    }),
  });
  explainLoading.value[item.question_id] = false;
  if (res.ok) {
    const body = await res.json();
    explanations.value[item.question_id] = body.explanation;
  }
}

type StudentTab = "practice" | "results";
const studentTab = ref<StudentTab>("practice");
type PracticeAttempt = { id: string; language: string; score: number; total: number; taken_at: string };
const resultsHistory = ref<PracticeAttempt[]>([]);

async function loadResults() {
  if (!token.value) return;
  const res = await fetch("/api/practice/attempts", { headers: authHeaders() });
  if (res.ok) resultsHistory.value = (await res.json()).attempts ?? [];
}

async function loadForRole() {
  if (!token.value) return;
  if (loggedInRole.value === "parent") await loadParentData();
  if (loggedInRole.value === "student") await Promise.all([loadStudentProfile(), loadResults()]);
}

// --- Login (also usable if someone lands here directly) ---
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
    await loadForRole();
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

onMounted(async () => {
  consumeHandoff();
  await loadForRole();
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

    <!-- Parent: monitoring dashboard -->
    <div v-else-if="loggedInRole === 'parent'" class="grid gap-4 sm:grid-cols-2">
      <section v-if="progress" class="rounded-xl border border-line bg-surface2 p-5">
        <h2 class="font-display text-base font-bold text-ink">{{ t("progress.title") }}</h2>
        <p class="mt-2 text-sm font-semibold text-ink">{{ progress.full_name }}</p>
        <p class="text-sm text-ink-muted">{{ t("progress.level") }}: {{ progress.level }}</p>
        <p class="text-sm text-ink-muted">🔥 {{ t("progress.streak") }}: {{ progress.streak_days }}</p>
      </section>

      <section v-if="balance" class="rounded-xl border border-line bg-surface2 p-5">
        <h2 class="font-display text-base font-bold text-ink">{{ t("balance.title") }}</h2>
        <p class="mt-2 text-lg font-semibold text-ink">{{ balance.balance }} ₸</p>
        <p class="text-sm text-ink-muted">{{ t("balance.status") }}: {{ t(`balance.${balance.payment_status}`) }}</p>
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

    <!-- Student: language track, real practice/official tests, results history -->
    <div v-else>
      <!-- One-time language track picker — shown until the student (or,
           from Phase 4 on, the director) has set it. -->
      <section v-if="studentProfile && !studentProfile.language" class="mx-auto max-w-sm rounded-xl border border-line bg-surface2 p-5 text-center">
        <h2 class="font-display text-base font-bold text-ink">{{ t("language.choose") }}</h2>
        <p class="mt-1 text-xs text-ink-muted">{{ t("language.chooseHint") }}</p>
        <div class="mt-4 grid grid-cols-2 gap-3">
          <button type="button" class="rounded-lg border border-line px-4 py-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent" @click="chooseLanguage('en')">
            {{ t("language.english") }}
          </button>
          <button type="button" class="rounded-lg border border-line px-4 py-3 text-sm font-semibold text-ink hover:border-accent hover:text-accent" @click="chooseLanguage('zh')">
            {{ t("language.chinese") }}
          </button>
        </div>
      </section>

      <template v-else>
        <div class="flex gap-1 border-b border-line">
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
            :class="studentTab === 'results' ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
            @click="studentTab = 'results'; loadResults()"
          >
            {{ t("results.title") }}
          </button>
        </div>

        <div v-if="studentTab === 'practice'" class="mt-4 grid gap-4 sm:grid-cols-2">
          <!-- Practice: unlimited retakes -->
          <section class="rounded-xl border border-line bg-surface2 p-5">
            <h2 class="font-display text-base font-bold text-ink">{{ t("practice.title") }}</h2>

            <template v-if="quizKind !== 'practice' || quizStatus === 'idle'">
              <button type="button" class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="startQuiz('practice')">
                {{ t("quiz.startPractice") }}
              </button>
            </template>
            <template v-else-if="quizKind === 'practice' && (quizStatus === 'loading' || quizStatus === 'submitting')">
              <p class="mt-3 text-sm text-ink-muted">…</p>
            </template>
            <template v-else-if="quizKind === 'practice' && quizStatus === 'empty'">
              <p class="mt-3 text-sm text-ink-muted">{{ t("quiz.empty") }}</p>
            </template>
            <template v-else-if="quizKind === 'practice' && quizStatus === 'active'">
              <p class="mb-1 mt-3 text-xs text-ink-muted">{{ t("quiz.question") }} {{ quizIndex + 1 }} / {{ quizQuestions.length }}</p>
              <p class="mb-3 text-sm font-medium text-ink">{{ quizQuestions[quizIndex].question }}</p>
              <div class="space-y-2">
                <button
                  v-for="(opt, i) in questionOptions(quizQuestions[quizIndex])"
                  :key="i"
                  type="button"
                  class="block w-full rounded-lg border border-line bg-surface1 px-3 py-2 text-left text-sm text-ink hover:border-accent"
                  @click="pickAnswer(i)"
                >
                  {{ opt }}
                </button>
              </div>
            </template>
            <template v-else-if="quizKind === 'practice' && quizStatus === 'result' && quizResult">
              <p class="mt-3 text-sm font-semibold text-ink">{{ t("quiz.score") }}: {{ quizResult.score }} / {{ quizResult.total }}</p>
              <div v-if="quizResult.results.some((r) => !r.is_correct)" class="mt-3 space-y-2">
                <div v-for="item in quizResult.results.filter((r) => !r.is_correct)" :key="item.question_id" class="rounded-lg border border-danger/30 bg-surface1 p-3 text-xs">
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
              <button type="button" class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="startQuiz('practice')">
                {{ t("quiz.retake") }}
              </button>
            </template>
            <template v-else-if="quizKind === 'practice' && quizStatus === 'locked'">
              <p class="mt-3 text-sm text-ink-muted">{{ quizLockedMessage }}</p>
            </template>
          </section>

          <!-- Official: one-time level-check test -->
          <section class="rounded-xl border border-line bg-surface2 p-5">
            <h2 class="font-display text-base font-bold text-ink">{{ t("official.title") }}</h2>

            <template v-if="quizKind !== 'official' || quizStatus === 'idle'">
              <p class="mt-1 text-sm text-ink-muted">{{ t("official.notice") }}</p>
              <button type="button" class="mt-3 w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast hover:opacity-90" @click="startQuiz('official')">
                {{ t("quiz.startOfficial") }}
              </button>
            </template>
            <template v-else-if="quizKind === 'official' && (quizStatus === 'loading' || quizStatus === 'submitting')">
              <p class="mt-3 text-sm text-ink-muted">…</p>
            </template>
            <template v-else-if="quizKind === 'official' && quizStatus === 'locked'">
              <p class="mt-3 text-sm text-ink-muted">{{ quizLockedMessage || t('quiz.locked') }}</p>
            </template>
            <template v-else-if="quizKind === 'official' && quizStatus === 'empty'">
              <p class="mt-3 text-sm text-ink-muted">{{ t("quiz.empty") }}</p>
            </template>
            <template v-else-if="quizKind === 'official' && quizStatus === 'active'">
              <p class="mb-1 mt-3 text-xs text-ink-muted">{{ t("quiz.question") }} {{ quizIndex + 1 }} / {{ quizQuestions.length }}</p>
              <p class="mb-3 text-sm font-medium text-ink">{{ quizQuestions[quizIndex].question }}</p>
              <div class="space-y-2">
                <button
                  v-for="(opt, i) in questionOptions(quizQuestions[quizIndex])"
                  :key="i"
                  type="button"
                  class="block w-full rounded-lg border border-line bg-surface1 px-3 py-2 text-left text-sm text-ink hover:border-accent"
                  @click="pickAnswer(i)"
                >
                  {{ opt }}
                </button>
              </div>
            </template>
            <template v-else-if="quizKind === 'official' && quizStatus === 'result' && quizResult">
              <p class="mt-3 text-sm font-semibold text-ink">{{ t("quiz.score") }}: {{ quizResult.score }} / {{ quizResult.total }}</p>
              <p class="mt-1 text-xs text-ink-muted">{{ t("quiz.locked") }}</p>
            </template>
          </section>
        </div>

        <div v-else class="mt-4 rounded-xl border border-line bg-surface2 p-5">
          <div v-for="attempt in resultsHistory" :key="attempt.id" class="flex items-center justify-between border-b border-line py-2 text-sm last:border-0">
            <span class="font-medium text-ink">{{ attempt.language === "en" ? "EN" : "中文" }}</span>
            <span class="text-ink-muted">{{ attempt.score }} / {{ attempt.total }}</span>
            <span class="text-xs text-ink-muted">{{ new Date(attempt.taken_at).toLocaleString() }}</span>
          </div>
          <p v-if="!resultsHistory.length" class="text-sm text-ink-muted">—</p>
        </div>
      </template>
    </div>

  </main>
</template>
