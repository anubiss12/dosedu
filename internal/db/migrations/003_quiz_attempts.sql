-- dosedu.kz — quiz attempt history (student practice/official test results)

CREATE TABLE quiz_attempts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id  UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    language    VARCHAR(8) NOT NULL,             -- 'en' | 'zh'
    kind        VARCHAR(16) NOT NULL DEFAULT 'practice', -- 'practice' | 'official'
    score       INTEGER NOT NULL,
    total       INTEGER NOT NULL,
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quiz_attempts_student ON quiz_attempts(student_id, taken_at DESC);
