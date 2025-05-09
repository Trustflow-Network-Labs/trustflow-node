package utils

import (
	"fmt"
	"strconv"
	"strings"
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

func (tm *TextManager) SplitAndTrimCsv(s string) []string {
	if s == "" {
		return []string{}
	}
	ssa := strings.Split(s, ",")
	for i, ss := range ssa {
		ssa[i] = strings.TrimSpace(ss)
	}
	return ssa
}

func (tm *TextManager) ToBool(s string) (bool, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, err
	}
	return b, nil
}

func (tm *TextManager) ToInt64(s string) (int64, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (tm *TextManager) ToInt(s string) (int, error) {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (tm *TextManager) ToFloat64(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}
