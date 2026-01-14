package main

import (
	"flag"
	"slices"
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

type v4 struct {
	x1, y1, x2, y2 int
}

type Mode int

const (
	ModeDefault Mode = iota
	ModeSearch
)

// TODO: Clean-up these global variables.

var (
	screen tcell.Screen
	logger *slog.Logger

	currPath        string
	currMode        Mode
	currSearchEntry string
	files           []fs.DirEntry
	// Keeps track of which position the cursor / selected row was on last time
	// for a given directory. This improves the experience of navigation by
	// allowing the user to quickly go back to the original path after they've
	// changed directories multiple times.
	positionHistory map[string]int

	// Used when the number of files is higher than what can fit on the screen.
	// This value indicates how many lines/rows have been scrolled past by the
	// user.
	scrollOffset       int
	parentScrollOffset int
	selectedIdx        int
)

var (
	StylePathIndicator = tcell.StyleDefault.Foreground(tcell.ColorGray)
	StyleActivePathIndicator = tcell.StyleDefault.Foreground(tcell.ColorBlue)
	StyleReset = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	StyleError = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkRed)
	StyleInfo = tcell.StyleDefault.Foreground(tcell.ColorYellow)
)

var RunesThatTriggerRedrawInDefault = []rune{
	'h',
	'j',
	'k',
	'l',
	'.',
}

var KeysThatTriggerRedrawInDefault = []tcell.Key{
	tcell.KeyBackspace,
	tcell.KeyCR,
	tcell.KeyTAB,
	tcell.KeyESC,
}

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

	screen.SetStyle(StyleReset)
	screen.Clear()

	pathToPrint := ""
	positionHistory = make(map[string]int)

	updateFileListingsUsingPath(currPath, config)
	drawFileList(screen, config)

	keyEnteredCh := make(chan *tcell.EventKey)
	errorCh := make(chan error)
	go render(keyEnteredCh, errorCh, config)

MainLoop:
	for {
		ev := screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawFileList(screen, config)

		case *tcell.EventKey:
			result, err := handleKeyPress(ev, config)
			if result.shouldQuit {
				pathToPrint = result.newPath
				break MainLoop
			}

			if err != nil {
				errorCh <- err
			} else {
				keyEnteredCh <- ev
			}
		}
	}

	// Assuming that the user is using one of the wrapper scripts (psurf.sh or
	// psurf.fish), this program will print the current directory and the
	// wrapper will change the shell's directory to what gets printed here.
	if pathToPrint != "" {
		fmt.Println(pathToPrint)
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
	secondaryPaneWidth := w / 6

	leftPaneDimensions := v4{
		x1: 0,
		y1: 2,
		x2: secondaryPaneWidth,
		y2: h - 1,
	}
	mainPaneDimensions := v4{
		x1: leftPaneDimensions.x2 + 2,
		y1: 2,
		x2: leftPaneDimensions.x2 + (3*secondaryPaneWidth),
		y2: h - 1,
	}
	rightPaneDimensions := v4{
		x1: mainPaneDimensions.x2 + 2,
		y1: 2,
		x2: mainPaneDimensions.x2 + secondaryPaneWidth,
		y2: h - 1,
	}

	sep1X := leftPaneDimensions.x2 + 1
	sep2X := mainPaneDimensions.x2 + 1

	for i := range h {
		screen.SetContent(sep1X, i, '|', nil, tcell.StyleDefault)
		screen.SetContent(sep2X, i, '|', nil, tcell.StyleDefault)
	}

	dimensions := v4{x1: mainPaneDimensions.x1, y1: 0, x2: w, y2: 0}
	switch currMode {
    case ModeDefault:
		drawText(screen, dimensions, StylePathIndicator, fmt.Sprintf("Navigating: %s", currPath))
	case ModeSearch:
		content := fmt.Sprintf("Searching: %s/%s", currPath, currSearchEntry)
		drawText(screen, dimensions, StyleActivePathIndicator, content)
	}

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
	if selectedIdx < len(files) && files[selectedIdx].IsDir() {
		childDir := filepath.Join(currPath, files[selectedIdx].Name())
		childFiles = getFilteredDirEntires(childDir, config)
	}

	drawPane(screen, parentFiles, leftPaneDimensions, parentSelectedIdx, parentScrollOffset)
	drawPane(screen, files, mainPaneDimensions, selectedIdx, scrollOffset)
	drawPane(screen, childFiles, rightPaneDimensions, 0, 0)
}

