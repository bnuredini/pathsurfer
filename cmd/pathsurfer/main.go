package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
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
	ModeRecordingMark
	ModeListeningForMark
)

type SearchBarPrefix string

const (
	SearchBarPrefixSearching  = "searching"
	SearchBarPrefixSearched   = "searched"
	SearchBarPrefixNavigating = "navigating"
)

const BigJumpLength = 22

// CLEANUP: Remove these global variables. Split them in separate struct types.
// Introduce different modules for different responsibilities: UI, user input,
// file management/parsing.
//
// Follow a simple: Model -> Render -> Update.
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
	searchBarPrefix    SearchBarPrefix

	// It makes sense to use this value only if waitingForAnotherKeyPress is
	// true. The purpose of these to variables is to add support for Vi-like
	// keybindings such as gg.
	previousKeyPressed        rune
	waitingForAnotherKeyPress bool

	marks map[rune]string
)

var (
	StylePathIndicator       = tcell.StyleDefault.Foreground(tcell.ColorGray)
	StyleActivePathIndicator = tcell.StyleDefault.Foreground(tcell.ColorBlue)
	StyleReset               = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	StyleError               = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkRed)
	StyleInfo                = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	StyleSelectedEntry       = tcell.StyleDefault.Background(tcell.ColorDarkBlue).Foreground(tcell.ColorWhite)
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

var ChainableKeybindings = map[rune][]rune {
	'g': []rune{'g'},
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

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
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
	searchBarPrefix = SearchBarPrefixNavigating

	marks, err = readMarks(config)
	if err != nil {
		// INCOMPLETE: Provide better information here.
		log.Fatalf("Failed to read marks: %v", err)
	}
	handleDirectoryChange(currPath, config)
	drawFileList(screen, config)
	drawInfoLine(screen)

	keyEnteredChan := make(chan *tcell.EventKey)
	errorChan := make(chan error)
	go render(keyEnteredChan, errorChan, config)

	running := true
	for running {
		ev := screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			drawFileList(screen, config)

		case *tcell.EventKey:
			result, err := handleKeyPress(ev, config)
			if result.shouldQuit {
				pathToPrint = result.newPath
				running = false
				break
			}

			if result.addingNewMark {
				marks, err = readMarks(config)
			}

			if err != nil {
				errorChan <- err
			} else {
				keyEnteredChan <- ev
			}
		}
	}

	screen.Fini()

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

// CLEANUP: This function updates too many state variables. Ideally, it should
// only touch the file listings.
func handleFileListingChange(rawFiles []fs.DirEntry, config *conf.Config) {
	updateFileListing(rawFiles, config)

	if len(files) == 0 {
		selectedIdx = 0
	} else if selectedIdx >= len(files) {
		selectedIdx = len(files) - 1
	}

	scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))
}

func updateFileListing(rawFiles []fs.DirEntry, config *conf.Config) {
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
}

func handleDirectoryChange(path string, config *conf.Config) {
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

	handleFileListingChange(dir, config)
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
		x2: leftPaneDimensions.x2 + (3 * secondaryPaneWidth),
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
		text := fmt.Sprintf("%s: %s", searchBarPrefix, currPath)
		drawText(screen, dimensions, StylePathIndicator, text)
	case ModeSearch:
		text := fmt.Sprintf("%s: %s/%s", searchBarPrefix, currPath, currSearchEntry)
		drawText(screen, dimensions, StyleActivePathIndicator, text)
		screen.ShowCursor(dimensions.x2+1, dimensions.y1)
		screen.SetCursorStyle(tcell.CursorStyleBlinkingBlock)
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

	parentScrollOffset = calculateScrollOffsetForHeight(
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
	if currMode == ModeSearch {
		// When in search mode, make sure the marker is at the top of the list.
		// Since the marker should be at the top, the pane should be drawn as if
		// both the selected index and the scroll offset are 0.
		drawPane(screen, files, mainPaneDimensions, 0, 0)
	} else {
		drawPane(screen, files, mainPaneDimensions, selectedIdx, scrollOffset)
	}
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
			style = StyleSelectedEntry
		}

		drawText(
			screen,
			v4{x1: dimensions.x1, y1: dimensions.y1 + i, x2: dimensions.x2, y2: dimensions.y1 + i},
			style,
			fmt.Sprintf("%s%s", prefix, file.Name()),
		)
	}
}

func drawInfoLine(screen tcell.Screen) {
	w, h := screen.Size()
	dimensions := v4{0, h - 1, w, h - 1}
	drawText(
		screen,
		dimensions,
		StyleInfo,
		"(j/k: up/down) (l: enter) (h: parent) (/: search) (. hidden) (q: quit)",
	)
}

