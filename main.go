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

	"github.com/bnuredini/pathsurfer/fuzzy"
)

var (
	buildTime string
	version   string
)

// TODO: Clean-up these global variables.

var (
	writeDebugLogs  bool
	logFilePath     string
	showHiddenFiles bool

	screen tcell.Screen
	logger *slog.Logger

	currPath        string
	currMode        Mode
	currSearchEntry string
	files           []fs.DirEntry
	selectedIdx     int
	// Used when the number of files is higher than what can fit on the screen.
	// This value indicates how many lines/rows have been scrolled past by the
	// user.
	scrollOffset int
	// Keeps track of which position the cursor / selected row was on last time
	// for a given directory. This improves the experience of navigation by
	// allowing the user to quickly go back to the original path after they've
	// changed directories multiple times.
	positionHistory map[string]int
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

	positionHistory = make(map[string]int)
}

func main() {
	// TOOD: Move these to config.go and add support for priting the config.
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
		&logFilePath,
		"log-file",
		DefaultLogFilePath,
		"The path of the file used for storing logs",
	)
	flag.Parse()

	if len(os.Args) > 1 {
		pathArg := strings.TrimSpace(os.Args[1])

		pathDirInfo, err := os.Stat(pathArg)
		if os.IsNotExist(err) {
			log.Fatalf("%q does not exist", pathArg)
		} else if err != nil {
			log.Fatalf("%q is not a valid path: %v", pathArg, err)
		} else if !pathDirInfo.IsDir() {
			log.Fatalf("%q is not a valid directory", pathArg)
		}

		currPath = pathArg
	}

	logDir := filepath.Dir(logFilePath)
	logDirInfo, err := os.Stat(logDir)
	if os.IsNotExist(err) {
		err = os.Mkdir(logDir, 0755)
		if err != nil {
			log.Printf("Failed to create %q for storing logs", logDir)
		}
	} else if err != nil {
		log.Fatalf("Failed to use %q for storing logs: %v", logDirInfo, err)
	} else if !logDirInfo.IsDir() {
		log.Fatalf("Cannot store logs in %q because %[1]q is not a directory", logDir)
	}

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
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
		if r := recover(); r != nil {
			logger.Error("application panicked", "panic", r)
		}

		if logFile != nil {
			logger.Debug("Application shutting down. Closing log file...")

			if closeErr := logFile.Close(); closeErr != nil {
				log.Fatalf("Failed to close log file: %v", closeErr)
			}
		}
	}()

	logger.Info("Starting...", "buildTime", buildTime, "version", version)

	if strings.TrimSpace(currPath) == "" {
		currPath, err = os.Getwd()
		if err != nil {
			logger.Info("Couldn't get current directory", "err", err)
			os.Exit(1)
		}
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

	updateFileListingsUsingPath(currPath)
	drawFileList(screen)

	finalPathToCd := ""
	keyEntered := make(chan *tcell.EventKey)

	go render(keyEntered)

MainLoop:
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawFileList(screen)

		case *tcell.EventKey:
			result := handleKeyPress(ev)
			if result.shouldQuit {
				finalPathToCd = result.newPath
				break MainLoop
			}

			keyEntered <- ev
		}
	}

	screen.Fini()

	if finalPathToCd != "" {
		fmt.Println(finalPathToCd)
	}
}

func updateFileListings(rawFiles []fs.DirEntry) {
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

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

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
	scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

	logger.Debug(
		"Finished updating the file listing", "selectedIndex", selectedIdx, "scrollOffset", scrollOffset,
	)
}

func updateFileListingsUsingPath(path string) {
	dir, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			logger.Error("Encountered a permissions issue when updating the file listing", "err", err)
		}

		logger.Error("Couldn't read directory", "currPath", currPath, "err", err)
		files = []fs.DirEntry{}
		selectedIdx = 0
		scrollOffset = 0

		return // TODO: Return an error here.
	}

	updateFileListings(dir)
}

func drawFileList(screen tcell.Screen) {
	screen.Clear()
	w, h := screen.Size()

	headerRowCount := 1
	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, fmt.Sprintf("Path: %s", currPath))

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
}

func drawInfoLine() {
	w, h := screen.Size()
	helpStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(0, h-1, w, h-1, helpStyle, "(j/k: up/down) (l: enter) (h: parent) (. hidden) (q: quit)")
}

