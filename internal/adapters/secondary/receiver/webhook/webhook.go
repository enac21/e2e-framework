// Package webhook implements the generic webhook receiver adapter for the e2e-testing-service.
// This file implements the WebhookReceiver, which polls the store for messages
// deposited by the webhook server, with configurable field extraction from the
// test YAML definition.
package webhook
