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
	currentPath     string
	files           []fs.DirEntry
	selectedIdx     int
	scrollOffset    int
	screen          tcell.Screen
	logger          *slog.Logger
	logFile         *os.File
	showHiddenFiles bool
)

func main() {
	flag.BoolVar(&writeDebugLogs, "debug", false, "Write debug logs")

	logFile, err := os.OpenFile("pathsurfer.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
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
		logger.Debug("Couldn't read directory %s: %v!", currentPath, err)
		files = []fs.DirEntry{}
		selectedIdx = 0
		scrollOffset = 0

		return
	}

	logger.Debug("Updating file list...", "rawFileCount", len(rawFiles), "path", currentPath)

	if showHiddenFiles {
		logger.Debug("Showing hidden files.")
		files = rawFiles
	} else {
		files = []fs.DirEntry{}

		for _, f := range rawFiles {
			if !showHiddenFiles && strings.HasPrefix(f.Name(), ".") {
				continue
			}

			files = append(files, f)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	// Adjust the current file marker if it's now out of bounds.
	if selectedIdx >= len(files) {
		if len(files) > 0 {
			selectedIdx = len(files) - 1
		} else {
			selectedIdx = 0
		}
	}
	if len(files) == 0 {
		selectedIdx = 0
	}

	logger.Debug("Selected index after bounds check", "selectedIdx", selectedIdx)

	// Adjust scrollOffset to ensure the selected index is visible.
	visibleListHeight := 0
	if screen != nil {
		_, screenHeight := screen.Size()
		visibleListHeight = max(screenHeight-2, 1)
	} else {
		visibleListHeight = 20 // TODO: Remove magic value.
	}

	if selectedIdx < scrollOffset {
		scrollOffset = selectedIdx
	} else if selectedIdx >= scrollOffset+visibleListHeight {
		scrollOffset = selectedIdx - visibleListHeight + 1
	}

	maxPossibleScrollOffset := 0
	if len(files) > 0 {
		maxPossibleScrollOffset = max(len(files)-visibleListHeight, 0)
	}

	if scrollOffset > maxPossibleScrollOffset {
		scrollOffset = maxPossibleScrollOffset
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	if len(files) == 0 {
		scrollOffset = 0
	}
	logger.Debug(
		"Finished updating the file listing", "selectedIndex", selectedIdx, "scrollOffset", scrollOffset,
	)
}

func drawUI() {
	screen.Clear()
	w, h := screen.Size()

	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, "Path: "+currentPath)

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
	currRow := y1
	currCol := x1

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
	if ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'q':
			return keyHandlingResult{shouldQuit: true, newPath: currentPath}
		case 'j':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx + 1) % len(files)

			if screen == nil {
				break
			}

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

			if screen == nil {
				break
			}

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
			showHiddenFiles = !showHiddenFiles
			logger.Debug(fmt.Sprintf("Toggled showHiddenFiles to: %v.", showHiddenFiles))

			var previouslySelectedFileName string
			if len(files) > 0 && selectedIdx >= 0 && selectedIdx < len(files) {
				previouslySelectedFileName = files[selectedIdx].Name()
			}

			updateFileListings()

			newSelectedIdx := -1
			if previouslySelectedFileName != "" {
				for i, file := range files {
					if file.Name() == previouslySelectedFileName {
						newSelectedIdx = i
						break
					}
				}
			}

			if newSelectedIdx != -1 {
				selectedIdx = newSelectedIdx

				if screen == nil {
					break
				}

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
				if scrollOffset < 0 {
					scrollOffset = 0
				}
			}

			logger.Debug(
				"Handled '.' press",
				"selectedIndex",
				selectedIdx,
				"scrollOffset",
				scrollOffset,
				"fileCount",
				len(files),
			)
		}
	}

	return keyHandlingResult{shouldQuit: false, newPath: ""}
}
