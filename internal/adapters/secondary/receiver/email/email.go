// Package email implements the email receiver adapter for the e2e-testing-service.
// This file implements the EmailReceiver, which connects to an IMAP server to
// poll for incoming email notifications, extracts the run_id from message content
// for correlation, and normalizes messages into the domain.Message format.
package email
