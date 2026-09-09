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

type Tab = "leads" | "students" | "staff" | "payments" | "schedule" | "reports" | "tickets";
const activeTab = ref<Tab>("leads");

function authHeaders() {
  return { Authorization: `Bearer ${token.value}` };
}

// --- JWT claims (client-side decode, display-only — the server is the
// real authority on branch scoping) ---
type Claims = { uid: string; role: string; branch_id?: string; subject?: string; is_network_owner?: boolean };
function decodeToken(tok: string | null): Claims | null {
  if (!tok) return null;
  try {
    const payload = tok.split(".")[1];
    return JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/")));
  } catch {
    return null;
  }
}
const claims = computed(() => decodeToken(token.value));
const isNetworkOwner = computed(() => claims.value?.is_network_owner === true);

// --- Branch switcher (network-owner directors only) ---
type Branch = { id: string; name: string; address: string; status: string; created_at: string };
const branches = ref<Branch[]>([]);
const selectedBranchId = ref("");
// A network-owner must pick a concrete branch before creating anything
// (teacher / room / group) — the backend 400s on branch_id-less writes.
const branchRequired = computed(() => isNetworkOwner.value && !selectedBranchId.value);

async function loadBranches() {
  if (!token.value) return;
  const res = await fetch("/api/branches", { headers: authHeaders() });
  if (res.ok) branches.value = (await res.json()).branches ?? [];
}

function withBranch(path: string) {
  if (isNetworkOwner.value && selectedBranchId.value) {
    return `${path}${path.includes("?") ? "&" : "?"}branch_id=${encodeURIComponent(selectedBranchId.value)}`;
  }
  return path;
}

// --- Leads Kanban ---
type Lead = {
  id: string;
  branch_id: string;
  full_name: string;
  phone: string;
  level_test_result: string;
  subject?: "english" | "chinese";
  stage: string;
};
const columns = ref<Record<string, Lead[]>>({ new: [], contacted: [], trial_scheduled: [], paid: [], lost: [] });
const stageOrder = ["new", "contacted", "trial_scheduled", "paid", "lost"] as const;
type Stage = (typeof stageOrder)[number];
const newCredentials = ref<{ loginCode: string; password: string; notice: string } | null>(null);

async function loadLeads() {
  if (!token.value) return;
  const res = await fetch(withBranch("/api/leads"), { headers: authHeaders() });
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
type Teacher = { id: string; email: string; full_name: string; subject: string };
const teachers = ref<Teacher[]>([]);
const newTeacherEmail = ref("");
const newTeacherName = ref("");
const newTeacherSubject = ref<"" | "english" | "chinese" | "mad" | "prodlenka">("");
const newTeacherCredentials = ref<{ email: string; password: string; notice: string } | null>(null);

async function loadTeachers() {
  if (!token.value) return;
  const res = await fetch(withBranch("/api/teachers"), { headers: authHeaders() });
  if (res.ok) teachers.value = (await res.json()).teachers ?? [];
}

async function createTeacher() {
  if (!token.value || !newTeacherEmail.value || branchRequired.value) return;
  const res = await fetch("/api/teachers", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      email: newTeacherEmail.value,
      full_name: newTeacherName.value,
      subject: newTeacherSubject.value || undefined,
      ...(isNetworkOwner.value ? { branch_id: selectedBranchId.value } : {}),
    }),
  });
  if (res.ok) {
    const data = await res.json();
    newTeacherCredentials.value = { email: data.email, password: data.temporary_password, notice: data.credentials_notice };
    newTeacherEmail.value = "";
    newTeacherName.value = "";
    newTeacherSubject.value = "";
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
  const res = await fetch(withBranch("/api/students"), { headers: authHeaders() });
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
  const res = await fetch(withBranch("/api/payments"), { headers: authHeaders() });
  if (res.ok) payments.value = (await res.json()).payments ?? [];
}

