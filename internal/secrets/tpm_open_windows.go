//go:build windows

package secrets

import (
	"errors"

	"github.com/google/go-tpm/tpm2/transport"
)

func openTPMDevice(string) (transport.TPMCloser, error) {
	return nil, errors.New("the tpm key provider is supported on Linux only")
}
