package ebpf

import _ "embed"
import "C"

//go:embed xdp_router.o
var RouterProgram []byte
