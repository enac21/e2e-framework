package domain

type ReceiverType = string

const (
	WebhookReceiverType ReceiverType = "webhook"
	ImapReceiverType    ReceiverType = "imap"
	APIReceiverType     ReceiverType = "api"
)
