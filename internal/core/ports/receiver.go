// Package ports defines the interfaces (contracts) that the core domain
// relies on. Adapters implement these interfaces to connect the domain
// to external infrastructure. This file defines the Receiver interface,
// which waits for and collects feedback from a notification channel.
package ports
