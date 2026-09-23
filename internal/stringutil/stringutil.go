package stringutil

import "strings"

func DeletePreviousWord(s string) string {
	if strings.TrimSpace(s) == "" || len(s) == 1 {
		return ""
	}

	endIndex := -1
	
	for i := len(s)-2; i >= 0; i-- {
		if s[i] == ' ' && s[i+1] != ' ' {
			endIndex = i+1
			break
		} 
	}
	
	if endIndex == -1 {
		return ""
	}

	return s[:endIndex]
}
