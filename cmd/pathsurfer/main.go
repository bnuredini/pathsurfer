package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/gdamore/tcell/v2"

	"github.com/bnuredini/pathsurfer/internal/conf"
	"github.com/bnuredini/pathsurfer/internal/fuzzy"
)

// TODO: Clean-up these global variables.

var (
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
	scrollOffset       int
	parentScrollOffset int
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

var (
	StylePathIndicator = tcell.StyleDefault.Foreground(tcell.ColorBlue)
)

func main() {
	config, err := conf.Init()
	if err != nil {
		log.Fatalf("Failed to boot up: %v", err)
	}

	if len(flag.Args()) > 0 {
		pathArg := strings.TrimSpace(flag.Args()[0])

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

	logDir := filepath.Dir(config.LogFilePath)
	logDirInfo, err := os.Stat(logDir)
	if os.IsNotExist(err) {
		err = os.Mkdir(logDir, 0755)
		if err != nil {
			log.Printf("Failed to create %q for storing logs", logDir)
		}
	} else if err != nil {
		log.Fatalf("Failed to use %q for storing logs: %v", logDir, err)
	} else if !logDirInfo.IsDir() {
		log.Fatalf("Cannot store logs in %q because %[1]q is not a directory", logDir)
	}

	logFile, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	logHandlerOpts := &slog.HandlerOptions{}
	if config.WriteDebugLogs {
		logHandlerOpts.Level = slog.LevelDebug
	} else {
		logHandlerOpts.Level = slog.LevelInfo
	}

	logHandler := slog.NewTextHandler(logFile, logHandlerOpts)
	logger = slog.New(logHandler)

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Application panicked", "panic", r)
		}

		if logFile != nil {
			logger.Debug("Application shutting down. Closing log file...")

			if closeErr := logFile.Close(); closeErr != nil {
				log.Fatalf("Failed to close log file: %v", closeErr)
			}
		}
	}()

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
	defer screen.Fini()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalCh
		screen.Fini()
		if logFile != nil {
			_ = logFile.Close()
		}
		os.Exit(0)
	}()

	screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset))
	screen.Clear()

	finalPathToCd := ""
	positionHistory = make(map[string]int)

	updateFileListingsUsingPath(currPath, config)
	drawFileList(screen, config)

	keyEnteredCh := make(chan *tcell.EventKey)

	go render(keyEnteredCh, config)

MainLoop:
	for {
		ev := screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawFileList(screen, config)

		case *tcell.EventKey:
			result := handleKeyPress(ev, config)
			if result.shouldQuit {
				finalPathToCd = result.newPath
				break MainLoop
			}

			keyEnteredCh <- ev
		}
	}

	// Assuming that the user is using one of the wrapper scripts (psurf.sh or
	// psurf.fish), this program will print the current directory and the wrapper
	// will change the directory to it.
	if finalPathToCd != "" {
		fmt.Println(finalPathToCd)
	}
}

