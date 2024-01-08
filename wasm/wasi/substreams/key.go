package substreams

import "strings"

type Key string

func (k Key) String() string {
	return string(k)
}

func (k Key) Segments() []string {
	return strings.Split(k.String(), ":")
}

func (k Key) SegmentAt(index int) string {
	return k.Segments()[index] //panics if out of bounds
}

func SegmentAt(key string, index int) string {
	return Key(key).SegmentAt(index)
}

func FirstSegment(key string) string {
	return SegmentAt(key, 0)
}

func LastSegment(key string) string {
	return SegmentAt(key, len(Key(key).Segments())-1)
}
