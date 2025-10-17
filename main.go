package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"

)

var (
	buildTime string
	version string
)

// TODO: Clean-up these global variables.

var (
	writeDebugLogs  bool
	logFile         string
	showHiddenFiles bool

	currPath  string
	files        []fs.DirEntry
	selectedIdx  int
	scrollOffset int
	screen       tcell.Screen
	logger       *slog.Logger

	currMode        Mode
	currSearchEntry string
)

type Mode int

const (
	ModeDefault Mode = iota
	ModeSearch
)

const (
	ProgramName = "pathsurfer"
)

var DefaultLogFilePath string

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
}

func main() {
	flag.BoolVar(
		&writeDebugLogs,
		"debug",
		false,
		"Determines whether debug logs are enabled (set to false by default)",
	)
	flag.BoolVar(
		&showHiddenFiles,
		"show-hidden-files",
		false,
		"Determines whether hidden files are shown (set to false by default)",
	)
	flag.StringVar(
		&logFile,
		"log-file",
		DefaultLogFilePath,
		"The path of the file used for storing logs",
	)
	flag.Parse()

	logDir := filepath.Dir(logFile)
	info, err := os.Stat(logDir)
	if os.IsNotExist(err) {
		if err = os.Mkdir(logDir, 0755); err != nil {
			log.Printf("Failed to create %q for storing logs", logDir)
		}
	} else if err != nil {
		log.Fatalf("Failed to use %q for storing logs: %v", info, err)
	} else if !info.IsDir() {
		log.Fatalf("Cannot store logs in %q because %[1]q is not a directory", logDir)
	}
	
	// TODO: Set an absolute path for the log file if no custom path was provided. The path should
	// be determined during installation.
	logFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	logHandlerOpts := &slog.HandlerOptions{}
	if writeDebugLogs {
		logHandlerOpts.Level = slog.LevelDebug
	} else {
		logHandlerOpts.Level = slog.LevelInfo
	}

	logHandler := slog.NewTextHandler(logFile, logHandlerOpts)
	logger = slog.New(logHandler)
	defer func() {
		if logFile != nil {
			logger.Debug("Application shutting down. Closing log file...")

			if closeErr := logFile.Close(); closeErr != nil {
				log.Fatalf("Failed to close log file: %v", closeErr)
			}
		}
	}()

	logger.Info("Starting...", "buildTime", buildTime, "version", version)

	currPath, err = os.Getwd()
	if err != nil {
		logger.Info("Couldn't get current directory", "err", err)
		os.Exit(1)
	}

	screen, err = tcell.NewScreen()
	if err != nil {
		logger.Error("Couldn't create screen", "err", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		logger.Error("Couldn't initialize screen", "err", err)
		os.Exit(1)
	}

	screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset))
	screen.Clear()

	// TODO: Passing nil here doesn't immediately indicate that the function is suppose to read the
	// current directory. Add another function for this case. Consider something like
	// updateFileListingFromCurrDir.
	updateFileListings(nil)
	drawFileList()

	finalPathToCd := ""

MainLoop:
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawFileList()
		case *tcell.EventKey:
			result := handleKeyPress(ev)
			if result.shouldQuit {
				finalPathToCd = result.newPath
				break MainLoop
			}
		}
	}

	screen.Fini()

	if finalPathToCd != "" {
		fmt.Println(finalPathToCd)
	}
}

func updateFileListings(rawFiles []fs.DirEntry) {
	if len(rawFiles) == 0 {
		fromCurrDir, err := os.ReadDir(currPath)
		if err != nil {
			if os.IsPermission(err) {
				logger.Error("Encountered a permissions issue when updating the file listing", "err", err)
			}

			logger.Error("Couldn't read directory", "currentPath", currPath, "err", err)
			files = []fs.DirEntry{}
			selectedIdx = 0
			scrollOffset = 0

			return // TODO: Return an error here.
		}

		rawFiles = fromCurrDir
	}

	logger.Debug("Updating file list...", "rawFileCount", len(rawFiles), "path", currPath)

	if showHiddenFiles {
		logger.Debug("Showing hidden files.")
		files = rawFiles
	} else {
		files = []fs.DirEntry{}

		for _, f := range rawFiles {
			if !strings.HasPrefix(f.Name(), ".") {
				files = append(files, f)
			}
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	// Adjust the current file marker if it's out of bounds.
	if len(files) == 0 {
		selectedIdx = 0
	} else if selectedIdx >= len(files) {
		selectedIdx = len(files) - 1
	}

	logger.Debug("Selected index after bounds check", "selectedIdx", selectedIdx)

	// Adjust scrollOffset to ensure the selected index is visible.
	_, screenHeight := screen.Size()
	heightUsableForFiles := max(screenHeight-2, 1)
	scrollOffset = adjustScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

	logger.Debug(
		"Finished updating the file listing", "selectedIndex", selectedIdx, "scrollOffset", scrollOffset,
	)
}

func drawFileList() {
	screen.Clear()
	w, h := screen.Size()

	headerRowCount := 1
	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, "Path: "+currPath)

	heightUsableForFiles := max(h-2, 0)

	for i := 0; i < heightUsableForFiles; i++ {
		fileIdx := scrollOffset + i
		if fileIdx >= len(files) {
			break
		}

		file := files[fileIdx]
		rowToDrawOn := i + headerRowCount

		style := tcell.StyleDefault
		prefix := "  "

		if file.IsDir() {
			style = style.Foreground(tcell.ColorGreen)
			prefix = "📁 "
		}
		if fileIdx == selectedIdx {
			style = style.Background(tcell.ColorDarkGray).Foreground(tcell.ColorWhite)
		}

		logger.Debug(fmt.Sprintf("Drawing file %v at row %v", file.Name(), rowToDrawOn))

		drawText(0, rowToDrawOn, w, rowToDrawOn, style, prefix+file.Name())
	}

	helpStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(0, h-1, w, h-1, helpStyle, "(j/k: up/down) (l: enter) (h: parent) (. hidden) (q: quit)")

	screen.Show()
}

func drawSearch(searchEntry string) {
	screen.Clear()
	w, h := screen.Size()

	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, "Path: "+currPath)

	searchBarStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(0, h-1, w, h-1, searchBarStyle, fmt.Sprintf("/%s", searchEntry))

	screen.Show()
}

