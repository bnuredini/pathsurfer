package stringutil

import "testing"

func TestDeletePreviousWord(t *testing.T) {
	data := []struct{
		Input string
		Want string
	}{
		{
			Input: "",
			Want: "",
		},
		{
			Input: "  ",
			Want: "",
		},
		{
			Input: "test",
			Want: "",
		},
		{
			Input: "test test",
			Want: "test ",
		},
		{
			Input: "test test test",
			Want: "test test ",
		},
		{
			Input: "test  test",
			Want: "test  ",
		},
		{
			Input: " test",
			Want: " ",
		},
		{
			Input: "  test",
			Want: "  ",
		},
		{
			Input: "  test  test",
			Want: "  test  ",
		},
		{
			Input: "test  test   test",
			Want: "test  test   ",
		},
		{
			Input: "test  test   test   ",
			Want: "test  test   ",
		},
	}

	for _, tt := range data {
		if  got, want := DeletePreviousWord(tt.Input), tt.Want; got != want {
			t.Errorf("DeletePreviousWord(%q) = %q, want %q", tt.Input, got, want)
		}
	}
}
