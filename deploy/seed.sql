-- dosedu.kz — ЛОКАЛ ТЕСТІЛЕУ ҮШІН seed деректері
-- Барлық пароль хэштері бұрыннан bcrypt арқылы есептелген (төмендегі
-- кестедегі ашық парольдерге сәйкес келеді).

-- 1) Филиал
INSERT INTO branches (id, name, address, status)
VALUES ('11111111-1111-1111-1111-111111111111', 'Тест филиалы — Алматы', 'Алматы, Абай даңғылы 1', 'active')
ON CONFLICT (id) DO NOTHING;

-- 2) Супер Админ — email: superadmin@dosedu.kz / пароль: SuprAdm1n!
INSERT INTO super_admins (email, password_hash, full_name)
VALUES ('superadmin@dosedu.kz',
        '$2b$10$OgVUJQngtpR2O2bQhIURb.qLrNaKTrUN2d/qD3YGFH8CYIYXff.fa',
        'Супер Админ')
ON CONFLICT (email) DO NOTHING;

-- 3) Директор — email: director@dosedu.kz / пароль: Dir3ct!1
INSERT INTO directors (branch_id, email, password_hash, full_name)
VALUES ('11111111-1111-1111-1111-111111111111',
        'director@dosedu.kz',
        '$2b$10$OjMwCLEkXh/bDhetvBMs..wJXlQUjyMJLukoxjkl928iNYf87Ih8m',
        'Тест Директор')
ON CONFLICT (email) DO NOTHING;

-- 4) Мұғалім — email: teacher@dosedu.kz / пароль: Teach3r
INSERT INTO teachers (branch_id, email, password_hash, full_name, subject)
VALUES ('11111111-1111-1111-1111-111111111111',
        'teacher@dosedu.kz',
        '$2b$10$OZyWiF4V3ztJxcQkk166o.4MPWR.LGcFS4Tm.F4RtwFxNrlfM8Dw2',
        'Тест Мұғалім',
        'English')
ON CONFLICT (email) DO NOTHING;

-- 5) Ата-ана — телефон: +77011234567 / пароль (4 цифр PIN): 1234
INSERT INTO parents (id, phone, password_hash, full_name)
VALUES ('22222222-2222-2222-2222-222222222222',
        '+77011234567',
        '$2b$10$TAKGVILetZWuPL2xAkDoKOjaquE.HRcd.HkxzT3KIyeSMNem5rC2S',
        'Тест Ата-ана')
ON CONFLICT (phone) DO NOTHING;

-- 6) Оқушы — логин: student001 / пароль (4 цифр PIN): 5678
INSERT INTO students (branch_id, parent_id, login_code, password_hash, full_name, program, level, balance, payment_status)
VALUES ('11111111-1111-1111-1111-111111111111',
        '22222222-2222-2222-2222-222222222222',
        'student001',
        '$2b$10$aPZ29v3iQOLJR.pU8ZLgou/8kfLCi1R8Wtth0YwJS0njTGm81oZGy',
        'Тест Оқушы',
        'language_course',
        'Beginner',
        15000,
        'paid')
ON CONFLICT (login_code) DO NOTHING;

-- Тексеру: барлық жазбалар енгізілгенін растау
SELECT 'branches' AS tbl, count(*) FROM branches
UNION ALL SELECT 'super_admins', count(*) FROM super_admins
UNION ALL SELECT 'directors', count(*) FROM directors
UNION ALL SELECT 'teachers', count(*) FROM teachers
UNION ALL SELECT 'parents', count(*) FROM parents
UNION ALL SELECT 'students', count(*) FROM students;
