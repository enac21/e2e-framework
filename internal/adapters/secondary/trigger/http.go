// Package trigger implements the secondary trigger adapter for the e2e-testing-service.
// This file implements the HTTP trigger, which executes the initial HTTP call
// that starts the notification flow. It resolves template variables in URL,
// headers, and body before making the request.
package trigger
