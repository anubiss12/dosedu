package telegram

import (
	"context"
	"fmt"
	"strings"
)

// Update is the minimal subset of the Telegram Bot API's Update object
// this integration needs.
type Update struct {
	Message *IncomingMessage `json:"message"`
}

type IncomingMessage struct {
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text string `json:"text"`
}

// ParentLinker and BalanceLookup are the two repository operations the
// webhook needs. Defined as interfaces here so this package doesn't
// import the repository package directly (keeps the dependency graph
// one-directional: repository <- telegram, not the other way around).
type ParentLinker interface {
	LinkTelegramChat(ctx context.Context, phone, chatID string) error
}

type ChildInfo struct {
	FullName      string
	Balance       float64
	PaymentStatus string
}

type BalanceLookup interface {
	FindChildrenByChatIDStrings(ctx context.Context, chatID string) ([]ChildInfo, error)
}

// WebhookHandler processes incoming bot updates. Wire it behind
// POST /public/telegram/webhook (Telegram calls this URL directly, so
// it must stay unauthenticated — protect it with a secret path segment
// or Telegram's X-Telegram-Bot-Api-Secret-Token header in production).
type WebhookHandler struct {
	client *Client
	linker ParentLinker
	lookup BalanceLookup
}

func NewWebhookHandler(client *Client, linker ParentLinker, lookup BalanceLookup) *WebhookHandler {
	return &WebhookHandler{client: client, linker: linker, lookup: lookup}
}

// Handle processes one parsed Update. Call this from the Gin handler
// after c.ShouldBindJSON(&update).
func (h *WebhookHandler) Handle(ctx context.Context, u Update) {
	if u.Message == nil {
		return
	}
	chatID := fmt.Sprintf("%d", u.Message.Chat.ID)
	text := strings.TrimSpace(u.Message.Text)

	switch {
	case strings.HasPrefix(text, "/start"):
		h.handleStart(ctx, chatID, text)
	case text == "/balance":
		h.handleBalance(ctx, chatID)
	default:
		_ = h.client.SendMessage(ctx, chatID,
			"Сәлем! Балаңыздың балансын білу үшін /balance деп жазыңыз.")
	}
}

// handleStart expects "/start <телефон нөмірі>" — the parent taps a
// deep link from the parent cabinet (dosedu.kz) that pre-fills this,
// linking their Telegram chat to their account.
func (h *WebhookHandler) handleStart(ctx context.Context, chatID, text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		_ = h.client.SendMessage(ctx, chatID,
			"Аккаунтыңызды байланыстыру үшін ата-ана кабинетіндегі Telegram сілтемесін пайдаланыңыз.")
		return
	}
	phone := parts[1]

	if err := h.linker.LinkTelegramChat(ctx, phone, chatID); err != nil {
		_ = h.client.SendMessage(ctx, chatID,
			"Нөмір табылмады. Телефон нөміріңіздің жүйеде тіркелгенін тексеріңіз.")
		return
	}
	_ = h.client.SendMessage(ctx, chatID,
		"✅ Аккаунт сәтті байланыстырылды! Енді сіз /balance арқылы балансты тексере аласыз.")
}

func (h *WebhookHandler) handleBalance(ctx context.Context, chatID string) {
	children, err := h.lookup.FindChildrenByChatIDStrings(ctx, chatID)
	if err != nil || len(children) == 0 {
		_ = h.client.SendMessage(ctx, chatID,
			"Балаңыз табылмады. Алдымен /start <телефон нөмірі> арқылы аккаунтыңызды байланыстырыңыз.")
		return
	}

	var sb strings.Builder
	for _, c := range children {
		sb.WriteString(fmt.Sprintf("💳 *%s*\nБаланс: %.0f ₸\nСтатус: %s\n\n",
			c.FullName, c.Balance, c.PaymentStatus))
	}
	_ = h.client.SendMessage(ctx, chatID, sb.String())
}
