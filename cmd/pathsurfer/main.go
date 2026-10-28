package main

import (
	"bufio"
	"errors"
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
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"

	"github.com/bnuredini/pathsurfer/internal/conf"
	"github.com/bnuredini/pathsurfer/internal/fuzzy"
	"github.com/bnuredini/pathsurfer/internal/stringutil"
	"github.com/bnuredini/pathsurfer/internal/wizard"
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
	SearchBarPrefixSearching = "searching"
	SearchBarPrefixSearched  = "searched"
)

var screen tcell.Screen

type model struct {
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
	marks                     map[rune]string
	shouldDisplayHelpSection  bool
	globalErr                 error
	shouldQuit                bool
}

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
	if err := handleSubcommands(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config, err := conf.Init()
	if err != nil {
		log.Fatalf("Failed to boot up: %v", err)
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

	screen.SetTitle("pathsurfer")
	screen.SetStyle(StyleReset)
	screen.Clear()
	defer screen.Fini()

	pathToPrint := ""
	m := initModel(config)
	render(m, config)

	running := true
	for running {
		ev := <-screen.EventQ()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			drawFileList(m, screen, config)
			drawShortHelpLine(screen)
			screen.Show()

			render(m, config)

		case *tcell.EventKey:
			slog.Debug("main loop", "keyRune", ev.Str(), "keyString", string(ev.Str()), "currMode", m.currMode, "positionHistory", m.positionHistory)

			var err error
			m, err = handleKeyPress(m, ev, config)
			if err != nil {
				m.globalErr = err
			}

			if m.shouldQuit {
				pathToPrint = m.currPath
				running = false
				break
			}

			render(m, config)
		}
	}

	// Assuming that the user is using one of the wrapper scripts (psurf.sh or
	// psurf.fish), this program will print the current directory when the user
	// breaks from the event loop. In which case the wrapper will change the
	// shell's directory to what gets printed here.
	if pathToPrint != "" {
		fmt.Println(pathToPrint)
	}
}

func initModel(config *conf.Config) model {
	m := model{}
	m.positionHistory = make(map[string]int)

	pathProvided := len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "+") 
	if pathProvided {
		m.currPath = os.Args[1]
	} else {
		var err error
		m.currPath, err = os.Getwd()
		if err != nil {
			slog.Info("Couldn't get current directory", "err", err)
			os.Exit(1)
		}
	}

	var err error
	m.marks, err = readMarks(config)
	if err != nil {
		m.globalErr = err
		slog.Info("Error whilst reading marks", "err", err)
		m.marks = map[rune]string{}
	}
	m = handleDirectoryChange(m, config)

	return m
}

func handleDirectoryChange(m model, config *conf.Config) model {
	dir, err := os.ReadDir(m.currPath)
	if err != nil {
		if os.IsPermission(err) {
			slog.Error("Encountered a permissions issue when updating the file listing", "err", err)
		}

		slog.Error("Couldn't read directory", "currPath", m.currPath, "err", err)
		m.files = []fs.DirEntry{}
		m.selectedIdx = 0
		m.scrollOffset = 0

		return m // TODO: Return an error here.
	}

	m = handleFileListingChange(m, dir, config)

	return m
}

// CLEANUP: This function updates too many state variables. Ideally, it should
// only touch the file listings.
func handleFileListingChange(m model, rawFiles []fs.DirEntry, config *conf.Config) model {
	sort.Slice(rawFiles, func(i, j int) bool {
		return rawFiles[i].Name() < rawFiles[j].Name()
	})

	m = updateFileListing(m, rawFiles, config)

	if len(m.files) == 0 {
		m.selectedIdx = 0
	} else if m.selectedIdx >= len(m.files) {
		m.selectedIdx = len(m.files) - 1
	}

	m.scrollOffset = calculateScrollOffset(m, screen, m.selectedIdx)

	return m
}

func updateFileListing(m model, rawFiles []fs.DirEntry, config *conf.Config) model {
	if config.ShowHiddenFiles {
		m.files = rawFiles
		return m
	}

	m.files = []fs.DirEntry{}

	for _, f := range rawFiles {
		if !strings.HasPrefix(f.Name(), ".") {
			m.files = append(m.files, f)
		}
	}

	return m
}

func updateFileListingWithSearch(m model, files []fs.DirEntry, config *conf.Config) model {
	matches, _ := searchInDir(m.currSearchEntry, files)
	return updateFileListing(m, matches, config)
}

