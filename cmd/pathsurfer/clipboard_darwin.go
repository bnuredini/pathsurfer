package main

// +build darwin

import (
	"os/exec"
)

func writeToClipboard(s string) error {
	cmd := exec.Command("pbcopy")
	
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	if _, err = input.Write([]byte(s)); err != nil {
		return err
	}

	if err = input.Close(); err != nil {
		return err
	}
	
	return cmd.Wait()
}
