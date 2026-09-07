-- Phase 2: language scoping for students/teachers + a real question bank
-- so practice/official level tests have actual content instead of being
-- frontend-hardcoded. PostgreSQL 15+.

-- 'en' | 'zh' | NULL (NULL = prodlenka/mad student, no language track)
ALTER TABLE students ADD COLUMN language VARCHAR(8) CHECK (language IN ('en', 'zh'));

-- 'en' | 'zh' | NULL (NULL = mad/prodlenka teacher, no test-upload access —
-- enforced starting Phase 3, the column just exists from here on)
ALTER TABLE teachers ADD COLUMN language_scope VARCHAR(8) CHECK (language_scope IN ('en', 'zh'));

-- ============================================================
-- QUESTION BANK (practice = unlimited retakes, official = one-time
-- level-advancement test a teacher marks as such on upload — see
-- internal/handlers/test_upload.go, wired for real in Phase 3)
-- ============================================================
CREATE TABLE test_questions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    branch_id       UUID REFERENCES branches(id) ON DELETE SET NULL,
    created_by      UUID REFERENCES teachers(id) ON DELETE SET NULL,
    language        VARCHAR(8) NOT NULL CHECK (language IN ('en', 'zh')),
    level           VARCHAR(64) NOT NULL,
    is_official     BOOLEAN NOT NULL DEFAULT false,
    question        TEXT NOT NULL,
    option_a        TEXT NOT NULL,
    option_b        TEXT NOT NULL,
    option_c        TEXT,
    option_d        TEXT,
    correct_option  SMALLINT NOT NULL CHECK (correct_option BETWEEN 0 AND 3),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_test_questions_lookup ON test_questions(language, level, is_official);

-- Seed content so the student practice/official flow is testable before
-- any teacher has uploaded real tests (Phase 3). branch_id/created_by
-- are left NULL — a global starter bank, not tied to one branch.
INSERT INTO test_questions (language, level, is_official, question, option_a, option_b, option_c, option_d, correct_option) VALUES
('en', 'Beginner', false, 'She ___ to school every day.', 'go', 'goes', 'going', 'gone', 1),
('en', 'Beginner', false, 'What is the plural of ''child''?', 'childs', 'children', 'childes', 'child', 1),
('en', 'Beginner', false, 'Choose the opposite of ''happy''.', 'sad', 'glad', 'fast', 'big', 0),
('en', 'Beginner', false, 'I ___ a book right now.', 'read', 'reads', 'am reading', 'readed', 2),
('en', 'Beginner', false, 'They ___ from Kazakhstan.', 'is', 'am', 'are', 'be', 2),
('en', 'Beginner', true, 'She ___ to school every day.', 'go', 'goes', 'going', 'gone', 1),
('en', 'Beginner', true, 'What is the plural of ''mouse''?', 'mouses', 'mice', 'mouse', 'mices', 1),
('en', 'Beginner', true, 'Choose the correct sentence.', 'He don''t like tea.', 'He doesn''t likes tea.', 'He doesn''t like tea.', 'He not like tea.', 2),
('en', 'Beginner', true, 'What is the past tense of ''go''?', 'goed', 'went', 'gone', 'going', 1),
('en', 'Beginner', true, 'Choose the opposite of ''big''.', 'small', 'tall', 'wide', 'long', 0),
('zh', 'HSK1', false, '"你好" (nǐ hǎo) мағынасы?', 'Сау бол', 'Сәлем', 'Рахмет', 'Кешіріңіз', 1),
('zh', 'HSK1', false, '"谢谢" (xièxiè) мағынасы?', 'Иә', 'Жоқ', 'Рахмет', 'Сәлем', 2),
('zh', 'HSK1', false, '"再见" (zàijiàn) мағынасы?', 'Сау бол', 'Рахмет', 'Сәлем', 'Кешіріңіз', 0),
('zh', 'HSK1', false, '"我" (wǒ) мағынасы?', 'Сен', 'Ол', 'Мен', 'Біз', 2),
('zh', 'HSK1', false, '"水" (shuǐ) мағынасы?', 'От', 'Су', 'Жер', 'Ауа', 1),
('zh', 'HSK1', true, '"你好" (nǐ hǎo) мағынасы?', 'Сау бол', 'Сәлем', 'Рахмет', 'Кешіріңіз', 1),
('zh', 'HSK1', true, '"谢谢" (xièxiè) мағынасы?', 'Иә', 'Жоқ', 'Рахмет', 'Сәлем', 2),
('zh', 'HSK1', true, '"多少钱" (duōshǎo qián) мағынасы?', 'Қалайсың?', 'Бағасы қанша?', 'Қайда?', 'Не бұл?', 1),
('zh', 'HSK1', true, '"今天" (jīntiān) мағынасы?', 'Ертең', 'Кеше', 'Бүгін', 'Қазір', 2),
('zh', 'HSK1', true, '"朋友" (péngyǒu) мағынасы?', 'Отбасы', 'Дос', 'Мұғалім', 'Дәрігер', 1);