func drawPane(screen tcell.Screen, entries []fs.DirEntry, dimensions v4, selectedMarker int, scrollMarker int) {
	heightUsableForFiles := dimensions.y2 - dimensions.y1

	for i := range heightUsableForFiles {
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
			v4{x1: dimensions.x1, y1: dimensions.y1+i, x2: dimensions.x2, y2: dimensions.y1+i},
			style,
			fmt.Sprintf("%s%s", prefix, file.Name()),
		)
	}
}

func drawInfoLine(screen tcell.Screen) {
	w, h := screen.Size()
	dimensions := v4{0, h-1, w, h-1}
	drawText(
		screen,
		dimensions,
		StyleInfo,
		"(j/k: up/down) (l: enter) (h: parent) (. hidden) (q: quit)",
	)
}

func drawErrorLine(screen tcell.Screen, err error) {
	w, h := screen.Size()
	dimensions := v4{0, h-1, w, h-1}
	drawText(
		screen,
		dimensions,
		StyleError,
		err.Error(),
	)
}

func drawText(screen tcell.Screen, dimensions v4, style tcell.Style, text string) {
	currCol := dimensions.x1
	currRow := dimensions.y1

	for _, r := range text {
		screen.SetContent(currCol, currRow, r, nil, style)
		currCol++
		if currCol >= dimensions.x2 {
			currRow++
			currCol = dimensions.x1
		}
		if currRow > dimensions.y2 {
			break
		}
	}
}

type keyHandlingResult struct {
	shouldQuit bool
	newPath    string
}

func handleKeyPress(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	// Some terminals deliver Ctrl+C as \x03. Code point 3 is the ASCII ETX
	// control character.
	if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 3 {
		return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil
	}

	if currMode == ModeDefault && ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'q':
			return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil

		case 'j':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx + 1) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-3, 1) // TODO: Store pane info elsewhere. That would remove this magic 3.

			scrollOffset = calculateScrollOffset(selectedIdx, scrollOffset, heightUsableForFiles, len(files))

		case 'k':
			if len(files) == 0 {
				break
			}

			selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
			_, screenHeight := screen.Size()
			heightUsableForFiles := max(screenHeight-3, 1)

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
			return keyHandlingResult{shouldQuit: false, newPath: ""}, nil
		}
	}

	if currMode == ModeSearch {
		switch ev.Key() {
		case tcell.KeyRune:
			currSearchEntry = currSearchEntry + string(ev.Rune())
			matches, err := searchInDir(currSearchEntry, files)
			if err != nil {
				return keyHandlingResult{}, err
			}

			updateFileListings(matches, config)

		case tcell.KeyBackspace, 127:
			if len(currSearchEntry) == 0 {
				break
			}

			currDirFiles, err := os.ReadDir(currPath)
			if err != nil {
				return keyHandlingResult{}, err
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
			// Disable search mode and ignore the current search string. This is
			// consistent with how searching work in Vim.
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

	return keyHandlingResult{shouldQuit: false, newPath: ""}, nil
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

func render(keyChangesCh chan *tcell.EventKey, errorCh chan error, config *conf.Config) {
	for {
		select {
		case  eventKey := <-keyChangesCh:
			keyRune := eventKey.Rune()
			key := eventKey.Key()

			logger.Debug("render: processing...", "keyRune", eventKey.Rune(), "keyString", string(eventKey.Rune()), "currMode", currMode, "selectedIdx", selectedIdx)

			shouldRedrawInDefault :=
				slices.Contains(RunesThatTriggerRedrawInDefault, keyRune) ||
				slices.Contains(KeysThatTriggerRedrawInDefault, key)

			if (currMode == ModeDefault || shouldRedrawInDefault) || currMode == ModeSearch {
				drawFileList(screen, config)
				drawInfoLine(screen)
				screen.Show()
			}

		case err := <-errorCh:
			drawErrorLine(screen, err)
			screen.Show()
		}
	}
}
