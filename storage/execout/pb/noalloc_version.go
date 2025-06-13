package pboutputcache

import (
	fmt "fmt"
)

// Faster marshalling through conversion to Array
func (m *Map) MarshalFast() (dAtA []byte, err error) {
	s := &Array{
		Items: make([]*Item, len(m.Kv)),
	}
	i := 0
	for _, item := range m.Kv {
		s.Items[i] = item
		i++
	}
	return s.MarshalVT()
}

// Faster unmarshalling when it was converted to array first
func (m *Map) UnmarshalFast(dAtA []byte) error {
	o := &Array{}
	if err := o.UnmarshalVTUnsafe(dAtA); err != nil {
		return fmt.Errorf("unmarshalling data: %w", err)
	}

	m.Kv = make(map[string]*Item)
	for _, item := range o.Items {
		m.Kv[item.BlockId] = item
	}
	return nil
}
