package utils

import (
	"fmt"
)

type TextManager struct {
}

func NewTextManager() *TextManager {
	return &TextManager{}
}

func (tm *TextManager) ShortenString(s string, prefixLen, suffixLen int) string {
	if len(s) <= prefixLen+suffixLen {
		return s // If the string is already short, return as is
	}
	return fmt.Sprintf("%s...%s", s[:prefixLen], s[len(s)-suffixLen:])
}
