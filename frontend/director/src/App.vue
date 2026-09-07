<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import LangSwitcher from "./components/LangSwitcher.vue";
import ThemeToggle from "./components/ThemeToggle.vue";

const { t } = useI18n();

const token = ref<string | null>(localStorage.getItem("dosedu_token"));
const identifier = ref("");
const password = ref("");
const loginError = ref(false);

type Tab = "leads" | "students" | "staff" | "payments" | "schedule" | "reports" | "tickets";
const activeTab = ref<Tab>("leads");

function authHeaders() {
  return { Authorization: `Bearer ${token.value}` };
}

// --- Leads Kanban ---
type Lead = { id: string; full_name: string; phone: string };
const columns = ref<Record<string, Lead[]>>({ new: [], contacted: [], trial_scheduled: [], paid: [], lost: [] });
const stageOrder = ["new", "contacted", "trial_scheduled", "paid", "lost"] as const;
type Stage = (typeof stageOrder)[number];
const newCredentials = ref<{ loginCode: string; password: string; notice: string } | null>(null);

async function loadLeads() {
  if (!token.value) return;
  const res = await fetch("/api/leads", { headers: authHeaders() });
  if (res.ok) columns.value = (await res.json()).columns;
}

async function moveStage(leadId: string, stage: Stage) {
  if (!token.value) return;
  const res = await fetch(`/api/leads/${leadId}/stage`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ stage }),
  });
  if (res.ok) {
    const data = await res.json();
    if (data.student_created) {
      newCredentials.value = { loginCode: data.login_code, password: data.temporary_password, notice: data.credentials_notice };
    }
    await loadLeads();
  }
}

// --- Staff (teachers) ---
type Teacher = { id: string; email: string; full_name: string; subject: string; language_scope?: string };
const teachers = ref<Teacher[]>([]);
const newTeacherEmail = ref("");
const newTeacherName = ref("");
const newTeacherSubject = ref("");
const newTeacherLanguageScope = ref<"" | "en" | "zh">("");
const newTeacherCredentials = ref<{ email: string; password: string; notice: string } | null>(null);

async function loadTeachers() {
  if (!token.value) return;
  const res = await fetch("/api/teachers", { headers: authHeaders() });
  if (res.ok) teachers.value = (await res.json()).teachers ?? [];
}

async function createTeacher() {
  if (!token.value || !newTeacherEmail.value) return;
  const res = await fetch("/api/teachers", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      email: newTeacherEmail.value,
      full_name: newTeacherName.value,
      subject: newTeacherSubject.value,
      language_scope: newTeacherLanguageScope.value || undefined,
    }),
  });
  if (res.ok) {
    const data = await res.json();
    newTeacherCredentials.value = { email: data.email, password: data.temporary_password, notice: data.credentials_notice };
    newTeacherEmail.value = "";
    newTeacherName.value = "";
    newTeacherSubject.value = "";
    newTeacherLanguageScope.value = "";
    await loadTeachers();
  }
}

// --- Students (full branch roster) ---
type Student = {
  id: string;
  full_name: string;
  level: string;
  language?: string | null;
  balance: number;
  payment_status: string;
};
const students = ref<Student[]>([]);

async function loadStudents() {
  if (!token.value) return;
  const res = await fetch("/api/students", { headers: authHeaders() });
  if (res.ok) students.value = (await res.json()).students ?? [];
}

// --- Payments (full branch history) ---
type Payment = {
  id: string;
  student_name: string;
  amount: number;
  status: string;
  method: string;
  period_start: string;
  period_end: string;
  created_at: string;
};
const payments = ref<Payment[]>([]);

async function loadPayments() {
  if (!token.value) return;
  const res = await fetch("/api/payments", { headers: authHeaders() });
  if (res.ok) payments.value = (await res.json()).payments ?? [];
}

