package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	imaplib "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/errorwrapper"
)

type GoIMAPClient struct {
	host     string
	port     int
	username string
	password string
	mailbox  string
	useTLS   bool
	conn     *imapclient.Client
}

func NewGoIMAPClient(host string, port int, username, password, mailbox string, useTLS bool) *GoIMAPClient {
	return &GoIMAPClient{
		host:     host,
		port:     port,
		username: username,
		password: password,
		mailbox:  mailbox,
		useTLS:   useTLS,
	}
}

func (c *GoIMAPClient) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	var (
		conn *imapclient.Client
		err  error
	)

	if c.useTLS {
		tlsCfg := &tls.Config{ServerName: c.host}
		conn, err = imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: tlsCfg})
	} else {
		rawConn, dialErr := net.Dial("tcp", addr)
		if dialErr != nil {
			return errorwrapper.Wrap(domain.ErrInternal, dialErr)
		}

		conn = imapclient.New(rawConn, nil)
	}

	if err != nil {
		return errorwrapper.Wrap(domain.ErrInternal, err)
	}

	if err = conn.Login(c.username, c.password).Wait(); err != nil {
		return errorwrapper.Wrap(domain.ErrInternal, err)
	}

	if _, err = conn.Select(c.mailbox, nil).Wait(); err != nil {
		return errorwrapper.Wrap(domain.ErrInternal, err)
	}

	c.conn = conn

	return nil
}

func (c *GoIMAPClient) SearchByRunID(ctx context.Context, runID string) (*domain.Message, error) {
	if err := c.conn.Noop().Wait(); err != nil {
		return nil, errorwrapper.Wrap(domain.ErrInternal, err)
	}

	msg, err := c.searchIn(ctx, runID, searchBySubject)
	if err != nil {
		return nil, err
	}

	if msg != nil {
		return msg, nil
	}

	return c.searchIn(ctx, runID, searchByBody)
}

func (c *GoIMAPClient) Disconnect() error {
	if c.conn == nil {
		return nil
	}

	return c.conn.Logout().Wait()
}

type searchCriteria func(runID string) *imaplib.SearchCriteria

func searchBySubject(runID string) *imaplib.SearchCriteria {
	return &imaplib.SearchCriteria{Header: []imaplib.SearchCriteriaHeaderField{{Key: "Subject", Value: runID}}}
}

func searchByBody(runID string) *imaplib.SearchCriteria {
	return &imaplib.SearchCriteria{Body: []string{runID}}
}

func (c *GoIMAPClient) searchIn(_ context.Context, runID string, criteria searchCriteria) (*domain.Message, error) {
	searchData, err := c.conn.Search(criteria(runID), nil).Wait()
	if err != nil {
		return nil, errorwrapper.Wrap(domain.ErrInternal, err)
	}

	seqNums := searchData.AllSeqNums()
	if len(seqNums) == 0 {
		return nil, nil
	}

	seqSet := imaplib.SeqSetNum(seqNums[0])

	fetchOptions := &imaplib.FetchOptions{
		BodySection: []*imaplib.FetchItemBodySection{{}},
	}

	messages, err := c.conn.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		return nil, errorwrapper.Wrap(domain.ErrInternal, err)
	}

	if len(messages) == 0 || len(messages[0].BodySection) == 0 {
		return nil, nil
	}

	return ParseMessage(runID, messages[0].BodySection[0].Bytes)
}