func drawErrorLine(screen tcell.Screen, err error) {
	w, h := screen.Size()
	dimensions := v4{0, h - 1, w, h - 1}
	drawText(
		screen,
		dimensions,
		StyleError,
		err.Error(),
	)
}

func drawMarkHintSection(screen tcell.Screen, config *conf.Config) {
	w, h := screen.Size()

	idx := 0
	for entry, value := range marks {
		dimensions := v4{0, (h - 1) - idx, w, (h - 1) - idx}
		drawText(
			screen,
			dimensions,
			StyleInfo,
			fmt.Sprintf("%c\t%s", entry, value),
		)

		idx++
	}
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
	shouldQuit    bool
	addingNewMark bool
	newPath       string
}

func handleKeyPress(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{shouldQuit: false, newPath: ""}

	// Some terminals deliver Ctrl+C as \x03. Code point 3 is the ASCII ETX
	// control character.
	if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 3 {
		return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil
	}

	var err error
	if currMode == ModeDefault {
		result, err = handleKeyPressInDefault(ev, config)
		if err != nil {
			return result, err
		}
	} else if currMode == ModeSearch {
		result, err = handleKeyPressInSearch(ev, config)
		if err != nil {
			return result, err
		}
	} else if currMode == ModeRecordingMark {
		if ev.Key() != tcell.KeyRune {
			return result, errors.New("setting mark: value for mark must be a rune")
		}

		err := storeNewMark(ev.Rune(), currPath, config)
		if err != nil {
			// INCOMPLETE: Handle this error. Put a red error in the bottom line.
			return result, err
		}

		currMode = ModeDefault
		result.addingNewMark = true
	}

	return result, nil
}

func handleKeyPressInDefault(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{shouldQuit: false, newPath: ""}
	
	if waitingForAnotherKeyPress && !canKeyPressesBeChained(previousKeyPressed, ev.Rune()) {
		// CLEANUP: Find a better reset value.
		previousKeyPressed = ' '
		waitingForAnotherKeyPress = false
	}

	switch ev.Rune() {
	case 'q':
		return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil

	case 'j':
		if len(files) == 0 {
			break
		}

		selectedIdx = (selectedIdx + 1) % len(files)
		scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))

	case 'k':
		if len(files) == 0 {
			break
		}

		selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
		scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))

	case 'h':
		positionHistory[currPath] = selectedIdx

		oldPath := currPath
		newPath := filepath.Dir(currPath)
		currPath = newPath

		idxFromHistory, ok := positionHistory[newPath]
		if !ok {
			handleDirectoryChange(currPath, config)

			for i, f := range files {
				if f.Name() == filepath.Base(oldPath) {
					selectedIdx = i
				}
			}
		} else {
			handleDirectoryChange(currPath, config)
			selectedIdx = idxFromHistory
		}

	case 'l':
		if selectedIdx < len(files) && files[selectedIdx].IsDir() {
			parentScrollOffset = scrollOffset
			positionHistory[currPath] = selectedIdx

			currPath = filepath.Join(currPath, files[selectedIdx].Name())
			handleDirectoryChange(currPath, config)

			if idxFromHistory, ok := positionHistory[currPath]; ok {
				selectedIdx = idxFromHistory
			} else {
				selectedIdx = 0
			}
		}

	case '.':
		config.ShowHiddenFiles = !config.ShowHiddenFiles
		handleDirectoryChange(currPath, config)

	case '/':
		currMode = ModeSearch
		searchBarPrefix = SearchBarPrefixSearching

	case 'm':
		currMode = ModeRecordingMark

	case '\'':
		currMode = ModeListeningForMark

	case 'g':
		if !waitingForAnotherKeyPress {
			waitingForAnotherKeyPress = true
			previousKeyPressed = 'g'
			break
		}

		if previousKeyPressed == 'g' {
			selectedIdx = 0
			scrollOffset = 0
		}

		waitingForAnotherKeyPress = false

	case 'G':
		selectedIdx = len(files) - 1

		_, screenHeight := screen.Size()
		heightUsableForFiles := max(screenHeight-3, 1)
		scrollOffset = max((len(files)-1)-(heightUsableForFiles-1), 0)
	}

	switch ev.Key() {
	case tcell.KeyCtrlD:
		if selectedIdx >= len(files)-1 {
			break
		}

		selectedIdx = min(selectedIdx+BigJumpLength, len(files)-1)
		scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))

	case tcell.KeyCtrlU:
		if selectedIdx <= BigJumpLength {
			selectedIdx = 0
		} else {
			selectedIdx = selectedIdx - BigJumpLength
		}

		scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))
	}
	
	return result, nil
}

