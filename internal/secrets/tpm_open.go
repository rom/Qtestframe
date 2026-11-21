//go:build !windows

package secrets

import (
	"os"

	"github.com/google/go-tpm/tpm2/transport"
	"github.com/google/go-tpm/tpm2/transport/linuxtpm"
	"github.com/google/go-tpm/tpm2/transport/linuxudstpm"
)

// openTPMDevice opens a TPM character device (/dev/tpmrm0) or the Unix
// socket of a TPM emulator (swtpm socket --server type=unixio,path=…).
func openTPMDevice(path string) (transport.TPMCloser, error) {
	if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
		return linuxudstpm.Open(path)
	}
	return linuxtpm.Open(path)
}
