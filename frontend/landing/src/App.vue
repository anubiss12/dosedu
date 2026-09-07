<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";

const { t, tm } = useI18n();

// ============================================================
// Lead form (public "get in touch" from the landing page)
// ============================================================
const fullName = ref("");
const phone = ref("");
const levelTest = ref("");
const submitting = ref(false);
const leadStatus = ref<"idle" | "success" | "error">("idle");

async function submitLead(extra?: { levelTestResult?: string }) {
  submitting.value = true;
  leadStatus.value = "idle";
  try {
    const res = await fetch("/api/public/leads", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        full_name: fullName.value,
        phone: phone.value,
        level_test_result: extra?.levelTestResult ?? levelTest.value,
      }),
    });
    if (!res.ok) throw new Error("request failed");
    leadStatus.value = "success";
    fullName.value = "";
    phone.value = "";
    levelTest.value = "";
  } catch {
    leadStatus.value = "error";
  } finally {
    submitting.value = false;
  }
}

// ============================================================
// Quiz bank — shared between the "Тіл курстары" trial test and the
// logged-in student's unlimited practice mode.
// ============================================================
type QuizQuestion = { q: string; options: string[]; correct: number };
const quizBank: Record<"en" | "zh", QuizQuestion[]> = {
  en: [
    { q: "She ___ to school every day.", options: ["go", "goes", "going", "gone"], correct: 1 },
    { q: "What is the plural of 'child'?", options: ["childs", "children", "childes", "child"], correct: 1 },
    { q: "Choose the opposite of 'happy'.", options: ["sad", "glad", "fast", "big"], correct: 0 },
  ],
  zh: [
    { q: "\"你好\" (nǐ hǎo) мағынасы?", options: ["Сау бол", "Сәлем", "Рахмет", "Кешіріңіз"], correct: 1 },
    { q: "\"谢谢\" (xièxiè) мағынасы?", options: ["Иә", "Жоқ", "Рахмет", "Сәлем"], correct: 2 },
    { q: "\"再见\" (zàijiàn) мағынасы?", options: ["Сау бол", "Рахмет", "Сәлем", "Кешіріңіз"], correct: 0 },
  ],
};

// ============================================================
// Trial test modal — "Тіл курстары" in the nav opens this: pick a
// language, answer a short quiz, then leave contact details.
// ============================================================
const showTrialModal = ref(false);
const trialLang = ref<"en" | "zh" | null>(null);
const trialStep = ref<"pick" | "quiz" | "contact" | "done">("pick");
const trialQIndex = ref(0);
const trialCorrectCount = ref(0);

function openTrial() {
  showTrialModal.value = true;
  trialLang.value = null;
  trialStep.value = "pick";
  trialQIndex.value = 0;
  trialCorrectCount.value = 0;
}

function pickTrialLang(lang: "en" | "zh") {
  trialLang.value = lang;
  trialStep.value = "quiz";
  trialQIndex.value = 0;
  trialCorrectCount.value = 0;
}

function answerTrial(optionIndex: number) {
  if (!trialLang.value) return;
  const questions = quizBank[trialLang.value];
  if (optionIndex === questions[trialQIndex.value].correct) {
    trialCorrectCount.value++;
  }
  if (trialQIndex.value + 1 < questions.length) {
    trialQIndex.value++;
  } else {
    trialStep.value = "contact";
  }
}

async function submitTrialContact() {
  const label = trialLang.value === "en" ? "English" : "Chinese";
  await submitLead({ levelTestResult: `${label}: ${trialCorrectCount.value}/${quizBank[trialLang.value!].length}` });
  trialStep.value = "done";
}

// ============================================================
// Prodlenka info modal — "Продленка" in the nav opens this.
// ============================================================
const showProdlenkaModal = ref(false);

// ============================================================
// Login modal — authenticates only. On success, hands the token off
// to the app.<domain> subdomain (a full page) instead of rendering
// the dashboard inline here, since the practice-test/monitoring
// experience deserves more room than a small popup.
// ============================================================
const showLoginModal = ref(false);
const identifier = ref("");
const password = ref("");
const loginError = ref(false);
const showForgotPassword = ref(false);

