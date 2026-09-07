package handlers

import (
	"context"

	"github.com/dosedu/lms/internal/repository"
	"github.com/dosedu/lms/internal/telegram"
)

// ParentAdapter satisfies telegram.ParentLinker and
// telegram.BalanceLookup by delegating to repository.ParentRepo and
// converting between the two packages' local types. This keeps
// internal/telegram free of a dependency on internal/repository.
type ParentAdapter struct {
	repo *repository.ParentRepo
}

func NewParentAdapter(repo *repository.ParentRepo) *ParentAdapter {
	return &ParentAdapter{repo: repo}
}

func (a *ParentAdapter) LinkTelegramChat(ctx context.Context, phone, chatID string) error {
	return a.repo.LinkTelegramChat(ctx, phone, chatID)
}

func (a *ParentAdapter) FindChildrenByChatIDStrings(ctx context.Context, chatID string) ([]telegram.ChildInfo, error) {
	children, err := a.repo.FindChildrenByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	out := make([]telegram.ChildInfo, 0, len(children))
	for _, c := range children {
		out = append(out, telegram.ChildInfo{
			FullName:      c.FullName,
			Balance:       c.Balance,
			PaymentStatus: c.PaymentStatus,
		})
	}
	return out, nil
}