func handleKeyPressInSearch(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	switch ev.Key() {
	case tcell.KeyRune:
		currSearchEntry = currSearchEntry + string(ev.Rune())
		matches, err := searchInDir(currSearchEntry, files)
		if err != nil {
			return keyHandlingResult{}, err
		}

		updateFileListing(matches, config)

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
			handleFileListingChange(currDirFiles, config)
		} else {
			currSearchEntry = currSearchEntry[:len(currSearchEntry)-1]
			matches, _ := searchInDir(currSearchEntry, currDirFiles)
			updateFileListing(matches, config)
		}

	case tcell.KeyCR:
		if currSearchEntry == "" {
			currDirFiles, err := os.ReadDir(currPath)
			if err != nil {
				log.Fatalf("Failed to read path %q", currPath)
			}

			handleFileListingChange(currDirFiles, config)
		}

		if len(files) == 0 {
			handleDirectoryChange(currPath, config)
		}

		currMode = ModeDefault
		currSearchEntry = ""

		// The user is now done with searching. Set the marker to point to the
		// first entry.
		selectedIdx = 0
		scrollOffset = 0
		searchBarPrefix = SearchBarPrefixSearched

	case tcell.KeyESC:
		// Disable search mode and ignore the current search string. This is
		// consistent with how searching work in Vim.
		currMode = ModeDefault
		currSearchEntry = ""
		searchBarPrefix = SearchBarPrefixNavigating
		handleDirectoryChange(currPath, config)

	case tcell.KeyTAB:
		if len(files) == 0 {
			break
		}

		firstMatch := files[0]
		if firstMatch.IsDir() {
			if selectedIdx < len(files) && files[selectedIdx].IsDir() {
				positionHistory[currPath] = selectedIdx

				currPath = filepath.Join(currPath, files[selectedIdx].Name())
				handleDirectoryChange(currPath, config)

				if idxFromHistory, ok := positionHistory[currPath]; ok {
					selectedIdx = idxFromHistory
				} else {
					selectedIdx = 0
				}
			}
		}

		if currSearchEntry == "" {
			searchBarPrefix = SearchBarPrefixNavigating
		} else {
			searchBarPrefix = SearchBarPrefixSearching
		}

		currSearchEntry = ""

	case tcell.KeyBacktab:
		parentPath := filepath.Dir(filepath.Clean(currPath))
		if currPath != parentPath {
			currPath = parentPath
		}

		handleDirectoryChange(currPath, config)
		selectedIdx = 0
		scrollOffset = 0
		currSearchEntry = ""
	}

	return keyHandlingResult{shouldQuit: false, newPath: ""}, nil
}

func storeNewMark(r rune, path string, config *conf.Config) error {
	// BUG: Check if there's a mark for this rune already.

	f, err := os.OpenFile(config.MarkFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	line := fmt.Sprintf("%c %s\n", r, path)
	_, err = f.WriteString(line)
	if err != nil {
		return err
	}

	return nil
}

func readMarks(config *conf.Config) (map[rune]string, error) {
	result := make(map[rune]string)

	f, err := os.Open(config.MarkFilePath)
	if err != nil {
		return result, err
	}
	defer f.Close()

	lineIdx := 0
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			msg := fmt.Sprintf("reading marks: line %v contains less than two components", lineIdx+1)
			return result, errors.New(msg)
		}

		runes := []rune(parts[0])
		if len(runes) != 1 {
			msg := fmt.Sprintf("reading marks: %v is not a valid rune", parts[0])
			return result, errors.New(msg)
		}

		r := runes[0]
		path := parts[1]
		result[r] = path

		lineIdx++
	}

	err = scanner.Err()

	return result, err
}

func calculateScrollOffset(screen tcell.Screen, selectedIdx, currScrollOffset, listLen int) int {
	_, screenHeight := screen.Size()
	heightUsableForFiles := max(screenHeight-3, 1)

	return calculateScrollOffsetForHeight(selectedIdx, scrollOffset, heightUsableForFiles, len(files))
}

// BUG: Wrapping is buggy right now. Try wrapping in a directory with a lot of files.
func calculateScrollOffsetForHeight(selectedIdx, currScrollOffset, heightUsableForFiles, listLen int) int {
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

	maxOffset := max((listLen-1)-(heightUsableForFiles-1), 0)

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

func render(keyChangesChan chan *tcell.EventKey, errorChan chan error, config *conf.Config) {
	for {
		select {
		case eventKey := <-keyChangesChan:
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
			} else if currMode == ModeListeningForMark {
				drawFileList(screen, config)
				drawMarkHintSection(screen, config)
				screen.Show()
			}

		case err := <-errorChan:
			drawErrorLine(screen, err)
			screen.Show()
		}
	}
}

func canKeyPressesBeChained(key1, key2 rune) bool {
	chainableWithKey1, ok := ChainableKeybindings[key1]
	if !ok {
		return false
	}
	
	return slices.Contains(chainableWithKey1, key2)
}
