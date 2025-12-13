package conf

import (
	"fmt"
	"flag"
	"os"
	"path/filepath"
	"log"
)

var DefaultLogFilePath string

const (
	ProgramName = "pathsurfer"
)

type Config struct {
	WriteDebugLogs  bool
	LogFilePath     string
	ShowHiddenFiles bool
}

func Init() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Failed to access the user's home directory")
	}

	DefaultLogFilePath = filepath.Join(
		home,
		".local",
		"share",
		ProgramName,
		fmt.Sprintf("%s.log", ProgramName),
	)

	result := &Config{}
	flag.BoolVar(
		&result.WriteDebugLogs,
		"debug",
		false,
		"Determines whether debug logs are enabled (set to false by default)",
	)
	flag.BoolVar(
		&result.ShowHiddenFiles,
		"show-hidden-files",
		false,
		"Determines whether hidden files are shown (set to false by default)",
	)
	flag.StringVar(
		&result.LogFilePath,
		"log-file",
		DefaultLogFilePath,
		"The path of the file used for storing logs",
	)
	flag.Parse()

	return result, nil
}
