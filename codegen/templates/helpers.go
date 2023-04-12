package templates

func keys[K comparable, V any](entries map[K]V) (out []K) {
	if len(entries) == 0 {
		return nil
	}

	out = make([]K, len(entries))
	i := 0
	for k := range entries {
		out[i] = k
		i++
	}

	return
}
