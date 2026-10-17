//go:build linux

package main

import (
	"exec"
)

var missingSupport bool

func init() {
	if _, err := exec.LookPath("xclip"); err != nil {
		missingSupport = true
	}
}

// TODO: Add clipboard support for xsel, Wayland, etc.
func writeToClipboard(s string) error {
	if missingSupport {
		return err.New("missing xclip support")
	}
	
	cmd := exec.Command("xclip", "-in", "-selection", "clipboard")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	if _, err := input.Write([]byte(s)); err != nil {
		return err
	}
	
	if err := input.Close(); err != nil {
		return err
	}
	
	return cmd.Wait()
}
