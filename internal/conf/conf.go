package conf

import (
	"fmt"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"encoding/json"
	"log"
)

var (
	buildTime string
	version   string
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
	flag.Usage = func() {
		cliOutput := flag.CommandLine.Output()

		fmt.Fprintln(cliOutput, "psurf - change directories quickly")
		fmt.Fprintln(cliOutput, "")
		fmt.Fprintln(cliOutput, "Usage:")
		fmt.Fprintln(cliOutput, "  psurf [options] [path]")
		fmt.Fprintln(cliOutput, "")
		fmt.Fprintln(cliOutput, "Options:")
		flag.PrintDefaults()
	}

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

	displayVersion := flag.Bool(
		"version",
		false,
		"Show version information",
	)
	displayHelp := flag.Bool(
		"help",
		false,
		"Show help information",
	)
	flag.Parse()

	if *displayVersion {
		fmt.Printf("version:\t%s\n", version)
		fmt.Printf("build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	if *displayHelp {
		flag.Usage()
		os.Exit(0)
	}

	return result, nil
}

func printConfig(c Config) error {
	configToPrint := Config{}
	valToPrint := reflect.ValueOf(&configToPrint).Elem()
	valToInspect := reflect.ValueOf(c)

	for i := range valToInspect.NumField() {
		if valToInspect.Type().Field(i).Tag.Get("sensitive") != "yes" {
			valToPrint.Field(i).Set(valToInspect.Field(i))
		}
	}

	b, err := json.Marshal(configToPrint)
	if err != nil {
		return err
	}

	log.Printf("build time: %v", buildTime)
	log.Printf("version: %v", version)
	log.Printf("config: %v", string(b))

	return nil
}
