// Package webhook implements the primary webhook adapter for the e2e-testing-service.
// This file defines the Extractor interface, which each webhook provider must
// implement to extract and normalize notification data from incoming payloads
// into a domain.Message.
package webhook
