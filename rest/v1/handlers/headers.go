package handlers

const (
	// XNeofsTokenSignature header contains base64 encoded signature of the token body.
	XNeofsTokenSignature = "X-Neofs-Token-Signature"

	// XNeofsTokenSignatureKey header contains hex encoded public key that corresponds the signature of the token body.
	XNeofsTokenSignatureKey = "X-Neofs-Token-Signature-Key"

	// XNeofsTokenLifetime header contains token lifetime in epoch.
	XNeofsTokenLifetime = "X-Neofs-Token-Lifetime"

	// XNeofsTokenScope header contains operation scope for auth (bearer) token.
	// It corresponds to 'object' or 'container' services in neofs.
	XNeofsTokenScope = "X-Neofs-Token-Scope"
)

type TokenScope string

const (
	UnknownScope   TokenScope = ""
	ObjectScope    TokenScope = "object"
	ContainerScope TokenScope = "container"
)

// Parse provided scope.
// If scope invalid the value will be UnknownScope.
func (s *TokenScope) Parse(scope string) {
	switch scope {
	default:
		*s = UnknownScope
	case string(ObjectScope):
		*s = ObjectScope
	case string(ContainerScope):
		*s = ContainerScope
	}
}
