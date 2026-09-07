-- dosedu.kz — миграция 002: сертификаттар + ішкі ticketing модулі

-- ============================================================
-- CERTIFICATES (курс/деңгей аяқталғанда автоматты PDF + QR)
-- ============================================================
CREATE TABLE certificates (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    student_id      UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    branch_id       UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    level           VARCHAR(64) NOT NULL,
    issued_by       UUID, -- teacher_id
    qr_token        UUID NOT NULL UNIQUE DEFAULT uuid_generate_v4(), -- QR public verify token
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_certificates_student ON certificates(student_id);

-- ============================================================
-- TICKETING (мұғалім / директор / ата-ана арасындағы өтініштер)
-- ============================================================
CREATE TYPE ticket_status AS ENUM ('open', 'in_progress', 'closed');
CREATE TYPE ticket_sender_role AS ENUM ('teacher', 'director', 'parent', 'student', 'super_admin');

CREATE TABLE tickets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    branch_id       UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    subject         VARCHAR(255) NOT NULL,
    status          ticket_status NOT NULL DEFAULT 'open',
    created_by_role ticket_sender_role NOT NULL,
    created_by_id   UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tickets_branch_status ON tickets(branch_id, status);

CREATE TABLE ticket_messages (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ticket_id       UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    sender_role     ticket_sender_role NOT NULL,
    sender_id       UUID NOT NULL,
    message         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_messages_ticket ON ticket_messages(ticket_id, created_at);

CREATE TRIGGER trg_tickets_updated BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