// --- Schedule (rooms, groups, slots — Conflict Checker) ---
type Room = { id: string; name: string };
type Group = { id: string; name: string; teacher_id: string; program: string; level: string };
type ScheduleSlot = { id: string; group_id: string; teacher_id: string; room_id: string; weekday: number; start_time: string; end_time: string };
const rooms = ref<Room[]>([]);
const groups = ref<Group[]>([]);
const slots = ref<ScheduleSlot[]>([]);
const newRoomName = ref("");
const newGroupName = ref("");
const newGroupTeacherId = ref("");
const newGroupProgram = ref<"language_course" | "prodlenka">("language_course");
const newGroupLevel = ref("");
const slotGroupId = ref("");
const slotTeacherId = ref("");
const slotRoomId = ref("");
const slotWeekday = ref(1);
const slotStart = ref("16:00");
const slotEnd = ref("17:30");
const slotError = ref("");
const weekdayNames = ["", "Дс", "Сс", "Ср", "Бс", "Жм", "Сб", "Жс"];

async function loadScheduleData() {
  if (!token.value) return;
  const [r, g, s] = await Promise.all([
    fetch("/api/rooms", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/groups", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch("/api/schedule", { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
  ]);
  rooms.value = r?.rooms ?? [];
  groups.value = g?.groups ?? [];
  slots.value = s?.schedule ?? [];
}

async function createRoom() {
  if (!token.value || !newRoomName.value) return;
  const res = await fetch("/api/rooms", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name: newRoomName.value }),
  });
  if (res.ok) {
    newRoomName.value = "";
    await loadScheduleData();
  }
}

async function createGroup() {
  if (!token.value || !newGroupName.value || !newGroupTeacherId.value) return;
  const res = await fetch("/api/groups", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      name: newGroupName.value,
      teacher_id: newGroupTeacherId.value,
      program: newGroupProgram.value,
      level: newGroupLevel.value,
    }),
  });
  if (res.ok) {
    newGroupName.value = "";
    newGroupLevel.value = "";
    await loadScheduleData();
  }
}

async function createSlot() {
  if (!token.value || !slotGroupId.value || !slotTeacherId.value || !slotRoomId.value) return;
  slotError.value = "";
  const res = await fetch("/api/schedule", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      group_id: slotGroupId.value,
      teacher_id: slotTeacherId.value,
      room_id: slotRoomId.value,
      weekday: slotWeekday.value,
      start_time: slotStart.value,
      end_time: slotEnd.value,
    }),
  });
  if (res.status === 409) {
    const data = await res.json();
    slotError.value = data.message ?? t("schedule.conflict");
    return;
  }
  if (res.ok) await loadScheduleData();
}

function roomName(id: string) {
  return rooms.value.find((r) => r.id === id)?.name ?? id;
}
function groupName(id: string) {
  return groups.value.find((g) => g.id === id)?.name ?? id;
}
function teacherName(id: string) {
  return teachers.value.find((t) => t.id === id)?.full_name ?? id;
}

// --- Reports ---
type OverdueStudent = { id: string; full_name: string; payment_status: string };
const overdueStudents = ref<OverdueStudent[]>([]);

async function loadReports() {
  if (!token.value) return;
  const res = await fetch("/api/reports", { headers: authHeaders() });
  if (res.ok) overdueStudents.value = (await res.json()).overdue_students ?? [];
}

// --- Tickets ---
type Ticket = { id: string; subject: string; status: string; created_by_role: string; created_at: string };
const tickets = ref<Ticket[]>([]);
const newTicketSubject = ref("");
const newTicketMessage = ref("");

async function loadTickets() {
  if (!token.value) return;
  const res = await fetch("/api/tickets", { headers: authHeaders() });
  if (res.ok) tickets.value = (await res.json()).tickets ?? [];
}

async function createTicket() {
  if (!token.value || !newTicketSubject.value || !newTicketMessage.value) return;
  const res = await fetch("/api/tickets", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ subject: newTicketSubject.value, message: newTicketMessage.value }),
  });
  if (res.ok) {
    newTicketSubject.value = "";
    newTicketMessage.value = "";
    await loadTickets();
  }
}

