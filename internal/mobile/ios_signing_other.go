//go:build !darwin || !cgo

package mobile

import "errors"

func iosNativeAvailable() bool { return false }
func iosNativeInspect(p12, profile []byte, password string) ([]byte, []byte, []byte, error) {
	return nil, nil, nil, errors.New("ios_signing_unsupported")
}
func iosNativeImport(p12 []byte, password, keychain, keychainPassword string) error {
	return errors.New("ios_signing_unsupported")
}
func iosNativeDelete(keychain string) error { return errors.New("ios_signing_unsupported") }
