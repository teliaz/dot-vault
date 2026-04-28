//go:build !darwin || !cgo

package biometric

func newPlatformAuthorizer() sensitiveAuthorizer {
	return nil
}
