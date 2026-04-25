// Package notifier implements the secondary notifier adapter for the e2e-testing-service.
// This file implements the WebhookNotifier, which sends failure alerts to a configured
// webhook URL. It resolves template variables (run_id, test_id, error, etc.) in the
// URL, headers, and body before making the HTTP call.
package notifier
