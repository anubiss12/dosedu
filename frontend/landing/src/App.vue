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
// Inline placement-test widget — lives in the hero, right where the
// old hardcoded "student journal" mock card used to be. Pick a
// language, answer the real backend-scored placement test (20 mixed
// -level questions, no client-side scoring), leave contact + branch
// details and get the server-computed score back, mapped to a
// CEFR/HSK level label client-side.
// ============================================================
type PlacementQuestion = {
  id: string;
  question: string;
  option_a: string;
  option_b: string;
  option_c?: string;
  option_d?: string;
};

type Branch = { id: string; name: string };

const trialLang = ref<"english" | "chinese" | null>(null);
const trialStep = ref<"pick" | "quiz" | "contact" | "done">("pick");
const trialQIndex = ref(0);
const trialQuestions = ref<PlacementQuestion[]>([]);
const trialAnswers = ref<Record<string, number>>({});
const trialLoading = ref(false);
const trialLoadError = ref(false);
const trialSubmitting = ref(false);
const trialSubmitError = ref(false);
const trialRateLimited = ref(false);
const trialScore = ref<{ score: number; total: number } | null>(null);
const trialLevel = ref("");
const trialBranchId = ref("");
const branches = ref<Branch[]>([]);
const branchesLoading = ref(false);

// Rough percentage bands, client-side only — the backend never sends
// a level, only a raw score/total (correct answers are never exposed
// either, see loadTrialQuestions).
const CEFR_LEVELS = ["A1", "A2", "B1", "B2", "C1", "C2"];
const HSK_LEVELS = ["HSK1", "HSK2", "HSK3", "HSK4", "HSK5", "HSK6"];

function computeLevel(subject: "english" | "chinese" | null, score: number, total: number): string {
  if (!subject || !total) return "";
  const pct = score / total;
  const levels = subject === "chinese" ? HSK_LEVELS : CEFR_LEVELS;
  if (pct < 0.25) return levels[0];
  if (pct < 0.4) return levels[1];
  if (pct < 0.55) return levels[2];
  if (pct < 0.7) return levels[3];
  if (pct < 0.85) return levels[4];
  return levels[5];
}

function trialOptions(question: PlacementQuestion): string[] {
  return [question.option_a, question.option_b, question.option_c, question.option_d].filter(
    (opt): opt is string => !!opt,
  );
}

async function loadBranches() {
  if (branches.value.length || branchesLoading.value) return;
  branchesLoading.value = true;
  try {
    const res = await fetch("/api/public/branches");
    if (!res.ok) throw new Error("request failed");
    const data = await res.json();
    branches.value = data.branches ?? [];
  } catch {
    branches.value = [];
  } finally {
    branchesLoading.value = false;
  }
}

function resetTrial() {
  trialLang.value = null;
  trialStep.value = "pick";
  trialQIndex.value = 0;
  trialQuestions.value = [];
  trialAnswers.value = {};
  trialLoadError.value = false;
  trialSubmitError.value = false;
  trialRateLimited.value = false;
  trialScore.value = null;
  trialLevel.value = "";
  trialBranchId.value = "";
  fullName.value = "";
  phone.value = "";
}

async function loadTrialQuestions() {
  if (!trialLang.value) return;
  trialLoading.value = true;
  trialLoadError.value = false;
  try {
    const res = await fetch(`/api/public/placement-test/questions?subject=${trialLang.value}`);
    if (!res.ok) throw new Error("request failed");
    const data = await res.json();
    trialQuestions.value = data.questions ?? [];
  } catch {
    trialLoadError.value = true;
  } finally {
    trialLoading.value = false;
  }
}

function pickTrialLang(lang: "english" | "chinese") {
  trialLang.value = lang;
  trialStep.value = "quiz";
  trialQIndex.value = 0;
  trialAnswers.value = {};
  void loadTrialQuestions();
}

function answerTrial(optionIndex: number) {
  const question = trialQuestions.value[trialQIndex.value];
  if (!question) return;
  trialAnswers.value[question.id] = optionIndex;
  if (trialQIndex.value + 1 < trialQuestions.value.length) {
    trialQIndex.value++;
  } else {
    trialStep.value = "contact";
    void loadBranches();
  }
}

