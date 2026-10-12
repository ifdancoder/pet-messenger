// Package application is the conversation context's Use Cases layer
// (Clean Architecture) / Application Core (Hexagonal): it defines the
// inbound and outbound ports (port/in, port/out) and implements the
// inbound ones as CQRS command and query handlers (command/, query/).
package application

// Page-size bounds shared by every list query in this context.
const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// EffectiveListLimit clamps a client-requested page size: non-positive
// means "use the default"; anything above the max is capped.
func EffectiveListLimit(requested int) int {
	switch {
	case requested <= 0:
		return DefaultListLimit
	case requested > MaxListLimit:
		return MaxListLimit
	default:
		return requested
	}
}
