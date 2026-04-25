// Package push implements the push notification receiver adapter for the e2e-testing-service.
// This file implements the PushReceiver, which handles push notification verification.
// The internal delivery strategy is TBD and will be decided separately.
// Normalizes messages into the domain.Message format with fields: title, body, data.
package push
