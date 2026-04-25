// Package ports defines the interfaces (contracts) that the core domain
// relies on. Adapters implement these interfaces to connect the domain
// to external infrastructure. This file defines the Trigger interface,
// which executes the initial HTTP call that starts the notification flow.
package ports
