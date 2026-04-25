// Package store implements the secondary store adapter for the e2e-testing-service.
// This file implements the RedisStore, which provides a Redis-backed temporary buffer
// with TTL for received messages. It handles message deposit/claim operations and
// recipient reservation/release for correlation.
package store