async function submitTrialContact() {
  trialSubmitting.value = true;
  trialSubmitError.value = false;
  trialRateLimited.value = false;
  try {
    const answers = Object.entries(trialAnswers.value).map(([question_id, selected]) => ({
      question_id,
      selected,
    }));
    const res = await fetch("/api/public/placement-test/submit", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        full_name: fullName.value,
        phone: phone.value,
        subject: trialLang.value,
        branch_id: trialBranchId.value || undefined,
        answers,
      }),
    });
    if (!res.ok) {
      if (res.status === 429) trialRateLimited.value = true;
      throw new Error("request failed");
    }
    const data = await res.json();
    trialScore.value = { score: data.score, total: data.total };
    trialLevel.value = computeLevel(trialLang.value, data.score, data.total);
    fullName.value = "";
    phone.value = "";
    trialStep.value = "done";
  } catch {
    trialSubmitError.value = true;
  } finally {
    trialSubmitting.value = false;
  }
}

// ============================================================
// Prodlenka info modal — "Продленка" in the nav opens this.
// ============================================================
const showProdlenkaModal = ref(false);

// ============================================================
// Program card icons (Programs grid, below) — plain emoji, no icon
// library in this repo.
// ============================================================
const programIcons: Record<"prodlenka" | "mad" | "english" | "chinese", string> = {
  prodlenka: "📚",
  mad: "🧩",
  english: "🇬🇧",
  chinese: "🇨🇳",
};
</script>

