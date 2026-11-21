//go:build !pkcs11 || !cgo

package secrets

import "errors"

// PKCS11Supported reports whether this binary was built with HSM support.
const PKCS11Supported = false

// ErrNoPKCS11 is returned when the pkcs11 provider is selected in a binary
// built without HSM support.
var ErrNoPKCS11 = errors.New("secrets: the pkcs11 (HSM) key provider is not available in this binary: " +
	"PKCS#11 needs cgo, and the default build is static (CGO_ENABLED=0). " +
	"Build with HSM support with `make go-pkcs11` (CGO_ENABLED=1 go build -tags pkcs11 ./cmd/qtestframe), see docs/operations.md")

// PKCS11 is not available in this build.
type PKCS11 struct{ Provider }

// NewPKCS11 returns ErrNoPKCS11.
func NewPKCS11(PKCS11Config) (*PKCS11, error) { return nil, ErrNoPKCS11 }
