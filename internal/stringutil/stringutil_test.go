package stringutil

import "testing"

func TestDeletePreviousWord(t *testing.T) {
	tests := []struct{
		Input string
		Expected string
	}{
		{
			Input: "",
			Expected: "",
		},
		{
			Input: "  ",
			Expected: "",
		},
		{
			Input: "test",
			Expected: "",
		},
		{
			Input: "test test",
			Expected: "test ",
		},
		{
			Input: "test test test",
			Expected: "test test ",
		},
		{
			Input: "test  test",
			Expected: "test  ",
		},
		{
			Input: " test",
			Expected: " ",
		},
		{
			Input: "  test",
			Expected: "  ",
		},
		{
			Input: "  test  test",
			Expected: "  test  ",
		},
		{
			Input: "test  test   test",
			Expected: "test  test   ",
		},
		{
			Input: "test  test   test   ",
			Expected: "test  test   ",
		},
	}

	for _, tt := range tests {
		got := DeletePreviousWord(tt.Input)
		if got != tt.Expected {
			t.Errorf("DeletePreviousWord(%q) = %q, want %q", tt.Input, got, tt.Expected)
		}
	}
}
