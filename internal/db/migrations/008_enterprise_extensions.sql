-- 008_enterprise_extensions.sql
--
-- Adds the schema pieces requested for the "enterprise" backend spec
-- that have a real, immediate use in this phase:
--   - students.qr_token: a stable per-student identifier for a future
--     QR-code-based check-in flow (today's /teacher/attendance/qr-checkin
--     still takes a plain student_id; this column reserves the column
--     so that flow can be upgraded to scan-a-token without another
--     migration).
--   - churn_alerts: the table a future absence-streak Cron job will
--     write to and GET /director/churn-alerts reads from. Created now,
--     left unpopulated — no writer exists yet, deliberately (see plan).
--   - branches.deleted_at: soft-delete for branches, distinct from the
--     existing `status` column (status = temporarily blocked/active;
--     deleted_at = removed from every listing, history preserved).

ALTER TABLE students ADD COLUMN qr_token UUID NOT NULL DEFAULT uuid_generate_v4() UNIQUE;

CREATE TYPE churn_alert_type AS ENUM ('language_absent_streak', 'care_absent_streak');

CREATE TABLE churn_alerts (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    branch_id      UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    student_id     UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    alert_type     churn_alert_type NOT NULL,
    streak_count   INT NOT NULL,
    acknowledged   BOOLEAN NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_churn_alerts_branch ON churn_alerts(branch_id, acknowledged);

ALTER TABLE branches ADD COLUMN deleted_at TIMESTAMPTZ;
