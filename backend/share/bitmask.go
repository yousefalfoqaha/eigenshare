package share

type Bitset []byte

func (b Bitset) Set(index int) {
	b[index/8] |= (1 << (index % 8))
}

func (b Bitset) IsSet(index int) bool {
	if index < 0 || index/8 >= len(b) {
		return false
	}

	return b[index/8]&(1<<(index%8)) != 0
}
