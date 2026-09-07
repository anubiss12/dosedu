package repository

import (
	"context"
	"time"
)

type Ticket struct {
	ID            string    `json:"id"`
	BranchID      string    `json:"branch_id"`
	Subject       string    `json:"subject"`
	Status        string    `json:"status"`
	CreatedByRole string    `json:"created_by_role"`
	CreatedByID   string    `json:"created_by_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type TicketMessage struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	SenderRole string    `json:"sender_role"`
	SenderID   string    `json:"sender_id"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

type TicketRepo struct{ store *Store }

func NewTicketRepo(s *Store) *TicketRepo { return &TicketRepo{store: s} }

// Create opens a new ticket and inserts its first message in one
// transaction, so a ticket never exists without its opening message.
func (r *TicketRepo) Create(ctx context.Context, branchID, subject, role, senderID, firstMessage string) (string, error) {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var ticketID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tickets (branch_id, subject, created_by_role, created_by_id)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, branchID, subject, role, senderID).Scan(&ticketID); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_messages (ticket_id, sender_role, sender_id, message)
		VALUES ($1, $2, $3, $4)
	`, ticketID, role, senderID, firstMessage); err != nil {
		return "", err
	}

	return ticketID, tx.Commit(ctx)
}

// AddMessage appends a reply and bumps the ticket's updated_at (and
// reopens it to "in_progress" if it had been closed).
func (r *TicketRepo) AddMessage(ctx context.Context, ticketID, role, senderID, message string) error {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_messages (ticket_id, sender_role, sender_id, message)
		VALUES ($1, $2, $3, $4)
	`, ticketID, role, senderID, message); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tickets SET status = 'in_progress', updated_at = now()
		WHERE id = $1 AND status != 'closed'
	`, ticketID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *TicketRepo) SetStatus(ctx context.Context, ticketID, branchID, status string) error {
	tag, err := r.store.Pool.Exec(ctx, `
		UPDATE tickets SET status = $1, updated_at = now()
		WHERE id = $2 AND branch_id = $3
	`, status, ticketID, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByBranch returns every ticket for a branch — the director/teacher
// inbox view.
func (r *TicketRepo) ListByBranch(ctx context.Context, branchID string) ([]Ticket, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, branch_id, subject, status, created_by_role, created_by_id, created_at
		FROM tickets WHERE branch_id = $1 ORDER BY updated_at DESC
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.BranchID, &t.Subject, &t.Status, &t.CreatedByRole, &t.CreatedByID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListMessages returns the full thread for one ticket, oldest first.
func (r *TicketRepo) ListMessages(ctx context.Context, ticketID string) ([]TicketMessage, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, ticket_id, sender_role, sender_id, message, created_at
		FROM ticket_messages WHERE ticket_id = $1 ORDER BY created_at ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TicketMessage
	for rows.Next() {
		var m TicketMessage
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderRole, &m.SenderID, &m.Message, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
