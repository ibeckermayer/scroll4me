package notifier

import (
	"fmt"

	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/digest"
	"github.com/ibeckermayer/scroll4me/internal/notifier/providers"
)

// Notifier handles sending digest notifications
type Notifier struct {
	sender Sender
}

// Sender defines the interface for email sending
type Sender interface {
	Send(to, subject, htmlBody, plainBody string) error
}

// New creates a new notifier with the given sender
func New(sender Sender) *Notifier {
	return &Notifier{sender: sender}
}

// NewFromConfig creates a notifier based on configuration
func NewFromConfig(cfg config.EmailConfig) (*Notifier, error) {
	var sender Sender

	switch cfg.Provider {
	case "smtp":
		sender = providers.NewSMTPSender(
			cfg.SMTPHost,
			cfg.SMTPPort,
			cfg.SMTPUser,
			cfg.SMTPPass,
			cfg.FromAddr,
		)
	// case "sendgrid":
	//     sender = providers.NewSendGridSender(cfg.APIKey, cfg.FromAddr)
	default:
		return nil, fmt.Errorf("unknown email provider: %s", cfg.Provider)
	}

	return New(sender), nil
}

// SendDigest sends a digest email
func (n *Notifier) SendDigest(d *digest.Digest, toAddr string) error {
	return n.sender.Send(toAddr, d.Subject, d.HTMLBody, d.PlainBody)
}
