// Package ports defines the interfaces (contracts) that the core domain
// relies on. Adapters implement these interfaces to connect the domain
// to external infrastructure. This file defines the Store interface,
// a temporary buffer (Redis-backed) for received messages with TTL support.
package ports
