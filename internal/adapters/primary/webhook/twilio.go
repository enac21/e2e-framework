// Package webhook implements the primary webhook adapter for the e2e-testing-service.
// This file implements the Twilio extractor, which handles incoming SMS/voice
// notification webhooks from Twilio, verifies signatures, and extracts the
// relevant fields into a domain.Message.
package webhook
