package stringutil

import "strings"

func DeletePreviousWord(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}

	endIndex := -1
	for i := len(s)-1; i >= 0; i-- {
		if s[i] == ' ' {
			endIndex = i
		} else if endIndex != -1 {
			break
		}
	}
	
	if endIndex == -1 {
		return ""
	}

	return s[:endIndex]
}
