package imap

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type IMAPReceiver struct {
	client ports.IMAPClient
	runID  string
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

	tls, _ := strconv.ParseBool(options["tls"])

	// TEMP USE OF VARS - REMOVE AFTER IMPLEMENTATION
	_ = host
	_ = portNum
	_ = username
	_ = password
	_ = mailbox
	_ = tls

	// TODO: initialize real IMAP client and assign to client field
	// client := imaplib.NewClient(host, portNum, username, password, mailbox, tls)
	return &IMAPReceiver{}, nil
}

func (r *IMAPReceiver) Start(ctx context.Context, runID string) error {
	if err := r.client.Connect(); err != nil {
		return fmt.Errorf("%w: failed to connect to IMAP server: %v", domain.ErrInternal, err)
	}

	r.runID = runID

	return nil
}

func (r *IMAPReceiver) Collect(ctx context.Context) (*domain.Message, error) {
	if r.runID == "" {
		return nil, fmt.Errorf("%w: receiver not started", domain.ErrConfiguration)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: timeout waiting for email with runID %s: %v", domain.ErrTimeout, r.runID, ctx.Err())
		case <-ticker.C:
			msg, err := r.client.SearchByRunID(ctx, r.runID)
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
	return r.client.Disconnect()
}
