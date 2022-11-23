package utils

func MinOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func MaxOf(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
