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
	"sort"
	"strings"
	"syscall"

	"github.com/gdamore/tcell/v2"

	"github.com/bnuredini/pathsurfer/internal/conf"
	"github.com/bnuredini/pathsurfer/internal/fuzzy"
	"github.com/bnuredini/pathsurfer/internal/stringutil"
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
	scrollOffset int
	// In the multi-pane view, the scroll offset of the parent directory is also renderd.
	parentScrollOffset int
	selectedIdx        int
	searchBarPrefix    SearchBarPrefix

	// It makes sense to use this value only if waitingForAnotherKeyPress is
	// true. The purpose of these to variables is to add support for Vi-like
	// keybindings such as gg.
	previousKeyPressed        string
	waitingForAnotherKeyPress bool

	marks map[rune]string

	shouldDisplayHelpSection bool
)

var (
	StylePathIndicator       = tcell.StyleDefault.Foreground(tcell.ColorGray)
	StyleActivePathIndicator = tcell.StyleDefault.Foreground(tcell.ColorBlue)
	StyleReset               = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	StyleError               = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkRed)
	StyleInfo                = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	StyleAttention           = tcell.StyleDefault.Background(tcell.ColorSteelBlue).Foreground(tcell.ColorWhite)
	StyleSelectedEntry       = tcell.StyleDefault.Background(tcell.ColorDarkBlue).Foreground(tcell.ColorWhite)
)

var RunesThatTriggerRedrawInDefault = []rune{
	'h',
	'j',
	'k',
	'l',
	'.',
	'\'',
}

var KeysThatTriggerRedrawInDefault = []tcell.Key{
	tcell.KeyBackspace,
	tcell.KeyCR,
	tcell.KeyTAB,
	tcell.KeyESC,
}

type keybinding struct {
	key         string
	description string
}

var ChainableKeybindings = map[string][]keybinding{
	"g": []keybinding{
		keybinding{key: "g", description: "Go to top"},
	},
	"c": []keybinding{
		keybinding{key: "c", description: "Copy file path"},
		keybinding{key: "n", description: "Copy file name"},
		keybinding{key: "d", description: "Copy current directory path"},
	},
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
		if err = os.Mkdir(logDir, 0755); err != nil {
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

	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("Application panicked", "panic", r)
		}

		if logFile != nil {
			slog.Debug("Application shutting down. Closing log file...")

			if closeErr := logFile.Close(); closeErr != nil {
				log.Fatalf("Failed to close log file: %v", closeErr)
			}
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan

		slog.Debug("Received interrupt signal. Exiting...")
		screen.Fini()
		if logFile != nil {
			_ = logFile.Close()
		}
		os.Exit(0)
	}()

	screen, err = tcell.NewScreen()
	if err != nil {
		slog.Error("Couldn't create screen", "err", err)
		os.Exit(1)
	}
	if err := screen.Init(); err != nil {
		slog.Error("Couldn't initialize screen", "err", err)
		os.Exit(1)
	}

	screen.SetStyle(StyleReset)
	screen.Clear()

	pathToPrint := ""
	positionHistory = make(map[string]int)
	searchBarPrefix = SearchBarPrefixNavigating

	if strings.TrimSpace(currPath) == "" {
		currPath, err = os.Getwd()
		if err != nil {
			slog.Info("Couldn't get current directory", "err", err)
			os.Exit(1)
		}
	}

	marks, err = readMarks(config)
	if err != nil {
		marks = map[rune]string{}
	}
	handleDirectoryChange(currPath, config)
	drawFileList(screen, config)
	drawShortInfoLine(screen)

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
			drawShortInfoLine(screen)
			screen.Show()

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
	// psurf.fish), this program will print the current directory when the user
	// breaks from the event loop. In which case the wrapper will change the
	// shell's directory to what gets printed here.
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
		slog.Debug("Failed to read directory", "path", path, "err", err)
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

func updateFileListing(rawFiles []fs.DirEntry, config *conf.Config) {
	if config.ShowHiddenFiles {
		files = rawFiles
		return
	} 
	
	files = []fs.DirEntry{}

	for _, f := range rawFiles {
		if !strings.HasPrefix(f.Name(), ".") {
			files = append(files, f)
		}
	}
}

func updateFileListingWithSearch(pattern string, files []fs.DirEntry, config *conf.Config, ) {
	matches, _ := searchInDir(currSearchEntry, files)
	updateFileListing(matches, config)
}

func handleDirectoryChange(path string, config *conf.Config) {
	dir, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			slog.Error("Encountered a permissions issue when updating the file listing", "err", err)
		}

		slog.Error("Couldn't read directory", "currPath", currPath, "err", err)
		files = []fs.DirEntry{}
		selectedIdx = 0
		scrollOffset = 0

		return // TODO: Return an error here.
	}

	handleFileListingChange(dir, config)
}

