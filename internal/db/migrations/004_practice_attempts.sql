-- Practice test attempt history — the student's "Нәтижелер" (Results) tab.

CREATE TABLE practice_attempts (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id  UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    language    VARCHAR(8) NOT NULL,  -- 'en' | 'zh'
    score       INTEGER NOT NULL,
    total       INTEGER NOT NULL,
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_practice_attempts_student ON practice_attempts(student_id, taken_at DESC);