func handleKeyPress(m model, ev *tcell.EventKey, config *conf.Config) (model, error) {
	if ev.Key() == tcell.KeyCtrlC {
		return m, nil
	}

	var err error
	switch m.currMode {
	case ModeDefault:
		m, err = handleKeyPressInDefault(m, ev, config)
	case ModeSearch:
		m, err = handleKeyPressInSearch(m, ev, config)
	case ModeRecordingMark:
		m, err = handleKeyPressInRecordingMark(m, ev, config)
	case ModeListeningForMark:
		m, err = handleKeyPressInListeningForMark(m, ev, config)
	}

	return m, err
}

func handleKeyPressInDefault(m model, ev *tcell.EventKey, config *conf.Config) (model, error) {
	if m.waitingForAnotherKeyPress && !canKeyPressesBeChained(m.previousKeyPressed, ev.Str()) {
		// CLEANUP: Find a better reset value. With this reset value and with
		// the fact that we're using ev.Rune(), users can't use the spacebar for
		// chainable keybindings.
		m.previousKeyPressed = " "
		m.waitingForAnotherKeyPress = false
	}

	if m.searchBarPrefix == SearchBarPrefixSearched {
		m.searchBarPrefix = ""
	}

	switch ev.Str() {
	case "q":
		m.shouldQuit = true
		return m, nil

	case "j":
		m = handleKeyPressDown(m)

	case "k":
		m = handleKeyPressUp(m)

	case "h":
		m = handleKeyPressLeft(m, config)

	case "l":
		m = handleKeyPressRight(m, config)

	case ".":
		config.ShowHiddenFiles = !config.ShowHiddenFiles
		m = handleDirectoryChange(m, config)

	case "/":
		m.currMode = ModeSearch
		m.searchBarPrefix = SearchBarPrefixSearching

	case "m":
		m.currMode = ModeRecordingMark

	case "'":
		m.currMode = ModeListeningForMark

	case "g":
		if !m.waitingForAnotherKeyPress {
			m.waitingForAnotherKeyPress = true
			m.previousKeyPressed = "g"
			break
		}

		if m.previousKeyPressed == "g" {
			m.selectedIdx = 0
			m.scrollOffset = 0
		}

		m.waitingForAnotherKeyPress = false

	case "c":
		if !m.waitingForAnotherKeyPress {
			m.waitingForAnotherKeyPress = true
			m.previousKeyPressed = "c"
			break
		}

		if m.previousKeyPressed == "c" && m.selectedIdx > 0 && m.selectedIdx < len(m.files) {
			s := filepath.Join(m.currPath, m.files[m.selectedIdx].Name())
			screen.SetClipboard([]byte(s))
		}

		m.waitingForAnotherKeyPress = false

	case "G":
		if len(m.files) == 0 {
			break
		}

		m.selectedIdx = len(m.files) - 1

		_, screenHeight := screen.Size()
		heightUsableForFiles := max(screenHeight-3, 1)
		m.scrollOffset = max((len(m.files)-1)-(heightUsableForFiles-1), 0)

	case "n":
		if m.previousKeyPressed == "c" && m.selectedIdx < len(m.files) {
			s := m.files[m.selectedIdx].Name()
			screen.SetClipboard([]byte(s))
		}

		m.waitingForAnotherKeyPress = false

	case "d":
		if m.previousKeyPressed == "c" && m.selectedIdx < len(m.files) {
			s := filepath.Dir(m.currPath)
			screen.SetClipboard([]byte(s))
		}

		m.waitingForAnotherKeyPress = false

	case "?":
		m.shouldDisplayHelpSection = true
	}

	switch ev.Key() {
	case tcell.KeyUp:
		m = handleKeyPressUp(m)

	case tcell.KeyDown:
		m = handleKeyPressDown(m)

	case tcell.KeyLeft:
		m = handleKeyPressLeft(m, config)

	case tcell.KeyRight:
		m = handleKeyPressRight(m, config)

	case tcell.KeyCtrlD:
		if m.selectedIdx >= len(m.files)-1 {
			break
		}

		m.selectedIdx = min(m.selectedIdx+config.BigJumpLength, len(m.files)-1)
		m.scrollOffset = calculateScrollOffset(m, screen, m.selectedIdx)

	case tcell.KeyCtrlU:
		if m.selectedIdx <= config.BigJumpLength {
			m.selectedIdx = 0
		} else {
			m.selectedIdx = m.selectedIdx - config.BigJumpLength
		}

		m.scrollOffset = calculateScrollOffset(m, screen, m.selectedIdx)

	case tcell.KeyESC:
		if m.currSearchEntry != "" {
			m.currSearchEntry = ""
			m.searchBarPrefix = ""
			m = handleDirectoryChange(m, config)
		}

		m.shouldDisplayHelpSection = false
	}

	return m, nil
}