func drawText(x1, y1, x2, y2 int, style tcell.Style, text string) {
	logger.Debug("Drawing text", "x1", x1, "y1", y1, "x2", x2, "y2", y2, "text", text)
	currCol := x1
	currRow := y1

	for _, r := range text {
		screen.SetContent(currCol, currRow, r, nil, style)
		currCol++
		if currCol >= x2 {
			currRow++
			currCol = x1
		}
		if currRow > y2 {
			break
		}
	}
}

type keyHandlingResult struct {
	shouldQuit bool
	newPath    string
}

func handleKeyPress(ev *tcell.EventKey) keyHandlingResult {
	var result keyHandlingResult
	logger.Debug("Handling key press", "keyRune", ev.Rune(), "keyString", string(ev.Rune()))

	if currMode == ModeDefault {
		switch ev.Rune() {
		case 'q':
			return keyHandlingResult{shouldQuit: true, newPath: currPath}

		case 'j':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx + 1) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-2, 1)

			scrollOffset = adjustScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

		case 'k':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-2, 1)

			scrollOffset = adjustScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

		case 'h':
			parentDir := filepath.Dir(currPath)
			if parentDir != currPath {
				currPath = parentDir
				updateFileListings(nil)
			}

		case 'l':
			if selectedIdx < len(files) && files[selectedIdx].IsDir() {
				currPath = filepath.Join(currPath, files[selectedIdx].Name())
				updateFileListings(nil)
			}

		case '.':
			logger.Debug("Flipped toggle for hidden files", "showHiddenFiles", showHiddenFiles)
			showHiddenFiles = !showHiddenFiles
			updateFileListings(nil)

		case '/':
			currMode = ModeSearch

			// TODO: Don't remove the files when entering search mode. Files should be removed from
			// the screen as they're being discarded by the fuzzy-finding algorithm in real time.
			// (We use a separate goroutine for this.)

			drawSearch("")
			return result
		}

		drawFileList()
	}

	if currMode == ModeSearch {
		switch ev.Key() {
		case tcell.KeyCR:
			currMode = ModeDefault

			matches, _ := searchInDir(currPath, currSearchEntry)
			// TODO: Handle the error from search.

			updateFileListings(matches)
			drawFileList()
		case tcell.KeyESC:
			// Disable search mode and ignore the current search string.
			currMode = ModeDefault
		case tcell.KeyRune:
			currSearchEntry = currSearchEntry  + string(ev.Rune())

			drawSearch(currSearchEntry)
		}
	}

	return result
}

func adjustScrollOffset(selectedIdx, currScrollOffset, heightUsableForFiles int) int {
	if selectedIdx < currScrollOffset {
		return selectedIdx
	} else if selectedIdx >= currScrollOffset+heightUsableForFiles {
		// Since the file marker is at the bottom, the scroll marker should be
		// (heightUsableForFileList - 1) rows behind the file marker.

		return selectedIdx - (heightUsableForFiles - 1)
	}

	maxPossibleScrollOffset := max((len(files)-1)-heightUsableForFiles, 0)

	return min(currScrollOffset, maxPossibleScrollOffset)
}

func searchInDir(path, pattern string) ([]fs.DirEntry, error) {
	result := []fs.DirEntry{}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			logger.Error("Encountered a permissions issue whilst searching", "err", err)
		} else {
			logger.Error("Failed to get directory entires whilst searching", "err", err)
		}

		return nil, err
	}

	// TODO: Use a fuzzy-finding algorithm.
	for _, dirEntry := range dirEntries {
		if strings.Contains(dirEntry.Name(), pattern) {
			result = append(result, dirEntry)
		}
	}

	return result, nil
}
