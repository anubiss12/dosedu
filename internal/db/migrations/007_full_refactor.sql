-- dosedu.kz — full backend refactor per updated TZ.
-- Course-type split (LANGUAGE vs CARE_AND_PREP), placement/practice/
-- official test model, MAD/prodlenka children without their own
-- login, multi-branch director scope, impersonation audit trail.
--
-- This replaces the language/practice/quiz model added in migrations
-- 005-006 — those tables carried only this session's own test/seed
-- data, so a clean cut is safe. Certificates, payments, attendance
-- and auth/password policy are untouched per the agreed scope.

-- ============================================================
-- Enums
-- ============================================================
CREATE TYPE course_type AS ENUM ('language', 'care_and_prep');
CREATE TYPE course_subject AS ENUM ('english', 'chinese', 'mad', 'prodlenka');
CREATE TYPE cefr_hsk_level AS ENUM (
  'A1', 'A2', 'B1', 'B2', 'C1', 'C2',            -- language: english
  'HSK1', 'HSK2', 'HSK3', 'HSK4', 'HSK5', 'HSK6' -- language: chinese
);
CREATE TYPE question_pool AS ENUM ('practice', 'official');
CREATE TYPE test_kind AS ENUM ('placement', 'practice', 'official');
CREATE TYPE homework_status AS ENUM ('done', 'not_done', 'partial', 'n_a');

-- ============================================================
-- Directors: single-branch OR whole-network scope
-- ============================================================
ALTER TABLE directors ADD COLUMN is_network_owner BOOLEAN NOT NULL DEFAULT false;

-- ============================================================
-- Leads: carry which subject a placement test was for
-- ============================================================
ALTER TABLE leads ADD COLUMN subject course_subject;

-- ============================================================
-- Groups: course_type/subject replace the old free-text program/level
-- ============================================================
ALTER TABLE groups ALTER COLUMN program SET DEFAULT 'language_course';
ALTER TABLE groups
  ADD COLUMN course_type course_type NOT NULL DEFAULT 'language',
  ADD COLUMN subject course_subject;
ALTER TABLE groups ALTER COLUMN level TYPE cefr_hsk_level USING NULL;

-- ============================================================
-- Students: course_type/subject; MAD/prodlenka children get no
-- login of their own (only their parent account exists) — the old
-- `language` column (migration 005) is superseded by `subject`.
-- ============================================================
ALTER TABLE students ALTER COLUMN program SET DEFAULT 'language_course';
ALTER TABLE students
  ALTER COLUMN login_code DROP NOT NULL,
  ALTER COLUMN password_hash DROP NOT NULL,
  ADD COLUMN course_type course_type NOT NULL DEFAULT 'language',
  ADD COLUMN subject course_subject,
  DROP COLUMN language;
ALTER TABLE students ALTER COLUMN level TYPE cefr_hsk_level USING NULL;

-- ============================================================
-- Teachers: subject becomes the single source of truth for both
-- display ("English"/"MAD") and access scoping — replaces the old
-- free-text `subject` VARCHAR and the `language_scope` column
-- (migration 005). NULL subject = no course assigned yet.
-- ============================================================
ALTER TABLE teachers DROP COLUMN language_scope;
ALTER TABLE teachers ALTER COLUMN subject TYPE course_subject USING NULL;

-- ============================================================
-- Question bank (replaces test_questions from migration 005)
-- ============================================================
DROP TABLE IF EXISTS quiz_attempts;
DROP TABLE IF EXISTS practice_attempts;
DROP TABLE IF EXISTS test_questions;

CREATE TABLE questions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    branch_id       UUID REFERENCES branches(id) ON DELETE SET NULL,
    created_by      UUID REFERENCES teachers(id) ON DELETE SET NULL,
    subject         course_subject NOT NULL CHECK (subject IN ('english', 'chinese')),
    level           cefr_hsk_level NOT NULL,
    pool            question_pool NOT NULL DEFAULT 'practice',
    question        TEXT NOT NULL,
    option_a        TEXT NOT NULL,
    option_b        TEXT NOT NULL,
    option_c        TEXT,
    option_d        TEXT,
    correct_option  SMALLINT NOT NULL CHECK (correct_option BETWEEN 0 AND 3),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_questions_lookup ON questions(subject, level, pool);

-- ============================================================
-- Official test assignment: a teacher assigns a one-time test to a
-- group; students in that group get exactly `attempts_allowed` tries
-- (enforced by uq_official_one_attempt below, for the default of 1).
-- ============================================================
CREATE TABLE test_assignments (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    branch_id         UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    group_id          UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_by        UUID NOT NULL REFERENCES teachers(id),
    subject           course_subject NOT NULL,
    level             cefr_hsk_level NOT NULL,
    attempts_allowed  SMALLINT NOT NULL DEFAULT 1,
    question_ids      UUID[] NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Test results: unified placement/practice/official attempt log
-- ============================================================
CREATE TABLE test_results (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    kind                test_kind NOT NULL,
    student_id          UUID REFERENCES students(id) ON DELETE CASCADE, -- NULL for placement
    lead_id             UUID REFERENCES leads(id) ON DELETE SET NULL,   -- placement only
    test_assignment_id  UUID REFERENCES test_assignments(id) ON DELETE SET NULL, -- official only
    subject             course_subject NOT NULL,
    level               cefr_hsk_level, -- NULL for placement (that's what it determines)
    score               INT NOT NULL,
    total               INT NOT NULL,
    taken_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_test_results_student ON test_results(student_id, kind);
CREATE UNIQUE INDEX uq_official_one_attempt
  ON test_results(test_assignment_id, student_id) WHERE kind = 'official';

-- ============================================================
-- CARE_AND_PREP daily log: attendance + teacher note + homework,
-- one row per student per day — what the parent's dashboard reads.
-- ============================================================
CREATE TABLE daily_logs (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id          UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    log_date            DATE NOT NULL,
    attendance_status   attendance_status NOT NULL,
    teacher_note        TEXT,
    homework_status     homework_status NOT NULL DEFAULT 'n_a',
    created_by          UUID NOT NULL REFERENCES teachers(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_daily_log_student_day ON daily_logs(student_id, log_date);

-- ============================================================
-- Impersonation audit: super-admin-only, never surfaced to the
-- impersonated user or to director/teacher/parent system logs.
-- ============================================================
CREATE TABLE impersonation_log (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    super_admin_id  UUID NOT NULL REFERENCES super_admins(id),
    target_role     VARCHAR(32) NOT NULL,
    target_id       UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
