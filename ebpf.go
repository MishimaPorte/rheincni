package rheincni

// opaque EBPF map
type EbpfMap struct{}

func (e *EbpfMap) Insert(key []byte, value []byte) error {
	return nil
}