<template>
  <header class="sticky top-0 z-30 border-b border-line bg-surface1/90 backdrop-blur">
    <div class="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 sm:px-6 lg:px-8">
      <div class="flex items-center gap-3">
        <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
        <span class="font-display text-lg font-bold text-ink">{{ t("app.title") }}</span>
      </div>

      <nav class="hidden items-center gap-8 md:flex">
        <a href="#placement-widget" class="text-sm font-medium text-ink-muted hover:text-ink">
          {{ t("nav.courses") }}
        </a>
        <button type="button" class="text-sm font-medium text-ink-muted hover:text-ink" @click="showProdlenkaModal = true">
          {{ t("nav.prodlenka") }}
        </button>
      </nav>

      <div class="flex items-center gap-3">
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

      <div id="placement-widget" class="scroll-mt-24">
        <div class="rounded-2xl border border-line bg-surface1 p-5 shadow-2xl shadow-accent/10 sm:p-6">
          <div class="mb-1 flex items-center justify-between">
            <h2 class="font-display text-lg font-bold text-ink">{{ t("trial.title") }}</h2>
            <span
              v-if="trialStep === 'quiz' && trialQuestions.length"
              class="font-numeric shrink-0 text-xs font-semibold text-ink-muted"
            >
              {{ t("trial.question") }} {{ trialQIndex + 1 }} / {{ trialQuestions.length }}
            </span>
          </div>

          <div
            v-if="trialStep === 'quiz' && trialQuestions.length"
            class="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-surface2"
          >
            <div
              class="h-full rounded-full bg-accent transition-all duration-300 ease-out"
              :style="{ width: `${((trialQIndex + 1) / trialQuestions.length) * 100}%` }"
            ></div>
          </div>

          <template v-if="trialStep === 'pick'">
            <p class="mb-4 mt-3 text-sm text-ink-muted">{{ t("trial.pickLanguage") }}</p>
            <div class="grid grid-cols-2 gap-3">
              <button
                type="button"
                class="rounded-lg border border-line px-4 py-4 text-center text-sm font-semibold text-ink transition hover:-translate-y-0.5 hover:border-accent hover:text-accent"
                @click="pickTrialLang('english')"
              >
                <span class="block text-2xl">🇬🇧</span>
                <span class="mt-2 block">{{ t("trial.english") }}</span>
              </button>
              <button
                type="button"
                class="rounded-lg border border-line px-4 py-4 text-center text-sm font-semibold text-ink transition hover:-translate-y-0.5 hover:border-accent hover:text-accent"
                @click="pickTrialLang('chinese')"
              >
                <span class="block text-2xl">🇨🇳</span>
                <span class="mt-2 block">{{ t("trial.chinese") }}</span>
              </button>
            </div>
          </template>

          <template v-else-if="trialStep === 'quiz' && trialLang">
            <template v-if="trialLoading">
              <p class="mt-4 text-sm text-ink-muted">{{ t("trial.loading") }}</p>
            </template>
            <template v-else-if="trialLoadError">
              <p class="mt-4 text-sm text-danger">{{ t("trial.loadError") }}</p>
              <button
                type="button"
                class="mt-4 w-full rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink"
                @click="loadTrialQuestions"
              >
                {{ t("trial.retry") }}
              </button>
            </template>
            <template v-else-if="trialQuestions.length">
              <Transition name="q-fade" mode="out-in">
                <div :key="trialQIndex" class="mt-4">
                  <p class="mb-4 text-sm font-medium text-ink">{{ trialQuestions[trialQIndex].question }}</p>
                  <div class="space-y-2">
                    <button
                      v-for="(opt, i) in trialOptions(trialQuestions[trialQIndex])"
                      :key="i"
                      type="button"
                      class="block w-full rounded-lg border border-line px-4 py-2 text-left text-sm text-ink transition hover:border-accent hover:text-accent"
                      @click="answerTrial(i)"
                    >
                      {{ opt }}
                    </button>
                  </div>
                </div>
              </Transition>
            </template>
          </template>

          <template v-else-if="trialStep === 'contact'">
            <p class="mb-4 mt-3 text-sm text-ink-muted">{{ t("trial.contactPrompt") }}</p>
            <form class="space-y-3" @submit.prevent="submitTrialContact">
              <label class="block text-sm text-ink-muted">
                {{ t("leadForm.fullName") }}
                <input v-model="fullName" required class="mt-1" />
              </label>
              <label class="block text-sm text-ink-muted">
                {{ t("leadForm.phone") }}
                <input v-model="phone" type="tel" required class="mt-1" />
              </label>
              <label class="block text-sm text-ink-muted">
                {{ t("trial.branchLabel") }}
                <select v-model="trialBranchId" class="mt-1">
                  <option value="">{{ t("trial.branchPlaceholder") }}</option>
                  <option v-for="b in branches" :key="b.id" :value="b.id">{{ b.name }}</option>
                </select>
              </label>
              <button
                type="submit"
                :disabled="trialSubmitting"
                class="font-accent w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90 disabled:opacity-60"
              >
                {{ t("trial.submit") }}
              </button>
              <p v-if="trialRateLimited" class="text-center text-sm text-danger">{{ t("trial.rateLimited") }}</p>
              <p v-else-if="trialSubmitError" class="text-center text-sm text-danger">{{ t("trial.submitError") }}</p>
            </form>
          </template>

          <template v-else-if="trialStep === 'done'">
            <div class="mt-3 text-center">
              <p class="text-sm text-success">{{ t("trial.success") }}</p>
              <p v-if="trialScore" class="font-numeric mt-3 text-3xl font-bold text-ink">
                {{ trialScore.score }} / {{ trialScore.total }}
              </p>
              <p v-if="trialScore" class="text-xs text-ink-muted">{{ t("trial.scoreLabel") }}</p>
              <p v-if="trialLevel" class="font-accent mt-3 inline-block rounded-full bg-badge1 px-4 py-1.5 text-sm font-semibold text-badge1-fg">
                {{ t("trial.levelLabel", { level: trialLevel }) }}
              </p>
              <button
                type="button"
                class="mt-5 w-full rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink hover:bg-surface2"
                @click="resetTrial"
              >
                {{ t("trial.startOver") }}
              </button>
            </div>
          </template>
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
          class="flex flex-col rounded-2xl border border-line bg-surface2 p-6 shadow-sm transition hover:-translate-y-1 hover:shadow-lg hover:shadow-accent/10"
        >
          <span class="flex h-11 w-11 items-center justify-center rounded-xl bg-badge1 text-xl">
            {{ programIcons[key] }}
          </span>
          <div class="mt-4 flex flex-wrap items-center gap-2">
            <h3 class="font-display text-lg font-bold text-ink">{{ t(`programs.${key}.title`) }}</h3>
          </div>
          <span class="font-accent mt-1 inline-block w-fit rounded-full bg-badge1 px-2.5 py-1 text-xs font-semibold text-badge1-fg">
            {{ t(`programs.${key}.badge`) }}
          </span>
          <ul class="mt-4 flex-1 space-y-2">
            <li
              v-for="(bullet, i) in (tm(`programs.${key}.bullets`) as unknown as string[])"
              :key="i"
              class="flex items-start gap-2 text-sm text-ink-muted"
            >
              <span class="mt-0.5 text-teal">✓</span>{{ bullet }}
            </li>
          </ul>
          <a
            v-if="key === 'english' || key === 'chinese'"
            href="#placement-widget"
            class="mt-5 inline-flex w-fit items-center gap-1 border-t border-line pt-4 text-sm font-semibold text-accent hover:underline"
          >
            {{ t("programs.tryTest") }} →
          </a>
          <a
            v-else
            href="#contact"
            class="mt-5 inline-flex w-fit items-center gap-1 border-t border-line pt-4 text-sm font-semibold text-accent hover:underline"
          >
            {{ t("prodlenka.cta") }} →
          </a>
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
</template>

<style scoped>
.q-fade-enter-active,
.q-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.q-fade-enter-from {
  opacity: 0;
  transform: translateX(16px);
}
.q-fade-leave-to {
  opacity: 0;
  transform: translateX(-16px);
}
</style>
