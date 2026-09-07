package telegram

import (
	"context"
	"fmt"
	"time"
)

// Notifier wraps Client with the specific message templates the spec
// calls for. Every method is a no-op (returns nil) if the bot isn't
// configured, so callers never need to branch on c.Enabled() themselves.
type Notifier struct {
	client *Client
}

func NewNotifier(client *Client) *Notifier {
	return &Notifier{client: client}
}

// NotifyQRCheckIn pings the parent the moment their child scans in/out
// at the center, per the spec's "QR ескертулер" requirement.
func (n *Notifier) NotifyQRCheckIn(ctx context.Context, parentChatID, studentName string, checkedIn bool, at time.Time) error {
	action := "келді"
	if !checkedIn {
		action = "кетті"
	}
	text := fmt.Sprintf("🔔 *%s* сабаққа/продленкаға %s.\n🕒 %s",
		studentName, action, at.Format("15:04, 02.01.2006"))
	return n.client.SendMessage(ctx, parentChatID, text)
}

// NotifyPaymentConfirmed tells the parent their manual payment was
// registered by staff.
func (n *Notifier) NotifyPaymentConfirmed(ctx context.Context, parentChatID, studentName string, amount float64, periodEnd time.Time) error {
	text := fmt.Sprintf("✅ *%s* үшін төлем расталды.\n💰 Сома: %.0f ₸\n📅 Жарамды: %s дейін",
		studentName, amount, periodEnd.Format("02.01.2006"))
	return n.client.SendMessage(ctx, parentChatID, text)
}

// NotifyPaymentOverdue warns the parent that a subscription has lapsed
// or is about to.
func (n *Notifier) NotifyPaymentOverdue(ctx context.Context, parentChatID, studentName string, expiredAt time.Time) error {
	text := fmt.Sprintf("⚠️ *%s* абонементінің мерзімі %s аяқталды. Материалдарға қолжетімділік шектелуі мүмкін — төлемді жаңартыңызшы.",
		studentName, expiredAt.Format("02.01.2006"))
	return n.client.SendMessage(ctx, parentChatID, text)
}

// NotifyBalance answers the /balance command with the student's current
// standing.
func (n *Notifier) NotifyBalance(ctx context.Context, parentChatID, studentName string, balance float64, paymentStatus string) error {
	text := fmt.Sprintf("💳 *%s* балансы: %.0f ₸\nСтатус: %s", studentName, balance, paymentStatus)
	return n.client.SendMessage(ctx, parentChatID, text)
}

// SendTemporaryPassword delivers a freshly generated password (e.g. a
// director account created by the super admin) over Telegram instead of
// plain email, per the spec's "Уақытша парольдер" requirement.
func (n *Notifier) SendTemporaryPassword(ctx context.Context, chatID, roleLabel, identifier, tempPassword string) error {
	text := fmt.Sprintf("🔐 %s есептік жазбаңыз құрылды.\nЛогин: `%s`\nУақытша құпия сөз: `%s`\n\nАлғаш кіргенде құпия сөзді ауыстыруды ұмытпаңыз.",
		roleLabel, identifier, tempPassword)
	return n.client.SendMessage(ctx, chatID, text)
}

// SendMonthlyPDFReport delivers the one-tap monthly progress PDF
// directly to the parent's chat.
func (n *Notifier) SendMonthlyPDFReport(ctx context.Context, parentChatID, studentName string, pdfBytes []byte, month string) error {
	filename := fmt.Sprintf("%s_%s.pdf", studentName, month)
	caption := fmt.Sprintf("📄 %s — %s айының үлгерім есебі", studentName, month)
	return n.client.SendDocument(ctx, parentChatID, filename, pdfBytes, caption)
}