func handleKeyPressInSearch(m model, ev *tcell.EventKey, config *conf.Config) (model, error) {
	switch ev.Key() {
	case tcell.KeyRune:
		if ev.Str() == "/" {
			return m, nil
		}

		// When in search mode, make sure the marker is at the top of the list.
		// Since the marker should be at the top, the pane should be drawn as if
		// both the selected index and the scroll offset are 0.
		if strings.TrimSpace(m.currSearchEntry) == "" {
			m.selectedIdx = 0
			m.scrollOffset = 0
		}

		m.currSearchEntry = m.currSearchEntry + string(ev.Str())
		matches, err := searchInDir(m.currSearchEntry, m.files)
		if err != nil {
			return m, err
		}

		m = updateFileListing(m, matches, config)

	case tcell.KeyBackspace, 127:
		if len(m.currSearchEntry) == 0 {
			break
		}

		currDirFiles, err := os.ReadDir(m.currPath)
		if err != nil {
			return m, err
		}

		if len(m.currSearchEntry) == 1 {
			m.currSearchEntry = ""
			m = handleFileListingChange(m, currDirFiles, config)
		} else if ev.Modifiers()&tcell.ModAlt != 0 {
			m.currSearchEntry = stringutil.DeletePreviousWord(m.currSearchEntry)
			if m.currSearchEntry == "" {
				m = handleFileListingChange(m, currDirFiles, config)
			} else {
				m = updateFileListingWithSearch(m, currDirFiles, config)
			}
		} else {
			m.currSearchEntry = m.currSearchEntry[:len(m.currSearchEntry)-1]
			m = updateFileListingWithSearch(m, currDirFiles, config)
		}

	case tcell.KeyCR:
		m.currMode = ModeDefault

		if strings.TrimSpace(m.currSearchEntry) == "" {
			m = handleDirectoryChange(m, config)
			break
		}

		// User is now done searching. Set the marker to point to the first entry.
		m.selectedIdx = 0
		m.scrollOffset = 0
		m.searchBarPrefix = SearchBarPrefixSearched

	case tcell.KeyESC:
		// Disable search mode and ignore the current search string. This is
		// consistent with how searching work in Vim.
		m.currMode = ModeDefault
		m.currSearchEntry = ""
		m.searchBarPrefix = ""
		m = handleDirectoryChange(m, config)

	case tcell.KeyTAB:
		if len(m.files) == 0 {
			break
		}

		firstMatch := m.files[0]
		if firstMatch.IsDir() {
			index, indexFound := getEntryIndexFromPath(m, m.currPath, firstMatch.Name(), config)
			if !indexFound {
				// TODO: Log error in screen.
				break
			}

			m.selectedIdx = index
			m.positionHistory[m.currPath] = index
			newPath := filepath.Join(m.currPath, firstMatch.Name())
			m.currPath = newPath
			m = handleDirectoryChange(m, config)
		}

		m.searchBarPrefix = SearchBarPrefixSearching
		m.currSearchEntry = ""

	case tcell.KeyBacktab:
		m = handleKeyPressLeft(m, config)
	}

	return m, nil
}

func handleKeyPressInRecordingMark(m model, ev *tcell.EventKey, config *conf.Config) (model, error) {
	m.currMode = ModeDefault

	if ev.Key() == tcell.KeyESC {
		return m, nil
	}

	if ev.Key() != tcell.KeyRune {
		return m, errors.New("setting mark: value for mark must be a rune")
	}

	r, _ := utf8.DecodeRuneInString(ev.Str())
	err := storeNewMark(r, m.currPath, config)
	if err != nil {
		return m, err
	}

	m.marks, err = readMarks(config)

	return m, err
}