// CLEANUP: This function updates too many state variables. Ideally, it should
// only touch the file listings.
func handleFileListingChange(rawFiles []fs.DirEntry, config *conf.Config) {
	sort.Slice(rawFiles, func(i, j int) bool {
		return rawFiles[i].Name() < rawFiles[j].Name()
	})
	
	updateFileListing(rawFiles, config)

	if len(files) == 0 {
		selectedIdx = 0
	} else if selectedIdx >= len(files) {
		selectedIdx = len(files) - 1
	}

	scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))
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
	text := fmt.Sprintf("%s: %s/%s", searchBarPrefix, currPath, currSearchEntry)
	switch currMode {
	case ModeSearch:
		drawText(screen, dimensions, StyleActivePathIndicator, text)
		screen.ShowCursor(dimensions.x1 + len(text), dimensions.y1)
		screen.SetCursorStyle(tcell.CursorStyleBlinkingBlock)
	default:
		drawText(screen, dimensions, StylePathIndicator, text)
		screen.HideCursor()
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

func drawMarkHintSection(screen tcell.Screen, config *conf.Config) {
	w, h := screen.Size()

	if len(marks) == 0 {
		drawText(
			screen,
			v4{0, (h - 1), w, (h - 1)},
			StyleInfo,
			"No bookmarks set",
		)
	}

	drawText(screen, v4{0, (h - 1) - len(marks), w, (h - 1) - len(marks)}, StyleInfo, "Bookmarks")

	index := len(marks) - 1
	for entry, value := range marks {
		dimensions := v4{0, (h - 1) - index, w, (h - 1) - index}
		drawText(
			screen,
			dimensions,
			StyleInfo,
			fmt.Sprintf("%c\t%s", entry, value),
		)

		index--
	}
}

func drawHintSection(screen tcell.Screen, config *conf.Config, keybindings []keybinding) {
	w, h := screen.Size()

	index := len(keybindings) - 1
	for _, k := range keybindings {
		dimensions := v4{0, (h - 1) - index, w, (h - 1) - index}
		drawText(
			screen,
			dimensions,
			StyleInfo,
			fmt.Sprintf("%s\t->\t%s", k.key, k.description),
		)

		index--
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

func drawShortInfoLine(screen tcell.Screen) {
	drawStatusLine(screen, HelpMessageShort, StyleInfo)
}

func drawStatusLine(screen tcell.Screen, text string, style tcell.Style) {
	_, h := screen.Size()
	drawFullLine(screen, h, text, style)
}

func drawFullLine(screen tcell.Screen, y int, text string, style tcell.Style) {
	w, _ := screen.Size()
	col := 0
	for _, r := range text {
		screen.SetContent(col, y, r, nil, style)

		if col >= w-1 {
			break
		}
		col++
	}

	for col < w {
		screen.SetContent(col, y, ' ', nil, style)
		col++
	}
}

func drawHelpSection(screen tcell.Screen) {
	// TODO: Find new lines and use that for the index.
	_, h := screen.Size()
	numLines := strings.Count(HelpMessage, "\n")
	if len(HelpMessage) > 0 && !strings.HasSuffix(HelpMessage, "\n") {
		numLines++
	}
	y := h - numLines
	scanner := bufio.NewScanner(strings.NewReader(HelpMessage))

	for scanner.Scan() {
		l := scanner.Text()
		drawFullLine(screen, y, l, StyleInfo)

		y++
		if y >= h {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("encountered an error while reading help message", "err", err)
	}
	/*
	 */
}

type keyHandlingResult struct {
	shouldQuit    bool
	addingNewMark bool
	newPath       string
}

func handleKeyPress(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{}

	// Some terminals deliver Ctrl+C as \x03. Code point 3 is the ASCII ETX
	// control character.
	if ev.Key() == tcell.KeyCtrlC || ev.Rune() == 3 {
		return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil
	}

	var err error
	switch currMode {
	case ModeDefault:
		result, err = handleKeyPressInDefault(ev, config)
	case ModeSearch:
		result, err = handleKeyPressInSearch(ev, config)
	case ModeRecordingMark:
		result, err = handleKeyPressInRecordingMark(ev, config)
	case ModeListeningForMark:
		result, err = handleKeyPressInListeningForMark(ev, config)
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

func handleKeyPressInDefault(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{}

	if waitingForAnotherKeyPress && !canKeyPressesBeChained(previousKeyPressed, string(ev.Rune())) {
		// CLEANUP: Find a better reset value. With this reset value and with
		// the fact that we're using ev.Rune(), users can't use the spacebar for
		// chainable keybindings.
		previousKeyPressed = " "
		waitingForAnotherKeyPress = false
	}

	if searchBarPrefix == SearchBarPrefixSearched {
		searchBarPrefix = SearchBarPrefixNavigating
	}

	switch ev.Rune() {
	case 'q':
		return keyHandlingResult{shouldQuit: true, newPath: currPath}, nil

	case 'j':
		handleKeyPressDown()

	case 'k':
		handleKeyPressUp()

	case 'h':
		handleKeyPressLeft(config)

	case 'l':
		handleKeyPressRight(config)

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
			previousKeyPressed = "g"
			break
		}

		if previousKeyPressed == "g" {
			selectedIdx = 0
			scrollOffset = 0
		}

		waitingForAnotherKeyPress = false

	case 'c':
		if !waitingForAnotherKeyPress {
			waitingForAnotherKeyPress = true
			previousKeyPressed = "c"
			break
		}

		if previousKeyPressed == "c" {
			s := filepath.Join(currPath, files[selectedIdx].Name())
			writeToClipboard(s)
		}

		waitingForAnotherKeyPress = false

	case 'G':
		selectedIdx = len(files) - 1

		_, screenHeight := screen.Size()
		heightUsableForFiles := max(screenHeight-3, 1)
		scrollOffset = max((len(files)-1)-(heightUsableForFiles-1), 0)

	case 'n':
		if previousKeyPressed == "c" && selectedIdx < len(files) {
			writeToClipboard(files[selectedIdx].Name())
		}
		
	case 'd':
		if previousKeyPressed == "c" && selectedIdx < len(files) {
			writeToClipboard(filepath.Dir(currPath))
		}

	case '?':
		shouldDisplayHelpSection = true
	}

	switch ev.Key() {
	case tcell.KeyUp:
		handleKeyPressUp()

	case tcell.KeyDown:
		handleKeyPressDown()

	case tcell.KeyLeft:
		handleKeyPressLeft(config)

	case tcell.KeyRight:
		handleKeyPressRight(config)

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

	case tcell.KeyESC:
		if currSearchEntry != "" {
			currSearchEntry = ""
			searchBarPrefix = SearchBarPrefixNavigating
			handleDirectoryChange(currPath, config)
		}

		shouldDisplayHelpSection = false
	}

	return result, nil
}

func handleKeyPressInSearch(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	switch ev.Key() {
	case tcell.KeyRune:

		// When in search mode, make sure the marker is at the top of the list.
		// Since the marker should be at the top, the pane should be drawn as if
		// both the selected index and the scroll offset are 0.
		if strings.TrimSpace(currSearchEntry) == "" {
			selectedIdx = 0
			scrollOffset = 0
		}

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
		} else if ev.Modifiers()&tcell.ModAlt != 0 {
			currSearchEntry = stringutil.DeletePreviousWord(currSearchEntry)
			if currSearchEntry == "" {
				handleFileListingChange(currDirFiles, config)
			} else {
				updateFileListingWithSearch(currSearchEntry, currDirFiles, config)
			}
		} else {
			currSearchEntry = currSearchEntry[:len(currSearchEntry)-1]
			updateFileListingWithSearch(currSearchEntry, currDirFiles, config)
		}

	case tcell.KeyCR:
		currMode = ModeDefault

		if strings.TrimSpace(currSearchEntry) == "" {
			handleDirectoryChange(currPath, config)
			break
		}

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
			index, indexFound := getEntryIndexFromPath(currPath, firstMatch.Name(), config)
			if !indexFound {
				// TODO: Log error in screen.
				break
			}

			positionHistory[currPath] = index
			newPath := filepath.Join(currPath, firstMatch.Name())
			handleDirectoryChange(newPath, config)
			currPath = newPath

			if indexFromHistory, ok := positionHistory[currPath]; ok {
				selectedIdx = indexFromHistory
			} else {
				selectedIdx = 0
			}
		}

		if currSearchEntry == "" {
			searchBarPrefix = SearchBarPrefixNavigating
		} else {
			searchBarPrefix = SearchBarPrefixSearching
		}

		currSearchEntry = ""

	case tcell.KeyBacktab:
		handleKeyPressLeft(config)
	}

	return keyHandlingResult{shouldQuit: false, newPath: ""}, nil
}

func handleKeyPressInRecordingMark(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{}
	currMode = ModeDefault

	if ev.Key() == tcell.KeyESC {
		return result, nil
	}

	if ev.Key() != tcell.KeyRune {
		return result, errors.New("setting mark: value for mark must be a rune")
	}

	err := storeNewMark(ev.Rune(), currPath, config)
	if err != nil {
		return result, err
	}

	currMode = ModeDefault
	result.addingNewMark = true

	return result, nil
}

func handleKeyPressInListeningForMark(ev *tcell.EventKey, config *conf.Config) (keyHandlingResult, error) {
	result := keyHandlingResult{}
	currMode = ModeDefault

	if ev.Key() == tcell.KeyESC {
		return result, nil
	}

	if ev.Key() != tcell.KeyRune {
		return result, errors.New("listening for mark: value for mark must be a rune")
	}

	r := ev.Rune()
	path, ok := marks[r]
	if !ok {
		return result, errors.New(fmt.Sprintf("listening for mark: %q is not set", r))
	}

	f, err := os.Open(path)
	if err != nil {
		return result, errors.New(fmt.Sprintf("%q is not a valid file", f.Name()))
	}
	defer f.Close()

	currPath = path
	handleDirectoryChange(currPath, config)
	selectedIdx = 0
	scrollOffset = 0
	currSearchEntry = ""

	return result, nil
}

func handleKeyPressDown() {
	if len(files) == 0 {
		return
	}

	selectedIdx = (selectedIdx + 1) % len(files)
	scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))
}

func handleKeyPressUp() {
	if len(files) == 0 {
		return
	}

	selectedIdx = (selectedIdx - 1 + len(files)) % len(files)
	scrollOffset = calculateScrollOffset(screen, selectedIdx, scrollOffset, len(files))
}

func handleKeyPressLeft(config *conf.Config) {
	positionHistory[currPath] = selectedIdx

	oldPath := currPath
	newPath := filepath.Dir(currPath)
	currPath = newPath
	currSearchEntry = ""

	indexFromHistory, ok := positionHistory[newPath]
	if !ok {
		for i, f := range files {
			if f.Name() == filepath.Base(oldPath) {
				selectedIdx = i
			}
		}
	} else {
		selectedIdx = indexFromHistory
	}

	handleDirectoryChange(currPath, config)
}

func handleKeyPressRight(config *conf.Config) {
	currSearchEntry = ""

	if selectedIdx < len(files) && files[selectedIdx].IsDir() {
		parentScrollOffset = scrollOffset
		positionHistory[currPath] = selectedIdx

		currPath = filepath.Join(currPath, files[selectedIdx].Name())
		handleDirectoryChange(currPath, config)

		if indexFromHistory, ok := positionHistory[currPath]; ok {
			selectedIdx = indexFromHistory
		} else {
			selectedIdx = 0
		}
	}
}

func storeNewMark(r rune, path string, config *conf.Config) error {
	// BUG: Check if there's a mark for this rune already.

	f, err := os.OpenFile(config.MarkFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		parts := strings.Split(l, " ")
		if len(parts) < 2 {
			slog.Debug("Found invalid line in mark file: %v", l)
			continue
		}

		if string(r) == parts[0] {
			return errors.New(fmt.Sprintf("recording mark: rune %q is already used", r))
		}
	}

	if err = scanner.Err(); err != nil {
		return err
	}

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
			slog.Debug("render", "keyRune", eventKey.Rune(), "keyString", string(eventKey.Rune()), "currMode", currMode, "positionHistory", positionHistory)

			switch currMode {
			case ModeDefault:
				renderForDefaultMode(screen, config)

			case ModeSearch:
				drawFileList(screen, config)
				drawShortInfoLine(screen)

			case ModeRecordingMark:
				drawFileList(screen, config)
				drawStatusLine(screen, "Listening for mark...", StyleAttention)

			case ModeListeningForMark:
				drawFileList(screen, config)
				drawMarkHintSection(screen, config)
			}

			screen.Show()

		case err := <-errorChan:
			drawStatusLine(screen, err.Error(), StyleError)
			screen.Show()
		}
	}
}

func renderForDefaultMode(screen tcell.Screen, config *conf.Config) {
	drawFileList(screen, config)

	if waitingForAnotherKeyPress {
		keybindings, ok := ChainableKeybindings[previousKeyPressed]
		if ok {
			drawHintSection(screen, config, keybindings)
		} else {
			drawShortInfoLine(screen)
		}

		return
	} else if shouldDisplayHelpSection {
		drawHelpSection(screen)
		return
	}

	drawShortInfoLine(screen)
}

func canKeyPressesBeChained(key1, key2 string) bool {
	chainableWithKey1, ok := ChainableKeybindings[key1]
	if !ok {
		return false
	}

	for _, k := range chainableWithKey1 {
		if k.key == key2 {
			return true
		}
	}

	return false
}

func getEntryIndexFromPath(path, entryToLookFor string, config *conf.Config) (int, bool) {
	pathEntries, err := os.ReadDir(path)
	if err != nil {
		slog.Debug("failed to read directory entries", "err", err, "currPath", currPath)
		return -1, false
	}

	index := 0
	for _, entry := range pathEntries {
		if !config.ShowHiddenFiles && strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.Name() == entryToLookFor {
			return index, true
		}

		index++
	}

	return -1, false
}