async function selectTab(tab: Tab) {
  activeTab.value = tab;
  if (tab === "students") await loadStudents();
  if (tab === "staff") await loadTeachers();
  if (tab === "payments") await loadPayments();
  if (tab === "schedule") { await loadTeachers(); await loadScheduleData(); }
  if (tab === "reports") await loadReports();
  if (tab === "tickets") await loadTickets();
}

async function login() {
  loginError.value = false;
  try {
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ identifier: identifier.value, password: password.value, role: "director" }),
    });
    if (!res.ok) throw new Error("invalid");
    const data = await res.json();
    token.value = data.token;
    localStorage.setItem("dosedu_token", data.token);
    activeTab.value = "leads";
    await loadLeads();
  } catch {
    loginError.value = true;
  }
}

function logout() {
  token.value = null;
  localStorage.removeItem("dosedu_token");
}

onMounted(loadLeads);
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

  <main class="px-4 py-6 sm:px-6">
    <form v-if="!token" class="mx-auto max-w-sm space-y-4 rounded-xl border border-line bg-surface2 p-5 sm:p-6" @submit.prevent="login">
      <h2 class="font-display text-lg font-bold text-ink">{{ t("login.title") }}</h2>
      <label class="block text-sm text-ink-muted">
        {{ t("login.identifier") }}
        <input v-model="identifier" type="email" required class="mt-1" />
      </label>
      <label class="block text-sm text-ink-muted">
        {{ t("login.password") }}
        <input v-model="password" type="password" required minlength="8" maxlength="8" class="mt-1" />
      </label>
      <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-accent-contrast hover:opacity-90">
        {{ t("login.submit") }}
      </button>
      <p v-if="loginError" class="text-center text-sm text-danger">{{ t("login.error") }}</p>
    </form>

    <div v-else class="mx-auto max-w-5xl">
      <div class="flex gap-1 overflow-x-auto border-b border-line">
        <button
          v-for="tab in (['leads', 'students', 'staff', 'payments', 'schedule', 'reports', 'tickets'] as Tab[])"
          :key="tab"
          type="button"
          class="font-accent shrink-0 border-b-2 px-4 py-2 text-sm font-medium"
          :class="activeTab === tab ? 'border-accent text-accent' : 'border-transparent text-ink-muted hover:text-ink'"
          @click="selectTab(tab)"
        >
          {{ t(`tabs.${tab}`) }}
        </button>
      </div>

      <!-- Leads -->
      <section v-if="activeTab === 'leads'" class="mt-4">
        <div v-if="newCredentials" class="mb-4 flex items-start justify-between gap-3 rounded-xl border border-accent bg-surface2 p-4">
          <div class="text-sm text-ink">
            <p class="font-semibold">
              Логин: <code class="rounded bg-surface3 px-1.5 py-0.5">{{ newCredentials.loginCode }}</code>
              · Пароль: <code class="rounded bg-surface3 px-1.5 py-0.5">{{ newCredentials.password }}</code>
            </p>
            <p class="mt-1 text-ink-muted">{{ newCredentials.notice }}</p>
          </div>
          <button type="button" class="shrink-0 text-ink-muted hover:text-ink" @click="newCredentials = null">✕</button>
        </div>

        <div class="flex gap-4 overflow-x-auto pb-2">
          <div v-for="stage in stageOrder" :key="stage" class="w-64 shrink-0 rounded-xl border border-line bg-surface2 p-4">
            <h3 class="font-display text-sm font-semibold text-ink">{{ t(`leads.${stage}`) }}</h3>
            <div v-for="lead in columns[stage]" :key="lead.id" class="mt-3 rounded-lg border border-line bg-surface1 p-3">
              <p class="text-sm font-semibold text-ink">{{ lead.full_name }}</p>
              <p class="text-xs text-ink-muted">{{ lead.phone }}</p>
              <select
                class="mt-2 w-full rounded-md border border-line bg-surface1 px-2 py-1 text-xs text-ink"
                :value="stage"
                @change="moveStage(lead.id, ($event.target as HTMLSelectElement).value as Stage)"
              >
                <option v-for="s in stageOrder" :key="s" :value="s">{{ t(`leads.${s}`) }}</option>
              </select>
            </div>
          </div>
        </div>
      </section>

      <!-- Staff -->
      <section v-if="activeTab === 'staff'" class="mt-4 grid gap-4 sm:grid-cols-2">
        <div>
          <form class="space-y-3 rounded-xl border border-line bg-surface2 p-5" @submit.prevent="createTeacher">
            <h3 class="font-display text-sm font-semibold text-ink">{{ t("staff.add") }}</h3>
            <label class="block text-sm text-ink-muted">Email<input v-model="newTeacherEmail" type="email" required class="mt-1" /></label>
            <label class="block text-sm text-ink-muted">{{ t("staff.fullName") }}<input v-model="newTeacherName" required class="mt-1" /></label>
            <label class="block text-sm text-ink-muted">{{ t("staff.subject") }}<input v-model="newTeacherSubject" class="mt-1" /></label>
            <label class="block text-sm text-ink-muted">
              {{ t("staff.languageScope") }}
              <select v-model="newTeacherLanguageScope" class="mt-1">
                <option value="">{{ t("staff.languageScopeNone") }}</option>
                <option value="en">{{ t("staff.languageScopeEn") }}</option>
                <option value="zh">{{ t("staff.languageScopeZh") }}</option>
              </select>
            </label>
            <button type="submit" class="font-accent w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast">{{ t("staff.create") }}</button>
          </form>
          <div v-if="newTeacherCredentials" class="mt-3 rounded-xl border border-accent bg-surface2 p-4 text-sm">
            <p class="font-semibold text-ink">
              {{ newTeacherCredentials.email }} ·
              <code class="rounded bg-surface3 px-1.5 py-0.5">{{ newTeacherCredentials.password }}</code>
            </p>
            <p class="mt-1 text-ink-muted">{{ newTeacherCredentials.notice }}</p>
          </div>
        </div>
        <div class="space-y-2 rounded-xl border border-line bg-surface2 p-5">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("staff.list") }}</h3>
          <div v-for="teacher in teachers" :key="teacher.id" class="flex items-center justify-between rounded-lg border border-line bg-surface1 p-3 text-sm">
            <div>
              <p class="font-semibold text-ink">{{ teacher.full_name || teacher.email }}</p>
              <p class="text-ink-muted">{{ teacher.email }} · {{ teacher.subject }}</p>
            </div>
            <span
              v-if="teacher.language_scope"
              class="font-accent shrink-0 rounded-full bg-badge1 px-2 py-0.5 text-xs font-semibold text-badge1-fg"
            >
              {{ teacher.language_scope === "zh" ? "中文" : "EN" }}
            </span>
          </div>
          <p v-if="!teachers.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>

      <!-- Students -->
      <section v-if="activeTab === 'students'" class="mt-4 overflow-x-auto rounded-xl border border-line bg-surface2 p-5">
        <h3 class="font-display mb-3 text-sm font-semibold text-ink">{{ t("students.title") }}</h3>
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-line text-ink-muted">
              <th class="pb-2 font-medium">{{ t("students.name") }}</th>
              <th class="pb-2 font-medium">{{ t("students.level") }}</th>
              <th class="pb-2 font-medium">{{ t("students.balance") }}</th>
              <th class="pb-2 font-medium">{{ t("students.status") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in students" :key="s.id" class="border-b border-line last:border-0">
              <td class="py-2 text-ink">{{ s.full_name }}</td>
              <td class="py-2 text-ink-muted">{{ s.level || "—" }}</td>
              <td class="font-numeric py-2 text-ink">{{ s.balance.toLocaleString() }} ₸</td>
              <td class="py-2">
                <span
                  class="font-accent rounded-full px-2 py-0.5 text-xs font-semibold"
                  :class="s.payment_status === 'paid' ? 'bg-success/15 text-success' : 'bg-danger/15 text-danger'"
                >
                  {{ s.payment_status }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="!students.length" class="mt-2 text-sm text-ink-muted">—</p>
      </section>

      <!-- Payments -->
      <section v-if="activeTab === 'payments'" class="mt-4 overflow-x-auto rounded-xl border border-line bg-surface2 p-5">
        <h3 class="font-display mb-3 text-sm font-semibold text-ink">{{ t("payments.title") }}</h3>
        <table class="w-full text-left text-sm">
          <thead>
            <tr class="border-b border-line text-ink-muted">
              <th class="pb-2 font-medium">{{ t("payments.student") }}</th>
              <th class="pb-2 font-medium">{{ t("payments.amount") }}</th>
              <th class="pb-2 font-medium">{{ t("payments.method") }}</th>
              <th class="pb-2 font-medium">{{ t("payments.date") }}</th>
              <th class="pb-2 font-medium">{{ t("payments.status") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in payments" :key="p.id" class="border-b border-line last:border-0">
              <td class="py-2 text-ink">{{ p.student_name }}</td>
              <td class="font-numeric py-2 text-ink">{{ p.amount.toLocaleString() }} ₸</td>
              <td class="py-2 text-ink-muted">{{ p.method }}</td>
              <td class="font-numeric py-2 text-ink-muted">{{ new Date(p.created_at).toLocaleDateString() }}</td>
              <td class="py-2">
                <span class="font-accent rounded-full bg-success/15 px-2 py-0.5 text-xs font-semibold text-success">{{ p.status }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="!payments.length" class="mt-2 text-sm text-ink-muted">—</p>
      </section>

      <!-- Schedule -->
      <section v-if="activeTab === 'schedule'" class="mt-4 space-y-4">
        <div class="grid gap-4 sm:grid-cols-3">
          <form class="space-y-3 rounded-xl border border-line bg-surface2 p-4" @submit.prevent="createRoom">
            <h3 class="font-display text-sm font-semibold text-ink">{{ t("schedule.addRoom") }}</h3>
            <input v-model="newRoomName" :placeholder="t('schedule.roomName')" required class="text-sm" />
            <button type="submit" class="w-full rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-contrast">{{ t("schedule.add") }}</button>
            <div class="mt-2 flex flex-wrap gap-1">
              <span v-for="room in rooms" :key="room.id" class="rounded-full bg-surface3 px-2 py-0.5 text-xs text-ink">{{ room.name }}</span>
            </div>
          </form>

          <form class="space-y-2 rounded-xl border border-line bg-surface2 p-4 sm:col-span-2" @submit.prevent="createGroup">
            <h3 class="font-display text-sm font-semibold text-ink">{{ t("schedule.addGroup") }}</h3>
            <div class="grid grid-cols-2 gap-2">
              <input v-model="newGroupName" :placeholder="t('schedule.groupName')" required class="text-sm" />
              <select v-model="newGroupTeacherId" required class="text-sm">
                <option value="" disabled>{{ t("schedule.teacher") }}</option>
                <option v-for="teacher in teachers" :key="teacher.id" :value="teacher.id">{{ teacher.full_name }}</option>
              </select>
              <select v-model="newGroupProgram" class="text-sm">
                <option value="language_course">{{ t("schedule.languageCourse") }}</option>
                <option value="prodlenka">{{ t("schedule.prodlenka") }}</option>
              </select>
              <input v-model="newGroupLevel" :placeholder="t('schedule.level')" class="text-sm" />
            </div>
            <button type="submit" class="w-full rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-contrast">{{ t("schedule.add") }}</button>
          </form>
        </div>

        <form class="space-y-3 rounded-xl border border-line bg-surface2 p-4" @submit.prevent="createSlot">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("schedule.addSlot") }}</h3>
          <div class="grid gap-2 sm:grid-cols-3">
            <select v-model="slotGroupId" required class="text-sm">
              <option value="" disabled>{{ t("schedule.group") }}</option>
              <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
            </select>
            <select v-model="slotTeacherId" required class="text-sm">
              <option value="" disabled>{{ t("schedule.teacher") }}</option>
              <option v-for="teacher in teachers" :key="teacher.id" :value="teacher.id">{{ teacher.full_name }}</option>
            </select>
            <select v-model="slotRoomId" required class="text-sm">
              <option value="" disabled>{{ t("schedule.room") }}</option>
              <option v-for="room in rooms" :key="room.id" :value="room.id">{{ room.name }}</option>
            </select>
            <select v-model.number="slotWeekday" class="text-sm">
              <option v-for="d in [1, 2, 3, 4, 5, 6, 7]" :key="d" :value="d">{{ weekdayNames[d] }}</option>
            </select>
            <input v-model="slotStart" type="time" class="text-sm" />
            <input v-model="slotEnd" type="time" class="text-sm" />
          </div>
          <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast sm:w-auto">
            {{ t("schedule.book") }}
          </button>
          <p v-if="slotError" class="text-sm font-medium text-danger">⚠️ {{ slotError }}</p>
        </form>

        <div class="rounded-xl border border-line bg-surface2 p-4">
          <h3 class="mb-2 font-display text-sm font-semibold text-ink">{{ t("schedule.grid") }}</h3>
          <div v-for="slot in slots" :key="slot.id" class="border-b border-line py-2 text-sm last:border-0">
            <span class="font-semibold text-ink">{{ weekdayNames[slot.weekday] }} {{ slot.start_time }}–{{ slot.end_time }}</span>
            <span class="text-ink-muted"> · {{ groupName(slot.group_id) }} · {{ teacherName(slot.teacher_id) }} · {{ roomName(slot.room_id) }}</span>
          </div>
          <p v-if="!slots.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>

      <!-- Reports -->
      <section v-if="activeTab === 'reports'" class="mt-4 rounded-xl border border-line bg-surface2 p-5">
        <h3 class="font-display text-sm font-semibold text-ink">{{ t("reports.overdue") }}</h3>
        <div v-for="s in overdueStudents" :key="s.id" class="mt-2 flex items-center justify-between border-b border-line py-2 text-sm last:border-0">
          <span class="text-ink">{{ s.full_name }}</span>
          <span class="rounded-full bg-danger/15 px-2 py-0.5 text-xs font-semibold text-danger">{{ s.payment_status }}</span>
        </div>
        <p v-if="!overdueStudents.length" class="mt-2 text-sm text-ink-muted">—</p>
      </section>

      <!-- Tickets -->
      <section v-if="activeTab === 'tickets'" class="mt-4 grid gap-4 sm:grid-cols-2">
        <form class="space-y-3 rounded-xl border border-line bg-surface2 p-5" @submit.prevent="createTicket">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("tickets.new") }}</h3>
          <input v-model="newTicketSubject" :placeholder="t('tickets.subject')" required class="text-sm" />
          <textarea v-model="newTicketMessage" :placeholder="t('tickets.message')" required rows="3" class="w-full rounded-lg border border-line bg-surface1 px-3 py-2 text-sm text-ink"></textarea>
          <button type="submit" class="w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast">{{ t("tickets.open") }}</button>
        </form>
        <div class="space-y-2 rounded-xl border border-line bg-surface2 p-5">
          <h3 class="font-display text-sm font-semibold text-ink">{{ t("tickets.inbox") }}</h3>
          <div v-for="ticket in tickets" :key="ticket.id" class="rounded-lg border border-line bg-surface1 p-3 text-sm">
            <p class="font-semibold text-ink">{{ ticket.subject }}</p>
            <p class="text-xs text-ink-muted">{{ ticket.created_by_role }} · {{ ticket.status }}</p>
          </div>
          <p v-if="!tickets.length" class="text-sm text-ink-muted">—</p>
        </div>
      </section>
    </div>
  </main>
</template>
