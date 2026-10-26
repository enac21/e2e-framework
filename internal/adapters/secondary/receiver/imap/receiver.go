package imap

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/logger"
)

type IMAPReceiver struct {
	Client  ports.IMAPClient
	RunID   string
	Vars    map[string]string
	Host    string
	Mailbox string
	logger  *logger.Logger
}

func NewIMAPReceiver(options map[string]string) (*IMAPReceiver, error) {
	host, ok := options["host"]
	if !ok || host == "" {
		return nil, fmt.Errorf("%w: imap receiver requires 'host' option", domain.ErrConfiguration)
	}

	portStr, ok := options["port"]
	if !ok || portStr == "" {
		return nil, fmt.Errorf("%w: imap receiver requires 'port' option", domain.ErrConfiguration)
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("%w: imap 'port' option must be a number: %v", domain.ErrConfiguration, err)
	}

	username := options["username"]
	password := options["password"]
	mailbox := options["mailbox"]
	if mailbox == "" {
		mailbox = "INBOX"
	}

	useTLS, _ := strconv.ParseBool(options["tls"])

	client := NewGoIMAPClient(host, portNum, username, password, mailbox, useTLS)

	return &IMAPReceiver{
		Client:  client,
		Host:    host,
		Mailbox: mailbox,
		logger:  logger.New("receiver").With(logger.AttrReceiver, domain.ImapReceiverType),
	}, nil
}

func (r *IMAPReceiver) log() *logger.Logger {
	if r.logger == nil {
		r.logger = logger.New("receiver").With(logger.AttrReceiver, domain.ImapReceiverType)
	}

	return r.logger
}

func (r *IMAPReceiver) Start(ctx context.Context, runID string, vars map[string]string) error {
	if err := r.Client.Connect(); err != nil {
		return fmt.Errorf("%w: failed to connect to IMAP server: %v", domain.ErrInternal, err)
	}

	r.log().Info().
		Str(logger.AttrRunID, runID).
		Str(logger.AttrHost, r.Host).
		Str(logger.AttrMailbox, r.Mailbox).
		Msg("connected to mailbox")

	r.RunID = runID
	r.Vars = vars

	return nil
}

func (r *IMAPReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.RunID == "" {
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log().Warn().
				Str(logger.AttrRunID, r.RunID).
				Str(logger.AttrMailbox, r.Mailbox).
				Err(ctx.Err()).
				Msg("timed out waiting for email")

			return nil, fmt.Errorf("%w: timeout waiting for email with runID %s: %v", domain.ErrTimeout, r.RunID, ctx.Err())
		case <-ticker.C:
			msg, err := r.Client.SearchByRunID(ctx, r.RunID)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to search inbox: %v", domain.ErrInternal, err)
			}

			if msg != nil {
				return msg, nil
			}
		}
	}
}

func (r *IMAPReceiver) Stop() error {
	r.log().Info().
		Str(logger.AttrRunID, r.RunID).
		Str(logger.AttrHost, r.Host).
		Str(logger.AttrMailbox, r.Mailbox).
		Msg("disconnected from mailbox")

	return r.Client.Disconnect()
}