function detectRole(value: string): "student" | "parent" {
  const looksLikePhone = /^\+?\d[\d\s()-]{6,}$/.test(value.trim());
  return looksLikePhone ? "parent" : "student";
}

function appPortalUrl(hash?: string): string {
  const { protocol, hostname, port } = window.location;
  const portSuffix = port ? `:${port}` : "";
  return `${protocol}//app.${hostname}${portSuffix}/${hash ? `#${hash}` : ""}`;
}

async function login() {
  loginError.value = false;
  const role = detectRole(identifier.value);
  try {
    const res = await fetch("/api/public/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifier: identifier.value, password: password.value, role }),
    });
    if (!res.ok) throw new Error("invalid");
    const data = await res.json();
    window.location.href = appPortalUrl(`token=${encodeURIComponent(data.token)}&role=${role}`);
  } catch {
    loginError.value = true;
  }
}

function openLogin() {
  // Already signed in on this device? Skip the form and go straight
  // to the full-page portal.
  if (localStorage.getItem("dosedu_token")) {
    window.location.href = appPortalUrl();
    return;
  }
  showLoginModal.value = true;
}
</script>

<template>
  <header class="sticky top-0 z-30 border-b border-line bg-surface1/90 backdrop-blur">
    <div class="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 sm:px-6 lg:px-8">
      <div class="flex items-center gap-3">
        <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
        <span class="font-display text-lg font-bold text-ink">{{ t("app.title") }}</span>
      </div>

      <nav class="hidden items-center gap-8 md:flex">
        <button type="button" class="text-sm font-medium text-ink-muted hover:text-ink" @click="openTrial">
          {{ t("nav.courses") }}
        </button>
        <button type="button" class="text-sm font-medium text-ink-muted hover:text-ink" @click="showProdlenkaModal = true">
          {{ t("nav.prodlenka") }}
        </button>
      </nav>

      <div class="flex items-center gap-3">
        <button
          type="button"
          class="font-accent rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast shadow-sm shadow-accent/30 hover:opacity-90"
          @click="openLogin"
        >
          {{ t("nav.login") }}
        </button>
        <LangSwitcher />
        <ThemeToggle />
      </div>
    </div>
  </header>

  <section class="relative scroll-mt-20 overflow-hidden bg-surface2 px-4 py-14 sm:px-6 sm:py-20 lg:px-8 xl:py-24">
    <div
      class="pointer-events-none absolute -right-24 -top-32 h-96 w-96 rounded-full bg-accent/20 blur-3xl"
      aria-hidden="true"
    ></div>
    <div
      class="pointer-events-none absolute -left-16 bottom-0 h-72 w-72 rounded-full bg-badge3/40 blur-3xl"
      aria-hidden="true"
    ></div>

    <div class="relative mx-auto grid max-w-7xl items-center gap-12 md:grid-cols-2">
      <div>
        <span class="font-accent inline-block rounded-full bg-badge1 px-3 py-1 text-xs font-semibold text-badge1-fg">
          {{ t("app.title") }}
        </span>
        <h1 class="mt-4 font-display text-4xl font-extrabold leading-tight text-ink sm:text-5xl xl:text-6xl">
          {{ t("hero.title") }}
        </h1>
        <p class="mt-5 max-w-prose text-lg text-ink-muted xl:text-xl">{{ t("hero.subtitle") }}</p>
        <div class="mt-8 flex flex-col gap-3 sm:flex-row">
          <a
            href="#contact"
            class="font-accent rounded-lg bg-accent px-6 py-3 text-center text-base font-semibold text-accent-contrast shadow-lg shadow-accent/30 hover:opacity-90"
          >
            {{ t("hero.ctaPrimary") }}
          </a>
          <a
            href="#how-it-works"
            class="font-accent rounded-lg border border-line bg-surface1/60 px-6 py-3 text-center text-base font-semibold text-ink backdrop-blur hover:bg-surface1"
          >
            {{ t("hero.ctaSecondary") }}
          </a>
        </div>
      </div>

      <div class="md:rotate-1">
        <div class="rounded-2xl border border-line bg-surface1 p-5 shadow-2xl shadow-accent/10 sm:p-6">
          <div class="mb-4 flex items-center gap-1.5">
            <span class="h-2.5 w-2.5 rounded-full bg-danger/60"></span>
            <span class="h-2.5 w-2.5 rounded-full bg-badge4-fg/70"></span>
            <span class="h-2.5 w-2.5 rounded-full bg-success/60"></span>
          </div>

          <p class="text-xs font-semibold text-ink-muted">{{ t("mockup.journalTitle") }}</p>
          <div class="mt-2 space-y-2">
            <div class="flex items-center justify-between rounded-lg bg-surface2 px-3 py-2 text-sm">
              <span class="text-ink">Айгерім Т.</span>
              <span class="font-numeric rounded-full bg-badge3 px-2 py-0.5 text-xs font-semibold text-badge3-fg">92</span>
            </div>
            <div class="flex items-center justify-between rounded-lg bg-surface2 px-3 py-2 text-sm">
              <span class="text-ink">Дамир С.</span>
              <span class="font-numeric rounded-full bg-badge3 px-2 py-0.5 text-xs font-semibold text-badge3-fg">88</span>
            </div>
          </div>

          <p class="mt-5 text-xs font-semibold text-ink-muted">{{ t("mockup.scheduleTitle") }}</p>
          <div class="mt-2 space-y-2">
            <div class="flex items-center justify-between rounded-lg bg-surface2 px-3 py-2 text-sm text-ink">
              <span>Дс, Ср, Жм</span>
              <span class="text-ink-muted">16:00–17:30</span>
            </div>
          </div>

          <p class="mt-5 text-xs font-semibold text-ink-muted">{{ t("mockup.balanceTitle") }}</p>
          <div class="mt-2 flex items-center justify-between rounded-lg bg-accent px-3 py-3">
            <span class="font-numeric text-2xl font-bold text-accent-contrast">15 000 ₸</span>
            <span class="font-accent rounded-full bg-white/20 px-2.5 py-1 text-xs font-semibold text-accent-contrast">
              {{ t("balance.paid") }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </section>

  <!-- Programs -->
  <section id="programs" class="scroll-mt-20 bg-surface1 px-4 py-16 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-6xl">
      <h2 class="text-center font-display text-3xl font-bold text-ink">{{ t("programs.title") }}</h2>

      <div class="mt-10 grid gap-6 sm:grid-cols-2">
        <div
          v-for="key in (['prodlenka', 'mad', 'english', 'chinese'] as const)"
          :key="key"
          class="rounded-2xl border border-line bg-surface2 p-6 shadow-sm transition hover:-translate-y-0.5 hover:shadow-lg hover:shadow-accent/10"
        >
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="font-display text-lg font-bold text-ink">{{ t(`programs.${key}.title`) }}</h3>
            <span class="font-accent rounded-full bg-badge1 px-2.5 py-1 text-xs font-semibold text-badge1-fg">
              {{ t(`programs.${key}.badge`) }}
            </span>
          </div>
          <ul class="mt-4 space-y-2">
            <li
              v-for="(bullet, i) in (tm(`programs.${key}.bullets`) as unknown as string[])"
              :key="i"
              class="flex items-start gap-2 text-sm text-ink-muted"
            >
              <span class="mt-0.5 text-teal">✓</span>{{ bullet }}
            </li>
          </ul>
        </div>
      </div>
    </div>
  </section>

  <section id="how-it-works" class="scroll-mt-20 bg-surface1 px-4 py-16 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-7xl">
      <h2 class="text-center font-display text-3xl font-bold text-ink">{{ t("howItWorks.title") }}</h2>

      <div class="relative mt-14 grid gap-10 md:grid-cols-3">
        <div class="absolute left-[16.6%] right-[16.6%] top-6 hidden h-px bg-line md:block"></div>

        <div v-for="n in 3" :key="n" class="relative flex flex-col items-center text-center">
          <span
            class="font-numeric relative z-10 flex h-12 w-12 items-center justify-center rounded-full bg-accent text-lg font-bold text-accent-contrast shadow-md shadow-accent/30"
          >
            {{ n }}
          </span>
          <h3 class="mt-4 font-display text-lg font-semibold text-ink">{{ t(`howItWorks.step${n}Title`) }}</h3>
          <p class="mt-2 max-w-xs text-sm text-ink-muted">{{ t(`howItWorks.step${n}Desc`) }}</p>
        </div>
      </div>
    </div>
  </section>

  <section id="why-us" class="scroll-mt-20 bg-surface2 px-4 py-16 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-5xl">
      <h2 class="text-center font-display text-3xl font-bold text-ink">{{ t("whyUs.title") }}</h2>

      <div class="mt-12 grid gap-x-10 gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="n in 6" :key="n" class="flex items-start gap-4">
          <span
            class="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl text-xl"
            :class="['bg-badge1 text-badge1-fg', 'bg-badge2 text-badge2-fg', 'bg-badge3 text-badge3-fg', 'bg-badge4 text-badge4-fg', 'bg-badge1 text-badge1-fg', 'bg-badge3 text-badge3-fg'][n - 1]"
          >
            {{ ["👩‍🏫", "📝", "📊", "🛡️", "🔔", "🎨"][n - 1] }}
          </span>
          <div>
            <h3 class="font-display text-base font-semibold text-ink">{{ t(`whyUs.item${n}Title`) }}</h3>
            <p class="mt-1 text-sm text-ink-muted">{{ t(`whyUs.item${n}Desc`) }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section class="bg-accent px-4 py-14 sm:px-6 lg:px-8">
    <div class="mx-auto grid max-w-5xl grid-cols-1 gap-8 text-center sm:grid-cols-3">
      <div v-for="n in 3" :key="n">
        <div class="font-numeric text-4xl font-extrabold text-accent-contrast sm:text-5xl">
          {{ t(`stats.stat${n}Number`) }}
        </div>
        <div class="mt-2 text-sm text-accent-contrast/80">{{ t(`stats.stat${n}Label`) }}</div>
      </div>
    </div>
  </section>

  <!-- Banner -->
  <section class="bg-surface2 px-4 py-10 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-4xl text-center">
      <h2 class="font-display text-2xl font-bold text-ink sm:text-3xl">{{ t("banner.title") }}</h2>
      <div class="mt-6 flex flex-wrap justify-center gap-6 text-sm text-ink-muted">
        <span class="flex items-center gap-2">🎲 {{ t("banner.playful") }}</span>
        <span class="flex items-center gap-2">✨ {{ t("banner.effective") }}</span>
        <span class="flex items-center gap-2">🎯 {{ t("banner.resultOriented") }}</span>
      </div>
    </div>
  </section>

  <section id="contact" class="scroll-mt-20 bg-surface1 px-4 py-16 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-md">
      <h2 class="text-center font-display text-3xl font-bold text-ink">{{ t("leadForm.title") }}</h2>
      <form class="mt-8 space-y-4" @submit.prevent="() => submitLead()">
        <label class="block text-sm text-ink-muted">
          {{ t("leadForm.fullName") }}
          <input v-model="fullName" required class="mt-1" />
        </label>
        <label class="block text-sm text-ink-muted">
          {{ t("leadForm.phone") }}
          <input v-model="phone" type="tel" required class="mt-1" />
        </label>
        <label class="block text-sm text-ink-muted">
          {{ t("leadForm.levelTest") }}
          <input v-model="levelTest" class="mt-1" />
        </label>
        <button
          type="submit"
          :disabled="submitting"
          class="font-accent w-full rounded-lg bg-accent px-6 py-3 text-base font-semibold text-accent-contrast shadow-md shadow-accent/30 hover:opacity-90 disabled:opacity-60"
        >
          {{ t("leadForm.submit") }}
        </button>
        <p v-if="leadStatus === 'success'" class="text-center text-sm text-success">{{ t("leadForm.success") }}</p>
        <p v-if="leadStatus === 'error'" class="text-center text-sm text-danger">{{ t("leadForm.error") }}</p>
      </form>
    </div>
  </section>

  <footer class="border-t border-line bg-surface2 px-4 py-12 sm:px-6 lg:px-8">
    <div class="mx-auto grid max-w-7xl gap-10 sm:grid-cols-2 lg:grid-cols-4">
      <div>
        <div class="flex items-center gap-2">
          <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
          <span class="font-display text-base font-bold text-ink">{{ t("app.title") }}</span>
        </div>
        <p class="mt-3 max-w-xs text-sm text-ink-muted">{{ t("footer.about") }}</p>
      </div>

      <div>
        <h4 class="font-display text-sm font-semibold text-ink">{{ t("footer.phoneLabel") }}</h4>
        <p class="mt-3 text-sm text-ink-muted">8700 100 1046</p>
        <p class="text-sm text-ink-muted">8702 968 7900</p>
        <h4 class="mt-5 font-display text-sm font-semibold text-ink">{{ t("footer.addressLabel") }}</h4>
        <p class="mt-3 text-sm text-ink-muted">Тұрғыт Озал 71</p>
      </div>

      <div>
        <h4 class="font-display text-sm font-semibold text-ink">{{ t("nav.courses") }}</h4>
        <ul class="mt-3 space-y-2 text-sm text-ink-muted">
          <li><a href="#how-it-works" class="hover:text-ink">{{ t("howItWorks.title") }}</a></li>
          <li><a href="#why-us" class="hover:text-ink">{{ t("whyUs.title") }}</a></li>
          <li><a href="#contact" class="hover:text-ink">{{ t("leadForm.title") }}</a></li>
        </ul>
      </div>

      <div>
        <h4 class="font-display text-sm font-semibold text-ink">DOS EDUCATION</h4>
        <ul class="mt-3 space-y-2 text-sm text-ink-muted">
          <li><a href="/privacy" class="hover:text-ink">{{ t("footer.privacyPolicy") }}</a></li>
          <li><a href="/offer" class="hover:text-ink">{{ t("footer.offer") }}</a></li>
        </ul>
      </div>
    </div>

    <div class="mx-auto mt-10 max-w-7xl border-t border-line pt-6 text-center text-sm text-ink-muted">
      © {{ new Date().getFullYear() }} DOS EDUCATION — {{ t("footer.rights") }}
    </div>
  </footer>

  <Teleport to="body">
    <div
      v-if="showTrialModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      @click.self="showTrialModal = false"
    >
      <div class="w-full max-w-sm rounded-2xl border border-line bg-surface1 p-6 shadow-xl">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="font-display text-lg font-bold text-ink">{{ t("trial.title") }}</h2>
          <button type="button" class="text-ink-muted hover:text-ink" @click="showTrialModal = false">✕</button>
        </div>

        <template v-if="trialStep === 'pick'">
          <p class="mb-4 text-sm text-ink-muted">{{ t("trial.pickLanguage") }}</p>
          <div class="grid grid-cols-2 gap-3">
            <button
              type="button"
              class="rounded-lg border border-line px-4 py-3 text-sm font-semibold text-ink transition hover:border-accent hover:text-accent"
              @click="pickTrialLang('en')"
            >
              {{ t("trial.english") }}
            </button>
            <button
              type="button"
              class="rounded-lg border border-line px-4 py-3 text-sm font-semibold text-ink transition hover:border-accent hover:text-accent"
              @click="pickTrialLang('zh')"
            >
              {{ t("trial.chinese") }}
            </button>
          </div>
        </template>

        <template v-else-if="trialStep === 'quiz' && trialLang">
          <p class="mb-1 text-xs text-ink-muted">
            {{ t("trial.question") }} {{ trialQIndex + 1 }} / {{ quizBank[trialLang].length }}
          </p>
          <p class="mb-4 text-sm font-medium text-ink">{{ quizBank[trialLang][trialQIndex].q }}</p>
          <div class="space-y-2">
            <button
              v-for="(opt, i) in quizBank[trialLang][trialQIndex].options"
              :key="i"
              type="button"
              class="block w-full rounded-lg border border-line px-4 py-2 text-left text-sm text-ink transition hover:border-accent hover:text-accent"
              @click="answerTrial(i)"
            >
              {{ opt }}
            </button>
          </div>
        </template>

        <template v-else-if="trialStep === 'contact'">
          <p class="mb-4 text-sm text-ink-muted">{{ t("trial.contactPrompt") }}</p>
          <form class="space-y-3" @submit.prevent="submitTrialContact">
            <label class="block text-sm text-ink-muted">
              {{ t("leadForm.fullName") }}
              <input v-model="fullName" required class="mt-1" />
            </label>
            <label class="block text-sm text-ink-muted">
              {{ t("leadForm.phone") }}
              <input v-model="phone" type="tel" required class="mt-1" />
            </label>
            <button type="submit" class="font-accent w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90">
              {{ t("trial.submit") }}
            </button>
          </form>
        </template>

        <template v-else-if="trialStep === 'done'">
          <p class="text-sm text-success">{{ t("trial.success") }}</p>
          <button
            type="button"
            class="mt-4 w-full rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink"
            @click="showTrialModal = false"
          >
            {{ t("login.logout") }}
          </button>
        </template>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="showProdlenkaModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      @click.self="showProdlenkaModal = false"
    >
      <div class="w-full max-w-md rounded-2xl border border-line bg-surface1 p-6 shadow-xl">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="font-display text-lg font-bold text-ink">{{ t("prodlenka.title") }}</h2>
          <button type="button" class="text-ink-muted hover:text-ink" @click="showProdlenkaModal = false">✕</button>
        </div>
        <p class="text-sm text-ink-muted">{{ t("prodlenka.intro") }}</p>
        <ul class="mt-4 space-y-2 text-sm text-ink">
          <li class="flex gap-2"><span>✅</span>{{ t("prodlenka.point1") }}</li>
          <li class="flex gap-2"><span>✅</span>{{ t("prodlenka.point2") }}</li>
          <li class="flex gap-2"><span>✅</span>{{ t("prodlenka.point3") }}</li>
        </ul>
        <a
          href="#contact"
          class="font-accent mt-5 block rounded-lg bg-accent px-4 py-2.5 text-center text-sm font-semibold text-accent-contrast hover:opacity-90"
          @click="showProdlenkaModal = false"
        >
          {{ t("prodlenka.cta") }}
        </a>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div
      v-if="showLoginModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      @click.self="showLoginModal = false"
    >
      <div class="w-full max-w-sm rounded-2xl border border-line bg-surface1 p-6 shadow-xl">
        <div class="mb-4 flex items-center justify-between">
          <h2 class="font-display text-lg font-bold text-ink">{{ t("login.title") }}</h2>
          <button type="button" class="text-ink-muted hover:text-ink" @click="showLoginModal = false">✕</button>
        </div>

        <form class="space-y-4" @submit.prevent="login">
          <label class="block text-sm text-ink-muted">
            {{ t("login.identifier") }}
            <input v-model="identifier" required class="mt-1" />
          </label>
          <label class="block text-sm text-ink-muted">
            {{ t("login.password") }}
            <input v-model="password" type="password" inputmode="numeric" required minlength="4" maxlength="4" class="mt-1" />
          </label>
          <button type="submit" class="font-accent w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90">
            {{ t("login.submit") }}
          </button>
          <p v-if="loginError" class="text-center text-sm text-danger">{{ t("login.error") }}</p>
          <button
            type="button"
            class="block w-full text-center text-xs text-ink-muted underline"
            @click="showForgotPassword = !showForgotPassword"
          >
            {{ t("login.forgotPassword") }}
          </button>
          <p v-if="showForgotPassword" class="text-center text-xs text-ink-muted">{{ t("login.forgotPasswordHint") }}</p>
        </form>
      </div>
    </div>
  </Teleport>
</template>
