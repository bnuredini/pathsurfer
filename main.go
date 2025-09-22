package main

import (
	"fmt"
	"io/fs"
	"flag"
	"os"
	"log/slog"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// TODO: Clean-up these global variables.

var (
	writeDebugLogs  bool
	logFile    		string
	currentPath     string
	files           []fs.DirEntry
	selectedIdx     int
	scrollOffset    int
	screen          tcell.Screen
	logger          *slog.Logger
	showHiddenFiles bool
)

func main() {
	flag.BoolVar(
		&writeDebugLogs,
		"debug",
		false,
		"Set this to true to enable debug logs (set to false by default)",
	)
	flag.StringVar(
		&logFile,
		"log-file",
		"pathsurfer.log",
		"The path of the file used for storing logs",
	)
	flag.Parse()

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

	logger.Info("Starting...")

	currentPath, err = os.Getwd()
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

	updateFileListings()
	drawUI()

	finalPathToCd := ""

MainLoop:
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawUI()
		case *tcell.EventKey:
			result := handleKeyPress(ev)
			if result.shouldQuit {
				finalPathToCd = result.newPath
				break MainLoop
			}

			drawUI()
		}
	}

	screen.Fini()

	if finalPathToCd != "" {
		fmt.Println(finalPathToCd)
	}
}

func updateFileListings() {
	rawFiles, err := os.ReadDir(currentPath)
	if err != nil {
		logger.Error("Couldn't read directory", "currentPath", currentPath, "err", err)
		files = []fs.DirEntry{}
		selectedIdx = 0
		scrollOffset = 0

		return // TODO: Return an error here.
	}

	logger.Debug("Updating file list...", "rawFileCount", len(rawFiles), "path", currentPath)

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
	heightUsableForFileList := max(screenHeight-2, 1)

	if selectedIdx < scrollOffset {
		scrollOffset = selectedIdx
	} else if selectedIdx >= scrollOffset+heightUsableForFileList {
		// Since the file marker is at the bottom, the scroll marker should be
		// (heightUsableForFileList - 1) rows behind the file marker.

		scrollOffset = selectedIdx - (heightUsableForFileList-1)
	}

	maxPossibleScrollOffset := max((len(files)-1) - heightUsableForFileList, 0)
	scrollOffset = min(scrollOffset, maxPossibleScrollOffset)

	logger.Debug(
		"Finished updating the file listing", "selectedIndex", selectedIdx, "scrollOffset", scrollOffset,
	)
}

func drawUI() {
	screen.Clear()
	w, h := screen.Size()

	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, "Path: " + currentPath)

	visibleListHeight := max(h-2, 0)

	for i := 0; i < visibleListHeight; i++ {
		fileIndexInFiles := scrollOffset + i
		if fileIndexInFiles >= len(files) {
			break
		}

		file := files[fileIndexInFiles]
		rowToDrawOn := i + 1

		style := tcell.StyleDefault
		prefix := "  "

		if file.IsDir() {
			style = style.Foreground(tcell.ColorGreen)
			prefix = "📁 "
		}
		if fileIndexInFiles == selectedIdx {
			style = style.Background(tcell.ColorDarkGray).Foreground(tcell.ColorWhite)
		}

		logger.Debug(fmt.Sprintf("Drawing file %v at row %v", file.Name(), rowToDrawOn))

		drawText(0, rowToDrawOn, w, rowToDrawOn, style, prefix+file.Name())
	}

	helpStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(0, h-1, w, h-1, helpStyle, "(j/k: up/down) (l: enter) (h: parent) (. hidden) (q: quit)")

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
	if ev.Key() != tcell.KeyRune {
		return keyHandlingResult{shouldQuit: false, newPath: ""}
	}

	switch ev.Rune() {
	case 'q':
		return keyHandlingResult{shouldQuit: true, newPath: currentPath}
	case 'j':
		if len(files) == 0 {
			break
		}

		selectedIdx = (selectedIdx + 1) % len(files)

		_, screenHeight := screen.Size()
		visibleListHeight := max(screenHeight-2, 1)
		if selectedIdx < scrollOffset {
			scrollOffset = selectedIdx
		} else if selectedIdx >= scrollOffset+visibleListHeight {
			scrollOffset++ // NOTE: This has to be updated when for keybindings like 3j, for example.
		}

		maxPossibleScrollOffset := max(len(files)-visibleListHeight, 0)

		if scrollOffset > maxPossibleScrollOffset {
			scrollOffset = maxPossibleScrollOffset
		}
	case 'k':
		if len(files) == 0 {
			break
		}

		selectedIdx = (selectedIdx - 1 + len(files)) % len(files)

		_, screenHeight := screen.Size()
		visibleListHeight := max(screenHeight-2, 1)
		if selectedIdx < scrollOffset {
			scrollOffset = selectedIdx
		} else if selectedIdx >= scrollOffset+visibleListHeight {
			scrollOffset = selectedIdx - visibleListHeight + 1
		}

		maxPossibleScrollOffset := max(len(files)-visibleListHeight, 0)

		if scrollOffset > maxPossibleScrollOffset {
			scrollOffset = maxPossibleScrollOffset
		}
	case 'h':
		parentDir := filepath.Dir(currentPath)
		if parentDir != currentPath {
			currentPath = parentDir
			updateFileListings()
		}
	case 'l':
		if len(files) > 0 && selectedIdx < len(files) && files[selectedIdx].IsDir() {
			currentPath = filepath.Join(currentPath, files[selectedIdx].Name())
			updateFileListings()
		}
	case '.':
	}

	return keyHandlingResult{shouldQuit: false, newPath: ""}
}
