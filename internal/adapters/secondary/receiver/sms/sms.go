// Package sms implements the SMS receiver adapter for the e2e-testing-service.
// This file implements the SmsReceiver, which polls the store for messages
// deposited by the Twilio webhook handler and normalizes them into the
// domain.Message format with fields: from, to, body.
package sms