func handleKeyPressInListeningForMark(m model, ev *tcell.EventKey, config *conf.Config) (model, error) {
	m.currMode = ModeDefault

	if ev.Key() == tcell.KeyESC {
		return m, nil
	}

	if ev.Key() != tcell.KeyRune {
		return m, errors.New("listening for mark: value for mark must be a rune")
	}

	r, _ := utf8.DecodeRuneInString(ev.Str())
	path, ok := m.marks[r]
	if !ok {
		return m, fmt.Errorf("listening for mark: %q is not set", r)
	}

	f, err := os.Open(path)
	if err != nil {
		return m, fmt.Errorf("%q is not a valid file", path)
	}
	defer f.Close()

	m.currPath = path
	m = handleDirectoryChange(m, config)
	m.selectedIdx = 0
	m.scrollOffset = 0
	m.currSearchEntry = ""

	return m, nil
}

func handleKeyPressDown(m model) model {
	if len(m.files) == 0 {
		return m
	}

	m.selectedIdx = (m.selectedIdx + 1) % len(m.files)
	m.scrollOffset = calculateScrollOffset(m, screen, m.selectedIdx)

	return m
}

func handleKeyPressUp(m model) model {
	if len(m.files) == 0 {
		return m
	}

	m.selectedIdx = (m.selectedIdx - 1 + len(m.files)) % len(m.files)
	m.scrollOffset = calculateScrollOffset(m, screen, m.selectedIdx)

	return m
}

func handleKeyPressLeft(m model, config *conf.Config) model {
	m.positionHistory[m.currPath] = m.selectedIdx

	oldPath := m.currPath
	newPath := filepath.Dir(m.currPath)
	m.currPath = newPath
	m.currSearchEntry = ""

	indexFromHistory, ok := m.positionHistory[newPath]
	if !ok {
		for i, f := range m.files {
			if f.Name() == filepath.Base(oldPath) {
				m.selectedIdx = i
			}
		}
	} else {
		m.selectedIdx = indexFromHistory
	}

	m = handleDirectoryChange(m, config)

	return m
}

func handleKeyPressRight(m model, config *conf.Config) model {
	m.currSearchEntry = ""

	if m.selectedIdx < len(m.files) && m.files[m.selectedIdx].IsDir() {
		m.parentScrollOffset = m.scrollOffset
		m.positionHistory[m.currPath] = m.selectedIdx

		m.currPath = filepath.Join(m.currPath, m.files[m.selectedIdx].Name())
		m = handleDirectoryChange(m, config)

		if indexFromHistory, ok := m.positionHistory[m.currPath]; ok {
			m.selectedIdx = indexFromHistory
		} else {
			m.selectedIdx = 0
		}
	}

	return m
}

// TODO: Reduce parameter count.
func calculateScrollOffset(m model, screen tcell.Screen, selectedIdx int) int {
	_, screenHeight := screen.Size()
	heightUsableForFiles := max(screenHeight-3, 1)

	return calculateScrollOffsetForHeight(selectedIdx, m.scrollOffset, heightUsableForFiles, len(m.files))
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

// TODO: Reduce parameter count.
func getEntryIndexFromPath(m model, path, entryToLookFor string, config *conf.Config) (int, bool) {
	pathEntries, err := os.ReadDir(path)
	if err != nil {
		slog.Debug("failed to read directory entries", "err", err, "currPath", m.currPath)
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

func getDirEntries(path string, config *conf.Config) []fs.DirEntry {
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
			slog.Debug("Found invalid line in mark file", "line", l)
			continue
		}

		if string(r) == parts[0] {
			return fmt.Errorf("recording mark: rune %q is already used", r)
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
		if len(parts) > 2 {
			result[r] = strings.Join(parts[1:], " ")
		} else {
			result[r] = parts[1]
		}

		lineIdx++
	}

	err = scanner.Err()

	return result, err
}

func handleSubcommands() error {
	if len(os.Args[1:]) <= 0 {
		return nil
	}

	arg := strings.TrimSpace(os.Args[1])
	switch arg {
	case "+wizard":
		return wizard.Start()
	default:
		return nil
	}

	pathDirInfo, err := os.Stat(arg)
	if os.IsNotExist(err) {
		log.Fatalf("%q does not exist", arg)
	} else if err != nil {
		log.Fatalf("%q is not a valid path: %v", arg, err)
	} else if !pathDirInfo.IsDir() {
		log.Fatalf("%q is not a valid directory", arg)
	}

	return nil
}
