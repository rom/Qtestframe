package hardening

// Limits are resource limits applied to sandboxed script processes.
type Limits struct {
	CPUSeconds   uint64
	FileSize     uint64
	OpenFiles    uint64
	AddressSpace uint64 // 0 = unlimited
}

// DefaultLimits are used by the script runner sandbox.
var DefaultLimits = Limits{CPUSeconds: 120, FileSize: 64 << 20, OpenFiles: 256}
