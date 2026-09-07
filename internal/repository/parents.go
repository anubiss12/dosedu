package repository

import (
	"context"
)

type ParentRepo struct{ store *Store }

func NewParentRepo(s *Store) *ParentRepo { return &ParentRepo{store: s} }

// LinkTelegramChat associates a parent's phone number with the Telegram
// chat_id captured during the bot's /start <phone> linking flow.
func (r *ParentRepo) LinkTelegramChat(ctx context.Context, phone, chatID string) error {
	tag, err := r.store.Pool.Exec(ctx, `
		UPDATE parents SET telegram_chat_id = $1 WHERE phone = $2
	`, chatID, phone)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ChildSummary is what the bot needs to answer /balance for a linked
// parent — potentially more than one child per parent.
type ChildSummary struct {
	StudentID     string
	FullName      string
	Balance       float64
	PaymentStatus string
}

// FindChildrenByChatID returns every student linked to the parent who
// owns this Telegram chat — used by the /balance bot command.
func (r *ParentRepo) FindChildrenByChatID(ctx context.Context, chatID string) ([]ChildSummary, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT s.id, s.full_name, s.balance, s.payment_status
		FROM students s
		JOIN parents p ON p.id = s.parent_id
		WHERE p.telegram_chat_id = $1
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChildSummary
	for rows.Next() {
		var c ChildSummary
		if err := rows.Scan(&c.StudentID, &c.FullName, &c.Balance, &c.PaymentStatus); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindParentChatIDByStudent is used by the notification flows (QR
// check-in, payment confirmation) to find where to send the alert.
func (r *ParentRepo) FindParentChatIDByStudent(ctx context.Context, studentID string) (string, error) {
	var chatID *string
	err := r.store.Pool.QueryRow(ctx, `
		SELECT p.telegram_chat_id
		FROM students s
		JOIN parents p ON p.id = s.parent_id
		WHERE s.id = $1
	`, studentID).Scan(&chatID)
	if err != nil {
		return "", err
	}
	if chatID == nil {
		return "", ErrNotFound // parent hasn't linked Telegram yet
	}
	return *chatID, nil
}
