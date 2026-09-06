package domain

type ReceiverType = string

const (
	RequestReceiverType ReceiverType = "request"
	ImapReceiverType    ReceiverType = "imap"
	APIReceiverType     ReceiverType = "api"
)