// --- Schedule (rooms, groups, slots — Conflict Checker) ---
type Room = { id: string; name: string };
type Group = {
  id: string;
  name: string;
  teacher_id: string;
  course_type: "language" | "care_and_prep";
  subject?: string;
  level?: string;
};
type ScheduleSlot = { id: string; group_id: string; teacher_id: string; room_id: string; weekday: number; start_time: string; end_time: string };
const rooms = ref<Room[]>([]);
const groups = ref<Group[]>([]);
const slots = ref<ScheduleSlot[]>([]);
const newRoomName = ref("");
const newGroupName = ref("");
const newGroupTeacherId = ref("");
const newGroupCourseType = ref<"language" | "care_and_prep">("language");
const languageSubjects = ["english", "chinese"] as const;
const careSubjects = ["mad", "prodlenka"] as const;
const newGroupSubject = ref<(typeof languageSubjects)[number] | (typeof careSubjects)[number]>("english");
const groupSubjectOptions = computed(() => (newGroupCourseType.value === "language" ? languageSubjects : careSubjects));
const cefrLevels = ["A1", "A2", "B1", "B2", "C1", "C2"];
const hskLevels = ["HSK1", "HSK2", "HSK3", "HSK4", "HSK5", "HSK6"];
const groupLevelOptions = computed(() => (newGroupSubject.value === "chinese" ? hskLevels : cefrLevels));
const newGroupLevel = ref(cefrLevels[0]);
const slotGroupId = ref("");
const slotTeacherId = ref("");
const slotRoomId = ref("");
const slotWeekday = ref(1);
const slotStart = ref("16:00");
const slotEnd = ref("17:30");
const slotError = ref("");
const weekdayNames = ["", "Дс", "Сс", "Ср", "Бс", "Жм", "Сб", "Жс"];

function onGroupCourseTypeChange() {
  newGroupSubject.value = groupSubjectOptions.value[0];
  newGroupLevel.value = newGroupCourseType.value === "language" ? groupLevelOptions.value[0] : "";
}
function onGroupSubjectChange() {
  if (newGroupCourseType.value === "language") newGroupLevel.value = groupLevelOptions.value[0];
}

async function loadScheduleData() {
  if (!token.value) return;
  const [r, g, s] = await Promise.all([
    fetch(withBranch("/api/rooms"), { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch(withBranch("/api/groups"), { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
    fetch(withBranch("/api/schedule"), { headers: authHeaders() }).then((r) => (r.ok ? r.json() : null)),
  ]);
  rooms.value = r?.rooms ?? [];
  groups.value = g?.groups ?? [];
  slots.value = s?.schedule ?? [];
}

async function createRoom() {
  if (!token.value || !newRoomName.value || branchRequired.value) return;
  const res = await fetch("/api/rooms", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      name: newRoomName.value,
      ...(isNetworkOwner.value ? { branch_id: selectedBranchId.value } : {}),
    }),
  });
  if (res.ok) {
    newRoomName.value = "";
    await loadScheduleData();
  }
}

async function createGroup() {
  if (!token.value || !newGroupName.value || !newGroupTeacherId.value || branchRequired.value) return;
  const res = await fetch("/api/groups", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      name: newGroupName.value,
      teacher_id: newGroupTeacherId.value,
      course_type: newGroupCourseType.value,
      subject: newGroupSubject.value,
      level: newGroupCourseType.value === "language" ? newGroupLevel.value : "",
      ...(isNetworkOwner.value ? { branch_id: selectedBranchId.value } : {}),
    }),
  });
  if (res.ok) {
    newGroupName.value = "";
    newGroupCourseType.value = "language";
    newGroupSubject.value = "english";
    newGroupLevel.value = cefrLevels[0];
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

// --- Group roster management ---
type RosterStudent = { id: string; full_name: string; level?: string };
const rosterOpenGroupId = ref<string | null>(null);
const rosterStudents = ref<RosterStudent[]>([]);
const rosterAddStudentId = ref("");
const rosterAvailableStudents = computed(() => students.value.filter((s) => !rosterStudents.value.some((rs) => rs.id === s.id)));

async function loadRoster(groupId: string) {
  if (!token.value) return;
  const res = await fetch(`/api/groups/${groupId}/students`, { headers: authHeaders() });
  if (res.ok) rosterStudents.value = (await res.json()).students ?? [];
}

async function toggleRoster(groupId: string) {
  if (rosterOpenGroupId.value === groupId) {
    rosterOpenGroupId.value = null;
    return;
  }
  rosterOpenGroupId.value = groupId;
  rosterAddStudentId.value = "";
  await Promise.all([loadRoster(groupId), loadStudents()]);
}

async function addRosterStudent(groupId: string) {
  if (!token.value || !rosterAddStudentId.value) return;
  const res = await fetch(`/api/groups/${groupId}/students`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ student_id: rosterAddStudentId.value }),
  });
  if (res.ok) {
    rosterAddStudentId.value = "";
    await loadRoster(groupId);
  }
}