func getFilteredDirEntires(path string, config *conf.Config) []fs.DirEntry {
	result := []fs.DirEntry{}

	if strings.TrimSpace(path) == "" {
		return result
	}

	rawFiles, err := os.ReadDir(path)
	if err != nil {
		// TODO: Return an error value here and display it on the screen.
		logger.Debug("Failed to read directory", "path", path, "err", err)
		return result
	}

	if config.ShowHiddenFiles {
		result = rawFiles
	} else {
		result = []fs.DirEntry{}

		for _, f := range rawFiles {
			if !strings.HasPrefix(f.Name(), ".") {
				result = append(result, f)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})

	return result
}

func updateFileListings(rawFiles []fs.DirEntry, config *conf.Config) {
	if config.ShowHiddenFiles {
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

	if len(files) == 0 {
		selectedIdx = 0
	} else if selectedIdx >= len(files) {
		selectedIdx = len(files) - 1
	}

	_, screenHeight := screen.Size()
	heightUsableForFiles := max(screenHeight-2, 1)
	scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles, len(files))
}

func updateFileListingsUsingPath(path string, config *conf.Config) {
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

	updateFileListings(dir, config)
}

func drawFileList(screen tcell.Screen, config *conf.Config) {
	screen.Clear()

	w, h := screen.Size()
	paneWidth := w / 3

	leftPaneDimensions := paneDimensions{
		x1: 0,
		y1: 1,
		x2: paneWidth,
		y2: h - 1,
	}
	mainPaneDimensions := paneDimensions{
		x1: leftPaneDimensions.x2 + 2, 
		y1: 1, 
		x2: leftPaneDimensions.x2 + paneWidth, 
		y2: h - 1,
	}
	rightPaneDimensions := paneDimensions{
		x1: mainPaneDimensions.x2 + 2, 
		y1: 1, 
		x2: mainPaneDimensions.x2 + paneWidth, 
		y2: h - 1,
	}

	sep1X := leftPaneDimensions.x2 + 1
	sep2X := mainPaneDimensions.x2 + 1

	for i := range h {
		screen.SetContent(sep1X, i, '|', nil, tcell.StyleDefault)
		screen.SetContent(sep2X, i, '|', nil, tcell.StyleDefault)
	}

	drawText(screen, 0, 0, w, 0, StylePathIndicator, fmt.Sprintf("Parent: %s", currPath))
	drawText(screen, mainPaneDimensions.x1, 0, w, 0, StylePathIndicator, fmt.Sprintf("Navigating: %s", currPath))

	files := getFilteredDirEntires(currPath, config)

	parentSelectedIdx := 0
	parentFiles := []fs.DirEntry{}
	parentDir := filepath.Dir(currPath)

	if strings.TrimSpace(parentDir) != "" || parentDir != "/" {
		parentFiles = getFilteredDirEntires(filepath.Dir(currPath), config)

		for i, f := range parentFiles {
			if f.Name() == filepath.Base(currPath) {
				parentSelectedIdx = i
			}
		}
	}

	parentScrollOffset = calculateScrollOffset(
		parentSelectedIdx,
		parentScrollOffset,
		max(h-2, 1),
		len(parentFiles),
	)

	childFiles := []fs.DirEntry{}
	if files[selectedIdx].IsDir() {
		childDir := filepath.Join(currPath, files[selectedIdx].Name())
		childFiles = getFilteredDirEntires(childDir, config)
	}

	drawPane(screen, parentFiles, leftPaneDimensions, parentSelectedIdx, parentScrollOffset)
	drawPane(screen, files, mainPaneDimensions, selectedIdx, scrollOffset)
	drawPane(screen, childFiles, rightPaneDimensions, 0, 0)
}

type paneDimensions struct {
	x1, y1, x2, y2 int
}

func drawPane(screen tcell.Screen, entries []fs.DirEntry, dimensions paneDimensions, selectedMarker int, scrollMarker int) {
	heightUsableForFiles := dimensions.y2 - dimensions.y1

	for i := 0; i < heightUsableForFiles; i++ {
		fileIdx := scrollMarker + i
		if fileIdx >= len(entries) {
			break
		}

		prefix := "  "
		style := tcell.StyleDefault
		file := entries[fileIdx]

		if file.IsDir() {
			prefix = "📁 "
			style = style.Foreground(tcell.ColorGreen)
		}
		if fileIdx == selectedMarker {
			style = style.Background(tcell.ColorDarkGray).Foreground(tcell.ColorWhite)
		}

		drawText(
			screen,
			dimensions.x1,
			i+1,
			dimensions.x2,
			i+1,
			style,
			fmt.Sprintf("%s%s", prefix, file.Name()),
		)
	}
}

func drawInfoLine(screen tcell.Screen) {
	w, h := screen.Size()
	helpStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(screen, 0, h-1, w, h-1, helpStyle, "(j/k: up/down) (l: enter) (h: parent) (. hidden) (q: quit)")
}

func drawSearchLine(screen tcell.Screen, searchEntry string) {
	w, h := screen.Size()
	searchBarStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)
	drawText(screen, 0, h-1, w, h-1, searchBarStyle, fmt.Sprintf("/%s", searchEntry))
}

