-- Phase 4: payment method, so the director's Payments view can show
-- how a payment was made (matches the reference UI's "Тәсіл" column).
ALTER TABLE payments ADD COLUMN method VARCHAR(16) NOT NULL DEFAULT 'cash' CHECK (method IN ('cash', 'transfer'));