func drawSearchLine(searchEntry string) {
	w, h := screen.Size()
	searchBarStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(0, h-1, w, h-1, searchBarStyle, fmt.Sprintf("/%s", searchEntry))
}

func drawSearchList(searchEntry string) {
	screen.Clear()
	w, _ := screen.Size()

	pathStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue)
	drawText(0, 0, w, 0, pathStyle, fmt.Sprintf("Path: %s", currPath))
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

	if currMode == ModeDefault && ev.Key() == tcell.KeyRune {
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

			scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

		case 'k':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-2, 1)

			scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles)

		case 'h':
			logger.Debug("Updating position history...", "path", currPath, "idx", selectedIdx)
			positionHistory[currPath] = selectedIdx

			oldPath := currPath
			newPath := filepath.Dir(currPath)
			currPath = newPath

			idxFromHistory, ok := positionHistory[newPath]
			if !ok {
				updateFileListingsUsingPath(currPath)

				for i, f := range files {
					if f.Name() == filepath.Base(oldPath) {
						selectedIdx = i
					}
				}
			} else {
				updateFileListingsUsingPath(currPath)
				selectedIdx = idxFromHistory
			}

		case 'l':
			if selectedIdx < len(files) && files[selectedIdx].IsDir() {
				positionHistory[currPath] = selectedIdx

				currPath = filepath.Join(currPath, files[selectedIdx].Name())
				updateFileListingsUsingPath(currPath)

				if idxFromHistory, ok := positionHistory[currPath]; ok {
					selectedIdx = idxFromHistory
				} else {
					selectedIdx = 0
				}
			}

		case '.':
			showHiddenFiles = !showHiddenFiles
			updateFileListingsUsingPath(currPath)

		case '/':
			currMode = ModeSearch
			return result
		}
	}

	if currMode == ModeSearch {
		switch ev.Key() {
		case tcell.KeyRune:
			currSearchEntry = currSearchEntry + string(ev.Rune())
			matches, _ := searchInDir(currSearchEntry, files)
			logger.Debug("Finished searching", "matches", matches)
			updateFileListings(matches)

		case tcell.KeyBackspace, 127:
			logger.Debug("Processing a backspace...", "currSearchEntry", currSearchEntry, "newSearchEntry", currSearchEntry[:len(currSearchEntry)-1])
			currSearchEntry = currSearchEntry[:len(currSearchEntry)-1]
			matches, _ := searchInDir(currSearchEntry, files)
			updateFileListings(matches)

		case tcell.KeyCR:
			currMode = ModeDefault
			matches, _ := searchInDir(currSearchEntry, files)
			currSearchEntry = ""
			updateFileListings(matches) // TODO: Handle the error from search.

		case tcell.KeyESC:
			// Disable search mode and ignore the current search string.
			currMode = ModeDefault
		}
	}

	return result
}

func calculateScrollOffset(selectedIdx, currScrollOffset, heightUsableForFiles int) int {
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

func searchInDir(pattern string, candidateFiles []fs.DirEntry) ([]fs.DirEntry, error) {
	if len(candidateFiles) == 0 {
		return []fs.DirEntry{}, nil
	}
	// PERF: Use better/custom data structures to avoid these transformations.
	result := []fs.DirEntry{}
	candidates := []string{}
	candidatesMap := make(map[string]fs.DirEntry)

	for _, f := range candidateFiles {
		candidates = append(candidates, f.Name())
		candidatesMap[f.Name()] = f
	}

	matches := fuzzy.Find(pattern, candidates)
	for _, match := range matches {
		if dirEntry, ok := candidatesMap[match.CandidateString]; ok {
			result = append(result, dirEntry)
		}
	}

	return result, nil
}

func render(keyChanges chan *tcell.EventKey) {
	logger.Debug("Started listening for changes to the listing...")
	for {
		eventKey := <-keyChanges
		keyRune := eventKey.Rune()
		key := eventKey.Key()

		logger.Debug("render: processing...", "keyRune", eventKey.Rune(), "keyString", string(eventKey.Rune()), "currMode", currMode, "selectedIdx", selectedIdx)

		switch currMode {
		case ModeDefault:
			if keyRune == 'h' || keyRune == 'j' || keyRune == 'k' || keyRune == 'l' || key == tcell.KeyCR {
				drawFileList(screen)
				drawInfoLine()
				screen.Show()
			}

		case ModeSearch:
			drawFileList(screen)
			drawSearchLine(currSearchEntry)
			screen.Show()
		}
	}
}
