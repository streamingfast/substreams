package block

import (
	"strconv"
	"strings"
)

func ParseRange(in string) *Range {
	if in == "" {
		return nil
	}
	ch := strings.Split(in, "-")
	lo, err := strconv.ParseInt(ch[0], 10, 64)
	if err != nil {
		panic(err)
	}
	hi, err := strconv.ParseInt(ch[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return NewRange(uint64(lo), uint64(hi))
}

func ParseRanges(in string) (out Ranges) {
	for _, e := range strings.Split(in, ",") {
		newRange := ParseRange(strings.Trim(e, " "))
		if newRange != nil {
			out = append(out, newRange)
		}
	}
	return
}
