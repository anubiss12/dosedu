-- 009_care_timing_extras.sql
--
-- Small additions needed to make the redesigned student/parent UI show
-- real data instead of placeholders:
--   - attendance.qr_checked_out_at: departure stamp, symmetric with the
--     existing qr_checked_in_at, so a care_and_prep child's "3-hour
--     session" can show an actual elapsed/remaining time.
--   - daily_logs.checklist: an optional, flexible list of
--     {label, done} items for a MAD/prodlenka day's report (subjects
--     covered / homework items), read by the parent portal. Nothing
--     writes to it yet (the teacher-side entry UI is a separate,
--     not-yet-requested task) — the parent UI falls back to the
--     existing homework_status badge while it's NULL.

ALTER TABLE attendance ADD COLUMN qr_checked_out_at TIMESTAMPTZ;
ALTER TABLE daily_logs ADD COLUMN checklist JSONB;
