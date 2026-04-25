// Package webhook implements the primary webhook adapter for the e2e-testing-service.
// This file is responsible for setting up the webhook HTTP server that receives
// incoming notifications from external providers (Twilio, Meta, etc.) and routes
// them to the appropriate extractor for processing.
package webhook
