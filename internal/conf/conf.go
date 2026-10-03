package conf

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
)

var (
	buildTime string
	version   string
)

var DefaultLogFilePath string
var DefaultMarkFilePath string

const (
	ProgramName = "pathsurfer"
)

type Config struct {
	WriteDebugLogs  bool
	LogFilePath     string
	MarkFilePath    string
	ShowHiddenFiles bool
}

func init() {
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
	DefaultMarkFilePath = filepath.Join(
		home,
		".config",
		ProgramName,
		fmt.Sprintf("%s.mark", ProgramName),
	)
}

func Init() (*Config, error) {
	flag.Usage = func() {
		cliOutput := flag.CommandLine.Output()

		fmt.Fprintln(cliOutput, "psurf - a tiny terminal utility for fast directory navigating")
		fmt.Fprintln(cliOutput, "")
		fmt.Fprintln(cliOutput, "Usage:")
		fmt.Fprintln(cliOutput, "  psurf [options] [path]")
		fmt.Fprintln(cliOutput, "")
		fmt.Fprintln(cliOutput, "Options:")
		flag.PrintDefaults()
	}

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
	flag.StringVar(
		&result.MarkFilePath,
		"mark-file",
		DefaultMarkFilePath,
		"The path of the file used for storing marks",
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
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "version:\t%s\n", version)
		fmt.Fprintf(out, "build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	if *displayHelp {
		flag.Usage()
		os.Exit(0)
	}

	markDir := filepath.Dir(result.MarkFilePath)
	markDirInfo, err := os.Stat(markDir)
	if os.IsNotExist(err) {
		if err = os.Mkdir(markDir, 0755); err != nil {
			log.Printf("Failed to create %q for storing marks", markDir)
		}
	} else if err != nil {
		log.Fatalf("Failed to use %q for storing marks: %v", markDir, err)
	} else if !markDirInfo.IsDir() {
		log.Fatalf("Cannot store marks in %q because %q is not a directory", result.MarkFilePath, markDir)
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
