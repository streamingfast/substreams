package request

import (
	"fmt"
	"regexp"
)

func validateNumbersOnly(in string) error {
	for _, r := range in {
		if r < '0' || r > '9' {
			return fmt.Errorf("only numbers are allowed")
		}
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
