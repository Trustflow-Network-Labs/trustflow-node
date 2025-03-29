package utils

import (
	"fmt"
	"strconv"
)

type TextManager struct {
}

func NewTextManager() *TextManager {
	return &TextManager{}
}

func (tm *TextManager) Shorten(s string, prefixLen, suffixLen int) string {
	if len(s) <= prefixLen+suffixLen {
		return s // If the string is already short, return as is
	}
	return fmt.Sprintf("%s...%s", s[:prefixLen], s[len(s)-suffixLen:])
}

func (tm *TextManager) ToBool(s string) (bool, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, err
	}
	return b, nil
}