async function removeRosterStudent(groupId: string, studentId: string) {
  if (!token.value) return;
  const res = await fetch(`/api/groups/${groupId}/students/${studentId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (res.ok) await loadRoster(groupId);
}

// --- Reports ---
type OverdueStudent = { id: string; full_name: string; payment_status: string };
const overdueStudents = ref<OverdueStudent[]>([]);

async function loadReports() {
  if (!token.value) return;
  const res = await fetch(withBranch("/api/reports"), { headers: authHeaders() });
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

async function loadForTab(tab: Tab) {
  if (tab === "leads") await loadLeads();
  if (tab === "students") await loadStudents();
  if (tab === "staff") await loadTeachers();
  if (tab === "payments") await loadPayments();
  if (tab === "schedule") { await loadTeachers(); await loadScheduleData(); }
  if (tab === "reports") await loadReports();
  if (tab === "tickets") await loadTickets();
}

async function selectTab(tab: Tab) {
  activeTab.value = tab;
  await loadForTab(tab);
}

// Re-fetch whatever the current tab shows when the network-owner
// switches branches (or back to "all branches").
async function onBranchChange() {
  rosterOpenGroupId.value = null;
  await loadForTab(activeTab.value);
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
    selectedBranchId.value = "";
    activeTab.value = "leads";
    if (isNetworkOwner.value) await loadBranches();
    await loadLeads();
  } catch {
    loginError.value = true;
  }
}

function logout() {
  token.value = null;
  localStorage.removeItem("dosedu_token");
}

onMounted(async () => {
  if (isNetworkOwner.value) await loadBranches();
  await loadLeads();
});
</script>

<template>
  <header class="flex items-center justify-between border-b border-line px-4 py-3 sm:px-6">
    <div class="flex items-center gap-3">
      <img src="/logo.svg" alt="DOS EDUCATION" class="logo" />
      <span class="font-display text-base font-bold text-ink sm:text-lg">{{ t("app.title") }}</span>
    </div>
    <div class="flex items-center gap-2 sm:gap-3">
      <select
        v-if="token && isNetworkOwner"
        v-model="selectedBranchId"
        class="rounded-lg border border-line bg-surface1 px-2 py-1 text-sm text-ink"
        @change="onBranchChange"
      >
        <option value="">{{ t("branch.all") }}</option>
        <option v-for="b in branches" :key="b.id" :value="b.id">{{ b.name }}</option>
      </select>
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
              <div v-if="lead.subject || lead.level_test_result" class="mt-1.5 flex flex-wrap gap-1">
                <span v-if="lead.subject" class="font-accent rounded-full bg-badge1 px-2 py-0.5 text-[10px] font-semibold text-badge1-fg">
                  {{ t(`subjects.${lead.subject}`) }}
                </span>
                <span v-if="lead.level_test_result" class="rounded-full bg-surface3 px-2 py-0.5 text-[10px] text-ink-muted">
                  {{ t("leads.levelTest") }}: {{ lead.level_test_result }}
                </span>
              </div>
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
            <label class="block text-sm text-ink-muted">
              {{ t("staff.subject") }}
              <select v-model="newTeacherSubject" class="mt-1">
                <option value="">{{ t("subjects.unassigned") }}</option>
                <option value="english">{{ t("subjects.english") }}</option>
                <option value="chinese">{{ t("subjects.chinese") }}</option>
                <option value="mad">{{ t("subjects.mad") }}</option>
                <option value="prodlenka">{{ t("subjects.prodlenka") }}</option>
              </select>
            </label>
            <button type="submit" :disabled="branchRequired" class="font-accent w-full rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-contrast disabled:opacity-60">{{ t("staff.create") }}</button>
            <p v-if="branchRequired" class="text-xs text-danger">{{ t("branch.required") }}</p>
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
              <p class="text-ink-muted">{{ teacher.email }}</p>
            </div>
            <span
              v-if="teacher.subject"
              class="font-accent shrink-0 rounded-full bg-badge1 px-2 py-0.5 text-xs font-semibold text-badge1-fg"
            >
              {{ t(`subjects.${teacher.subject}`) }}
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
            <button type="submit" :disabled="branchRequired" class="w-full rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-contrast disabled:opacity-60">{{ t("schedule.add") }}</button>
            <p v-if="branchRequired" class="text-xs text-danger">{{ t("branch.required") }}</p>
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
              <select v-model="newGroupCourseType" class="text-sm" @change="onGroupCourseTypeChange">
                <option value="language">{{ t("courseTypes.language") }}</option>
                <option value="care_and_prep">{{ t("courseTypes.care_and_prep") }}</option>
              </select>
              <select v-model="newGroupSubject" class="text-sm" @change="onGroupSubjectChange">
                <option v-for="s in groupSubjectOptions" :key="s" :value="s">{{ t(`subjects.${s}`) }}</option>
              </select>
              <select v-if="newGroupCourseType === 'language'" v-model="newGroupLevel" class="text-sm">
                <option v-for="lvl in groupLevelOptions" :key="lvl" :value="lvl">{{ lvl }}</option>
              </select>
            </div>
            <button type="submit" :disabled="branchRequired" class="w-full rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-contrast disabled:opacity-60">{{ t("schedule.add") }}</button>
            <p v-if="branchRequired" class="text-xs text-danger">{{ t("branch.required") }}</p>
          </form>
        </div>

        <div class="rounded-xl border border-line bg-surface2 p-4">
          <h3 class="mb-2 font-display text-sm font-semibold text-ink">{{ t("schedule.groupsList") }}</h3>
          <div v-for="g in groups" :key="g.id" class="border-b border-line py-2 text-sm last:border-0">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div>
                <span class="font-semibold text-ink">{{ g.name }}</span>
                <span class="text-ink-muted">
                  · {{ teacherName(g.teacher_id) }} · {{ t(`courseTypes.${g.course_type}`) }}
                  <template v-if="g.subject"> · {{ t(`subjects.${g.subject}`) }}</template>
                  <template v-if="g.level"> · {{ g.level }}</template>
                </span>
              </div>
              <button
                type="button"
                class="font-accent shrink-0 rounded-lg border border-line px-2 py-1 text-xs font-semibold text-ink hover:bg-surface3"
                @click="toggleRoster(g.id)"
              >
                {{ rosterOpenGroupId === g.id ? t("schedule.hideRoster") : t("schedule.manageRoster") }}
              </button>
            </div>

            <div v-if="rosterOpenGroupId === g.id" class="mt-2 rounded-lg border border-line bg-surface1 p-3">
              <div v-for="rs in rosterStudents" :key="rs.id" class="flex items-center justify-between border-b border-line py-1.5 text-xs last:border-0">
                <span class="text-ink">{{ rs.full_name }} <span v-if="rs.level" class="text-ink-muted">· {{ rs.level }}</span></span>
                <button type="button" class="text-danger hover:underline" @click="removeRosterStudent(g.id, rs.id)">{{ t("schedule.removeStudent") }}</button>
              </div>
              <p v-if="!rosterStudents.length" class="text-xs text-ink-muted">{{ t("schedule.noRosterStudents") }}</p>

              <div class="mt-2 flex gap-2">
                <select v-model="rosterAddStudentId" class="flex-1 text-xs">
                  <option value="" disabled>{{ t("schedule.selectStudent") }}</option>
                  <option v-for="s in rosterAvailableStudents" :key="s.id" :value="s.id">{{ s.full_name }}</option>
                </select>
                <button
                  type="button"
                  :disabled="!rosterAddStudentId"
                  class="shrink-0 rounded-lg bg-accent px-2.5 py-1 text-xs font-semibold text-accent-contrast disabled:opacity-60"
                  @click="addRosterStudent(g.id)"
                >
                  {{ t("schedule.addStudent") }}
                </button>
              </div>
            </div>
          </div>
          <p v-if="!groups.length" class="text-sm text-ink-muted">—</p>
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
