package auth

// NewDeviceToken returns a high-entropy bearer token suitable for a Runner.
// The Hub persists only HashToken(token).
func NewDeviceToken() (string, error) {
	return newRawToken(32)
}
