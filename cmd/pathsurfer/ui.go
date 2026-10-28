package main

import (
	"bufio"
	"log/slog"
	"path/filepath"
	"strings"
	"fmt"
	"io/fs"
	
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"

	"github.com/bnuredini/pathsurfer/internal/conf"
)

var (
	StylePathIndicator       = tcell.StyleDefault.Foreground(color.White)
	StyleActivePathIndicator = tcell.StyleDefault.Foreground(color.Blue)
	StyleReset               = tcell.StyleDefault.Background(color.Reset).Foreground(color.Reset)
	StyleError               = tcell.StyleDefault.Foreground(color.White).Background(color.DarkRed)
	StyleInfo                = tcell.StyleDefault.Foreground(color.Yellow)
	StyleAttention           = tcell.StyleDefault.Background(color.SteelBlue).Foreground(color.White)
	StyleSelectedEntry       = tcell.StyleDefault.Background(color.DarkBlue).Foreground(color.White)
	StyleDirEntry            = tcell.StyleDefault.Foreground(color.Green)
)

func render(m model, config *conf.Config) {
	switch m.currMode {
	case ModeDefault:
		renderForDefaultMode(m, screen, config)

	case ModeSearch:
		drawFileList(m, screen, config)
		drawShortHelpLine(screen)

	case ModeRecordingMark:
		drawFileList(m, screen, config)
		drawStatusLine(screen, "Listening for mark...", StyleAttention)

	case ModeListeningForMark:
		drawFileList(m, screen, config)
		drawMarkHintSection(m, screen)
	}

	if m.globalErr != nil {
		drawStatusLine(screen, m.globalErr.Error(), StyleError)
	}

	screen.SetTitle(fmt.Sprintf("pathsurfer - %v", m.currPath))
	screen.Show()
}

func renderForDefaultMode(m model, screen tcell.Screen, config *conf.Config) {
	drawFileList(m, screen, config)

	if m.waitingForAnotherKeyPress {
		keybindings, ok := ChainableKeybindings[m.previousKeyPressed]
		if ok {
			drawHintSection(screen, keybindings)
		} else {
			drawShortHelpLine(screen)
		}

		return
	} else if m.shouldDisplayHelpSection {
		drawHelpSection(screen)
		return
	}

	drawShortHelpLine(screen)
}

func drawFileList(m model, screen tcell.Screen, config *conf.Config) {
	screen.Clear()
	
	w, _ := screen.Size()
	{
		dimensions := v4{x1: 0, y1: 0, x2: w, y2: 0}
		text := fmt.Sprintf("%s/%s", m.currPath, m.currSearchEntry)
		if m.searchBarPrefix != "" {
			text = fmt.Sprintf("%s: %s", m.searchBarPrefix, text)
		}

		switch m.currMode {
		case ModeSearch:
			drawText(screen, dimensions, StyleActivePathIndicator, text)
			screen.ShowCursor(dimensions.x1+len(text), dimensions.y1)
			screen.SetCursorStyle(tcell.CursorStyleBlinkingBlock)
		default:
			drawText(screen, dimensions, StylePathIndicator, text)
			screen.HideCursor()
		}
	}

	drawParentPane(m, screen, config)
	drawMainPane(m, screen)
	drawChildPane(m, screen, config)
}

func drawParentPane(m model, screen tcell.Screen, config *conf.Config) {
	parentSelectedIdx := 0
	parentFiles := []fs.DirEntry{}
	parentDir := filepath.Dir(m.currPath)

	if strings.TrimSpace(parentDir) != "" || parentDir != "/" {
		parentFiles = getDirEntries(filepath.Dir(m.currPath), config)

		for i, f := range parentFiles {
			if f.Name() == filepath.Base(m.currPath) {
				parentSelectedIdx = i
			}
		}
	}
	
	w, h := screen.Size()
	m.parentScrollOffset = calculateScrollOffsetForHeight(
		parentSelectedIdx,
		m.parentScrollOffset,
		max(h-2, 1),
		len(parentFiles),
	)
	dimensions := v4{x1: 0, y1: 1, x2: w / 6, y2: h - 1}
	
	drawPane(
		screen,
		pane{
			entries:        parentFiles,
			dimensions:     dimensions,
			selectedMarker: parentSelectedIdx,
			scrollMarker:   m.parentScrollOffset,
			isActive:       false,
		},
	)
}

