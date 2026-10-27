// TODO: Three improvements:
// 
// * Selection options, allowing the user to specify what integration level they
//   want: which shells to integrate for & whether they want key binding support.
// * Recovering from errors: if fish integration failed, don't stop, try to
//   integrate bash and the rest of the things. 
// * Idempotent: if some part of the setup is already there, do the rest.
 
package wizard

import (
	"fmt"
	"runtime"
	"path/filepath"
	"os"
	"io"
	
	"github.com/bnuredini/pathsurfer/scripts"
)

var binaryInstallPath string
var currentBinaryPath string

var fishConfigDirPath string
var fishFunctionPath string
var fishKeybindingsPath string

var bashOrZshFunctionPath string

var bashrcPath string
var zshrcPath string

var bashOrZshRcString string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Failed to access the user's home directory\n")
		os.Exit(1)
	}
	
	binaryInstallPath = filepath.Join(home, ".local", "bin", "pathsurfer")
	currentBinaryPath, err = os.Executable()
	if err != nil {
		panic(err)
	}
	
	fishConfigDirPath = filepath.Join(home, ".config", "fish", "conf.d")
	fishFunctionPath = filepath.Join(fishConfigDirPath, "psurf.fish")
	fishKeybindingsPath = filepath.Join(fishConfigDirPath, "psurf_keybindings.fish")
	
	bashOrZshFunctionPath = filepath.Join(home, ".local", "share", "pathsurfer", "functions", "psurf.sh")
	
	bashrcPath = filepath.Join(home, ".bashrc")
	zshrcPath = filepath.Join(home, ".zshrc")

	bashOrZshRcString = fmt.Sprintf("\n# Load the psurf shell function\n source %v\n", bashOrZshFunctionPath)
}

func Start() error {
	fmt.Printf("\nRunning the wizard...\n\n")
	
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = darwinWizard()
	case "linux":
		// TODO: Wizard support for Linux.
	case "windows":
		// TODO: Wizard support for Windows.
	default:
		fmt.Printf("Cannot run the wizard: failed to detect operating system.")
		os.Exit(1)
	}
	
	if err != nil {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
	
	return nil
}

func darwinWizard() error {
	if err := os.MkdirAll(fishConfigDirPath, 0755); err != nil {
		fmt.Printf("Failed to create %q: %v", fishConfigDirPath, err)
	}
	
	fishFunctionFile, err := os.OpenFile(fishFunctionPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open %q: %v\n", fishFunctionPath, err)
	}
	defer fishFunctionFile.Close()

	fishFileFromFS, err := scripts.Files.Open("psurf.fish")
	if err != nil {
		fmt.Printf("Failed open psurf.fish: %v\n", err)
	}
	defer fishFileFromFS.Close()

	_, err = io.Copy(fishFunctionFile, fishFileFromFS)
	if err != nil {
		fmt.Printf("Failed to write to %q: %v\n", fishFunctionPath, err)
	}
	
	fmt.Printf(" [x] Integrated fish\n")
	
	bashOrZshFunctionDirPath := filepath.Dir(bashOrZshFunctionPath)
	if err = os.MkdirAll(bashOrZshFunctionDirPath, 0755); err != nil {
		fmt.Printf("Failed to create %q: %v", bashOrZshFunctionDirPath, err)
	}

	shFunctionFromFS, err := scripts.Files.Open("psurf.sh")
	if err != nil {
		fmt.Printf("Failed to read psurf.sh: %v\n", err)
	}
	defer shFunctionFromFS.Close()
	
	shFunctionFile, err := os.OpenFile(bashOrZshFunctionPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open %q: %v\n", bashOrZshFunctionPath, err)
	}
	defer shFunctionFile.Close()
	
	if _, err = io.Copy(shFunctionFile, shFunctionFromFS); err != nil {
		fmt.Printf("Failed to write to %q: %v\n", bashOrZshFunctionPath, err)
	}
	
	bashrcFile, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open %v: %q\n", bashrcPath, err)
	}
	defer bashrcFile.Close()
	
	_, err = bashrcFile.Write([]byte(bashOrZshRcString))
	if err != nil {
		fmt.Printf("Failed to write to %v: %q\n", bashrcFile, err)
	}
	
	fmt.Printf(" [x] Integrated bash\n")
	
	zshrcFile, err := os.OpenFile(zshrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open %v: %q\n", zshrcPath, err)
	}
	defer zshrcFile.Close()
	
	_, err = zshrcFile.Write([]byte(bashOrZshRcString))
	if err != nil {
		fmt.Printf("Failed to write to %v: %q\n", zshrcFile, err)
	}
	
	fmt.Printf(" [x] Integrated zsh\n")
	
	binaryInstallDirPath := filepath.Dir(binaryInstallPath)
	if err := os.MkdirAll(binaryInstallDirPath, 0755); err != nil {
		fmt.Printf("Failed to install the binary at %q: %v\n", binaryInstallDirPath, err)
		os.Exit(1)
	}

	if err = os.Rename(currentBinaryPath, binaryInstallPath); err != nil {
		fmt.Printf("Failed to move the binary to %q: %v", binaryInstallPath, err)
	}
	
	fmt.Printf(" [x] Installed %v\n", binaryInstallPath)

	return nil
}
