// Package ports defines the interfaces (contracts) that the core domain
// relies on. Adapters implement these interfaces to connect the domain
// to external infrastructure. This file defines the Notifier interface,
// which sends alerts to a configured webhook when a test fails.
package ports