func drawMainPane(m model, screen tcell.Screen) {
	w, h := screen.Size()
	secondaryPaneWidth := w / 6
	dimensions := v4{
		x1: secondaryPaneWidth + 2,
		y1: 1,
		x2: secondaryPaneWidth + (3 * secondaryPaneWidth),
		y2: h - 1,
	}
	
	drawPane(
		screen,
		pane{
			entries:        m.files,
			dimensions:     dimensions,
			selectedMarker: m.selectedIdx,
			scrollMarker:   m.scrollOffset,
			isActive:       true,
			displayMessageIfEmpty: true,
		},
	)
}

func drawChildPane(m model, screen tcell.Screen, config *conf.Config) {
	childFiles := []fs.DirEntry{}
	if m.selectedIdx < len(m.files) && m.files[m.selectedIdx].IsDir() {
		childDir := filepath.Join(m.currPath, m.files[m.selectedIdx].Name())
		childFiles = getDirEntries(childDir, config)
	}
	
	w, h := screen.Size()
	dimensions := v4{
		x1: ((4*w) / 6),
		y1: 1,
		x2: w,
		y2: h - 1,
	}
	
	drawPane(
		screen,
		pane{
			entries:        childFiles,
			dimensions:     dimensions,
			selectedMarker: 0,
			scrollMarker:   0,
			isActive:       false,
		},
	)
}

type pane struct {
	entries        []fs.DirEntry
	dimensions     v4
	selectedMarker int
	scrollMarker   int
	isActive       bool
	displayMessageIfEmpty bool
}

func drawPane(screen tcell.Screen, p pane) {
	if len(p.entries) == 0 && p.displayMessageIfEmpty {
		drawText(
			screen,
			v4{x1: p.dimensions.x1, y1: p.dimensions.y1, x2: p.dimensions.x2, y2: p.dimensions.y1},
			StyleInfo,
			"No entries",
		)
		
		return
	}
	
	heightUsableForFiles := p.dimensions.y2 - p.dimensions.y1

	for i := range heightUsableForFiles {
		fileIdx := p.scrollMarker + i
		if fileIdx >= len(p.entries) {
			break
		}

		prefix := "  "
		file := p.entries[fileIdx]
		calculatedStyle := tcell.StyleDefault

		if file.IsDir() {
			prefix = "📁 "
			calculatedStyle = StyleDirEntry
		}

		if fileIdx == p.selectedMarker {
			calculatedStyle = StyleSelectedEntry
		}

		if !p.isActive {
			calculatedStyle = calculatedStyle.Dim(true)
		}

		drawText(
			screen,
			v4{x1: p.dimensions.x1, y1: p.dimensions.y1 + i, x2: p.dimensions.x2, y2: p.dimensions.y1 + i},
			calculatedStyle,
			fmt.Sprintf("%s%s", prefix, file.Name()),
		)
	}
}

func drawMarkHintSection(m model, screen tcell.Screen) {
	w, h := screen.Size()

	if len(m.marks) == 0 {
		drawText(
			screen,
			v4{0, (h - 1), w, (h - 1)},
			StyleInfo,
			"No bookmarks set!",
		)

		return
	}

	drawFullLine(screen, (h-2)-len(m.marks), "Bookmarks", StyleInfo)
	drawFullLine(screen, (h-1)-len(m.marks), "", StyleInfo)

	index := len(m.marks) - 1
	for entry, value := range m.marks {
		drawFullLine(
			screen,
			(h-1)-index,
			fmt.Sprintf("%c\t%s", entry, value),
			StyleInfo,
		)

		index--
	}
}

func drawHintSection(screen tcell.Screen, keybindings []keybinding) {
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

	for currCol < dimensions.x2 {
		screen.SetContent(currCol, currRow, ' ', nil, style)
		currCol++
	}

	for currCol < dimensions.x2 {
		screen.SetContent(currCol, currRow, ' ', nil, style)
		currCol++
	}
}

func drawShortHelpLine(screen tcell.Screen) {
	_, h := screen.Size()
	drawFullLine(screen, h-1, HelpMessageShort, StyleInfo)
}

func drawStatusLine(screen tcell.Screen, text string, style tcell.Style) {
	_, h := screen.Size()
	drawFullLine(screen, h-1, text, style)
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
}