func drawText(screen tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, text string) {
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

func handleKeyPress(ev *tcell.EventKey, config *conf.Config) keyHandlingResult {
	var result keyHandlingResult

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

			scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles, len(files))

		case 'k':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-2, 1)

			scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles, len(files))

		case 'h':
			positionHistory[currPath] = selectedIdx

			oldPath := currPath
			newPath := filepath.Dir(currPath)
			currPath = newPath

			idxFromHistory, ok := positionHistory[newPath]
			if !ok {
				updateFileListingsUsingPath(currPath, config)

				for i, f := range files {
					if f.Name() == filepath.Base(oldPath) {
						selectedIdx = i
					}
				}
			} else {
				updateFileListingsUsingPath(currPath, config)
				selectedIdx = idxFromHistory
			}

		case 'l':
			if selectedIdx < len(files) && files[selectedIdx].IsDir() {
				parentScrollOffset = scrollOffset
				positionHistory[currPath] = selectedIdx

				currPath = filepath.Join(currPath, files[selectedIdx].Name())
				updateFileListingsUsingPath(currPath, config)

				if idxFromHistory, ok := positionHistory[currPath]; ok {
					selectedIdx = idxFromHistory
				} else {
					selectedIdx = 0
				}
			}

		case '.':
			config.ShowHiddenFiles = !config.ShowHiddenFiles
			updateFileListingsUsingPath(currPath, config)

		case '/':
			currMode = ModeSearch
			return result
		}
	}

	if currMode == ModeSearch {
		switch ev.Key() {
		case tcell.KeyRune:
			currSearchEntry = currSearchEntry + string(ev.Rune())
			matches, _ := searchInDir(currSearchEntry, files) // TODO: Handle the error from search.
			updateFileListings(matches, config)

		case tcell.KeyBackspace, 127:
			if len(currSearchEntry) == 0 {
				break
			}

			currDirFiles, err := os.ReadDir(currPath)
			if err != nil {
				log.Fatalf("Failed to read path %q", currPath)
			}

			if len(currSearchEntry) == 1 {
				currSearchEntry = ""
				updateFileListings(currDirFiles, config)
			} else {
				currSearchEntry = currSearchEntry[:len(currSearchEntry)-1]
				matches, _ := searchInDir(currSearchEntry, currDirFiles)
				updateFileListings(matches, config)
			}

		case tcell.KeyCR:
			if currSearchEntry == "" {
				currDirFiles, err := os.ReadDir(currPath)
				if err != nil {
					log.Fatalf("Failed to read path %q", currPath)
				}

				updateFileListings(currDirFiles, config)
			}

			currMode = ModeDefault
			currSearchEntry = ""

		case tcell.KeyESC:
			// Disable search mode and ignore the current search string.
			currMode = ModeDefault
			currSearchEntry = ""

		case tcell.KeyTAB:
			if len(files) == 0 {
				break
			}

			firstMatch := files[0]
			if firstMatch.IsDir() {
				if selectedIdx < len(files) && files[selectedIdx].IsDir() {
					positionHistory[currPath] = selectedIdx

					currPath = filepath.Join(currPath, files[selectedIdx].Name())
					updateFileListingsUsingPath(currPath, config)

					if idxFromHistory, ok := positionHistory[currPath]; ok {
						selectedIdx = idxFromHistory
					} else {
						selectedIdx = 0
					}
				}
			}

			currSearchEntry = ""
		}
	}

	return result
}

// TODO: Wrapping is buggy right now. Try wrapping in a directory with a lot of files.
func calculateScrollOffset(selectedIdx, currScrollOffset, heightUsableForFiles, listLen int) int {
	result := 0

	if selectedIdx < currScrollOffset {
		// Gone over the top edge. The scroll marker should be where the current
		// file marker is.
		result = selectedIdx
	} else if selectedIdx >= currScrollOffset+heightUsableForFiles {
		// Gone over the bottom edge. Since the file marker is at the bottom,
		// the scroll marker should be (heightUsableForFileList - 1) rows behind
		// the file marker.
		result = selectedIdx - (heightUsableForFiles - 1)
	} else {
		// Keep the scroll offset the same as long as the edges are not being touched.
		result = currScrollOffset
	}

	maxOffset := max((listLen-1)-heightUsableForFiles, 0)

	return min(result, maxOffset)
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

func render(keyChanges chan *tcell.EventKey, config *conf.Config) {
	logger.Debug("Started listening for changes to the listing...")
	for {
		eventKey := <-keyChanges
		keyRune := eventKey.Rune()
		key := eventKey.Key()

		logger.Debug("render: processing...", "keyRune", eventKey.Rune(), "keyString", string(eventKey.Rune()), "currMode", currMode, "selectedIdx", selectedIdx)

		switch currMode {
		case ModeDefault:
			if keyRune == 'h' || keyRune == 'j' || keyRune == 'k' || keyRune == 'l' || keyRune == '.' || key == tcell.KeyBackspace || key == tcell.KeyCR || key == tcell.KeyTAB {
				drawFileList(screen, config)
				drawInfoLine(screen)
				screen.Show()
			}

		case ModeSearch:
			drawFileList(screen, config)
			drawSearchLine(screen, currSearchEntry)
			screen.Show()
		}
	}
}
