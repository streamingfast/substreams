package request

import (
	"fmt"
	"regexp"
)

var positiveNegativeNumbers = regexp.MustCompile(`^[\-]?\d+$`)

func validateNumbersOnly(in string) error {
	if in == "" {
		return nil
	}
	if !positiveNegativeNumbers.MatchString(in) {
		return fmt.Errorf("only numbers allowed (positive and negative)")
	}
	return nil
}

var relativeNumbers = regexp.MustCompile(`^[\+\-]?\d+$`)

func validateNumberOrRelativeValue(in string) error {
	if !relativeNumbers.MatchString(in) {
		return fmt.Errorf("only numbers allowed, optionally prefixed by - or +")
	}
	return nil
}
