// Package webhook implements the primary webhook adapter for the e2e-testing-service.
// This file implements the Meta extractor, which handles incoming WhatsApp/Messenger
// notification webhooks from Meta, verifies signatures, and extracts the
// relevant fields into a domain.Message.
package webhook
