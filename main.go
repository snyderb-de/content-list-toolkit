package main

import (
	"context"
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type stage int

const (
	stageHome stage = iota
	stagePickSource
	stagePickOutputDir
	stageSetOutput
	stageConfirmOverwrite
	stageScanning
	stageEmailPickSource
	stageEmailPickDest
	stageEmailCopying
	stageDone
	stageFailed
)

const (
	focusFileName = iota
	focusExcludeExts
	focusFoldersOnly
	focusFolderDepth
	focusHashAlgorithm
	focusHidden
	focusSystem
	focusXLSX
	focusPreserveZeros
	focusDeleteCSV
	focusStart
	focusCount
)

type flowMode int

const (
	flowScan flowMode = iota
	flowEmailCopy
)

const introMarkdown = `# Content List Toolkit

Fast recursive folder tools for very large collections.

- Generate a recursive CSV inventory, with optional XLSX export
- Copy email-related files into a chosen destination while preserving folders
- Choose each workflow from the main app

CSV is the safest default for huge scans because the app never builds the whole table in memory.`

type dirItem struct {
	name string
	path string
}

func (d dirItem) FilterValue() string { return d.name }
func (d dirItem) Title() string       { return safeTerminalText(d.name) }
func (d dirItem) Description() string { return safeTerminalText(d.path) }

type actionItem struct {
	title       string
	description string
	flow        flowMode
}

func (a actionItem) FilterValue() string { return a.title }
func (a actionItem) Title() string       { return a.title }
func (a actionItem) Description() string { return a.description }

type scanProgressMsg struct {
	phase            string
	files            uint64
	directories      uint64
	bytes            uint64
	filtered         uint64
	totalFiles       uint64
	totalDirectories uint64
	totalBytes       uint64
	currentItem      string
	percent          float64
	eta              time.Duration
	elapsed          time.Duration
}

type scanErrorMsg struct {
	err error
}

type scanCanceledMsg struct{}

type emailCopyDoneMsg struct {
	sourceDir    string
	destDir      string
	manifestPath string
	copied       uint64
	elapsed      time.Duration
}

type model struct {
	stage          stage
	width          int
	height         int
	list           list.Model
	outputInput    textinput.Model
	excludeInput   textinput.Model
	depthInput     textinput.Model
	settingsFocus  int
	hashAlgorithm  hashAlgorithm
	excludeHidden  bool
	excludeSystem  bool
	createXLSX     bool
	preserveZeros  bool
	deleteCSV      bool
	foldersOnly    bool
	spinner        spinner.Model
	activeFlow     flowMode
	sourceDir      string
	outputDir      string
	outputPath     string
	emailSourceDir string
	emailDestDir   string
	pendingPath    string
	err            error
	done           scanDoneMsg
	emailDone      emailCopyDoneMsg
	progress       scanProgressMsg
	glamourIntro   string
	quitting       bool
	scanStartedAt  time.Time
}

type sourceKeyMap struct {
	Choose key.Binding
	Up     key.Binding
}

var sourceKeys = sourceKeyMap{
	Choose: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open / choose")),
	Up:     key.NewBinding(key.WithKeys("backspace", "h"), key.WithHelp("backspace", "up")),
}

func main() {
	startDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	if hasArg("--gui") || isGUIContext() {
		if err := launchGUI(startDir); err != nil {
			fmt.Fprintf(os.Stderr, "gui error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runTUI(startDir)
}

func runTUI(startDir string) {
	intro := renderMarkdown(introMarkdown, 88)

	outputInput := textinput.New()
	outputInput.Placeholder = "content-list.csv"
	outputInput.Prompt = ""
	outputInput.CharLimit = 0
	outputInput.Width = 80

	excludeInput := textinput.New()
	excludeInput.Placeholder = "tmp,log"
	excludeInput.Prompt = ""
	excludeInput.CharLimit = 0
	excludeInput.Width = 80

	depthInput := textinput.New()
	depthInput.Placeholder = "0 = all levels"
	depthInput.Prompt = ""
	depthInput.CharLimit = 3
	depthInput.Width = 20

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{
		stage:          stageHome,
		list:           newActionList(),
		outputInput:    outputInput,
		excludeInput:   excludeInput,
		depthInput:     depthInput,
		settingsFocus:  focusFileName,
		hashAlgorithm:  defaultHashAlgorithm(),
		excludeHidden:  true,
		excludeSystem:  true,
		createXLSX:     true,
		preserveZeros:  true,
		deleteCSV:      true,
		spinner:        spin,
		activeFlow:     flowScan,
		sourceDir:      startDir,
		outputDir:      startDir,
		emailSourceDir: startDir,
		emailDestDir:   startDir,
		glamourIntro:   intro,
	}
	m.syncSettingsFocus()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app error: %v\n", err)
		os.Exit(1)
	}
}

func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

func isGUIContext() bool {
	exe := os.Args[0]
	// macOS .app bundle
	if strings.Contains(exe, ".app/Contents/MacOS/") {
		return true
	}
	// Wails dev mode passes devserver URL flag
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--wails-devserverurl") || strings.HasPrefix(arg, "--wails-devserver-url") {
			return true
		}
	}
	// Windows: Wails links with -H windowsgui (PE Subsystem 2); go build defaults to CUI (Subsystem 3).
	// Read our own PE header so the GUI binary routes correctly regardless of how it is launched.
	if runtime.GOOS == "windows" && isWindowsPEGUISubsystem(exe) {
		return true
	}
	return false
}

func isWindowsPEGUISubsystem(exePath string) bool {
	f, err := os.Open(exePath)
	if err != nil {
		return false
	}
	defer f.Close()
	pef, err := pe.NewFile(f)
	if err != nil {
		return false
	}
	defer pef.Close()
	const imageSubsystemWindowsGUI = 2
	switch oh := pef.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return oh.Subsystem == imageSubsystemWindowsGUI
	case *pe.OptionalHeader64:
		return oh.Subsystem == imageSubsystemWindowsGUI
	}
	return false
}

func newSourceList(currentDir string) list.Model {
	return newDirectoryList(currentDir, "Source Folder")
}

func newActionList() list.Model {
	items := []list.Item{
		actionItem{
			title:       "Generate",
			description: "Scan a folder to CSV, with optional hashing, filters, and XLSX export.",
			flow:        flowScan,
		},
		actionItem{
			title:       "Copy Email Files",
			description: "Copy supported email files into a chosen destination and save a manifest report.",
			flow:        flowEmailCopy,
		},
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(1)

	l := list.New(items, delegate, 0, 0)
	l.Title = "Main Menu"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	l.SetShowPagination(false)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	return l
}

func newDirectoryList(currentDir, titlePrefix string) list.Model {
	items := make([]list.Item, 0)
	parent := filepath.Dir(currentDir)
	if parent != currentDir {
		items = append(items, dirItem{name: "..", path: parent})
	}

	entries, err := os.ReadDir(currentDir)
	if err == nil {
		dirs := make([]dirItem, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirs = append(dirs, dirItem{
				name: entry.Name(),
				path: filepath.Join(currentDir, entry.Name()),
			})
		}
		slices.SortFunc(dirs, func(a, b dirItem) int {
			return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
		})
		for _, item := range dirs {
			items = append(items, item)
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	l := list.New(items, delegate, 0, 0)
	l.Title = fmt.Sprintf("%s: %s", titlePrefix, safeTerminalText(currentDir))
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	l.SetShowPagination(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	return l
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) resetToHome() model {
	m.stage = stageHome
	m.activeFlow = flowScan
	m.list = newActionList()
	m.list.SetSize(m.width-8, boundedListHeight(m.height))
	m.err = nil
	m.pendingPath = ""
	return m
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-8, boundedListHeight(msg.Height))
		m.outputInput.Width = max(20, msg.Width-28)
		m.excludeInput.Width = max(20, msg.Width-28)
		m.depthInput.Width = max(10, min(20, msg.Width-28))
		return m, nil
	case tea.KeyMsg:
		switch m.stage {
		case stageHome:
			return m.updateHomeStage(msg)
		case stagePickSource:
			return m.updateSourceStage(msg)
		case stagePickOutputDir:
			return m.updateOutputDirStage(msg)
		case stageSetOutput:
			return m.updateOutputStage(msg)
		case stageConfirmOverwrite:
			return m.updateConfirmOverwriteStage(msg)
		case stageScanning:
			if msg.String() == "s" || msg.String() == "esc" {
				cancelActiveScan()
				return m, nil
			}
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
		case stageEmailPickSource:
			return m.updateEmailSourceStage(msg)
		case stageEmailPickDest:
			return m.updateEmailDestStage(msg)
		case stageEmailCopying:
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
		case stageDone, stageFailed:
			if m.activeFlow == flowEmailCopy && msg.String() == "enter" {
				m = m.resetToHome()
				return m, nil
			}
			if msg.String() == "enter" || msg.String() == "q" || msg.String() == "ctrl+c" {
				m.quitting = true
				return m, tea.Quit
			}
		}
	case spinner.TickMsg:
		if m.stage == stageScanning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case scanProgressMsg:
		m.progress = msg
		return m, waitForProgress()
	case scanDoneMsg:
		m.activeFlow = flowScan
		m.stage = stageDone
		m.done = msg
		m.outputPath = msg.outputPath
		return m, nil
	case emailCopyDoneMsg:
		m.activeFlow = flowEmailCopy
		m.stage = stageDone
		m.emailDone = msg
		return m, nil
	case scanErrorMsg:
		m.stage = stageFailed
		m.err = msg.err
		return m, nil
	case scanCanceledMsg:
		m.stage = stageSetOutput
		m.err = fmt.Errorf("scan stopped")
		m.syncSettingsFocus()
		return m, textinput.Blink
	}

	return m, nil
}

func (m model) updateHomeStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		selected, ok := m.list.SelectedItem().(actionItem)
		if !ok {
			return m, nil
		}
		switch selected.flow {
		case flowScan:
			m.activeFlow = flowScan
			m.stage = stagePickSource
			m.list = newSourceList(m.sourceDir)
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		case flowEmailCopy:
			m.activeFlow = flowEmailCopy
			m.stage = stageEmailPickSource
			m.list = newDirectoryList(m.emailSourceDir, "Email Copy Source Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateSourceStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, sourceKeys.Up):
		parent := filepath.Dir(m.sourceDir)
		if parent != m.sourceDir {
			m.sourceDir = parent
			m.list = newSourceList(parent)
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
		}
		return m, nil
	case key.Matches(msg, sourceKeys.Choose):
		selected, ok := m.list.SelectedItem().(dirItem)
		if !ok {
			m.outputDir = m.sourceDir
			m.stage = stagePickOutputDir
			m.list = newDirectoryList(m.outputDir, "Output Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		if selected.name == ".." {
			m.sourceDir = selected.path
			m.list = newSourceList(selected.path)
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		m.sourceDir = selected.path
		m.list = newSourceList(selected.path)
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == " ":
		m.outputDir = m.sourceDir
		m.stage = stagePickOutputDir
		m.list = newDirectoryList(m.outputDir, "Output Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == "esc":
		m = m.resetToHome()
		return m, nil
	case msg.String() == "q" || msg.String() == "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateOutputDirStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, sourceKeys.Up):
		parent := filepath.Dir(m.outputDir)
		if parent != m.outputDir {
			m.outputDir = parent
			m.list = newDirectoryList(parent, "Output Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
		}
		return m, nil
	case key.Matches(msg, sourceKeys.Choose):
		selected, ok := m.list.SelectedItem().(dirItem)
		if !ok {
			m.stage = stageSetOutput
			m.outputInput.SetValue(defaultOutputFilename(m.sourceDir))
			m.syncSettingsFocus()
			return m, textinput.Blink
		}
		if selected.name == ".." {
			m.outputDir = selected.path
			m.list = newDirectoryList(selected.path, "Output Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		m.outputDir = selected.path
		m.list = newDirectoryList(selected.path, "Output Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == " ":
		m.stage = stageSetOutput
		m.outputInput.SetValue(defaultOutputFilename(m.sourceDir))
		m.syncSettingsFocus()
		return m, textinput.Blink
	case msg.String() == "esc":
		m.stage = stagePickSource
		m.list = newDirectoryList(m.sourceDir, "Source Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == "q" || msg.String() == "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateEmailSourceStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, sourceKeys.Up):
		parent := filepath.Dir(m.emailSourceDir)
		if parent != m.emailSourceDir {
			m.emailSourceDir = parent
			m.list = newDirectoryList(parent, "Email Copy Source Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
		}
		return m, nil
	case key.Matches(msg, sourceKeys.Choose):
		selected, ok := m.list.SelectedItem().(dirItem)
		if !ok {
			m.emailDestDir = m.emailSourceDir
			m.stage = stageEmailPickDest
			m.list = newDirectoryList(m.emailDestDir, "Email Copy Destination")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		if selected.name == ".." {
			m.emailSourceDir = selected.path
			m.list = newDirectoryList(selected.path, "Email Copy Source Folder")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		m.emailSourceDir = selected.path
		m.list = newDirectoryList(selected.path, "Email Copy Source Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == " ":
		m.emailDestDir = m.emailSourceDir
		m.stage = stageEmailPickDest
		m.list = newDirectoryList(m.emailDestDir, "Email Copy Destination")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == "esc":
		m = m.resetToHome()
		return m, nil
	case msg.String() == "q" || msg.String() == "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateEmailDestStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, sourceKeys.Up):
		parent := filepath.Dir(m.emailDestDir)
		if parent != m.emailDestDir {
			m.emailDestDir = parent
			m.list = newDirectoryList(parent, "Email Copy Destination")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
		}
		return m, nil
	case key.Matches(msg, sourceKeys.Choose):
		selected, ok := m.list.SelectedItem().(dirItem)
		if !ok {
			return m.beginEmailCopy()
		}
		if selected.name == ".." {
			m.emailDestDir = selected.path
			m.list = newDirectoryList(selected.path, "Email Copy Destination")
			m.list.SetSize(m.width-8, boundedListHeight(m.height))
			return m, nil
		}
		m.emailDestDir = selected.path
		m.list = newDirectoryList(selected.path, "Email Copy Destination")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == " ":
		return m.beginEmailCopy()
	case msg.String() == "esc":
		m.stage = stageEmailPickSource
		m.list = newDirectoryList(m.emailSourceDir, "Email Copy Source Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		return m, nil
	case msg.String() == "q" || msg.String() == "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateOutputStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.stage = stagePickOutputDir
		m.list = newDirectoryList(m.outputDir, "Output Folder")
		m.list.SetSize(m.width-8, boundedListHeight(m.height))
		m.syncSettingsFocus()
		return m, nil
	case "tab", "down":
		m.stepSettingsFocus(1)
		m.syncSettingsFocus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.stepSettingsFocus(-1)
		m.syncSettingsFocus()
		return m, textinput.Blink
	case " ":
		switch m.settingsFocus {
		case focusFoldersOnly:
			m.foldersOnly = !m.foldersOnly
			if m.foldersOnly {
				m.outputInput.SetValue(defaultFolderListFilename(m.sourceDir))
			} else {
				m.outputInput.SetValue(defaultOutputFilename(m.sourceDir))
			}
			m.syncSettingsFocus()
			return m, nil
		case focusHashAlgorithm:
			m.hashAlgorithm = m.hashAlgorithm.Next()
			return m, nil
		case focusHidden:
			m.excludeHidden = !m.excludeHidden
			return m, nil
		case focusSystem:
			m.excludeSystem = !m.excludeSystem
			return m, nil
		case focusXLSX:
			m.createXLSX = !m.createXLSX
			if m.createXLSX {
				m.preserveZeros = true
				m.deleteCSV = true
			} else {
				m.preserveZeros = false
				m.deleteCSV = false
			}
			m.syncSettingsFocus()
			return m, nil
		case focusPreserveZeros:
			if m.createXLSX {
				m.preserveZeros = !m.preserveZeros
			}
			return m, nil
		case focusDeleteCSV:
			if m.createXLSX {
				m.deleteCSV = !m.deleteCSV
			}
			return m, nil
		}
	case "enter":
		switch m.settingsFocus {
		case focusFoldersOnly:
			m.foldersOnly = !m.foldersOnly
			if m.foldersOnly {
				m.outputInput.SetValue(defaultFolderListFilename(m.sourceDir))
			} else {
				m.outputInput.SetValue(defaultOutputFilename(m.sourceDir))
			}
			m.syncSettingsFocus()
			return m, nil
		case focusHashAlgorithm:
			m.hashAlgorithm = m.hashAlgorithm.Next()
			return m, nil
		case focusHidden:
			m.excludeHidden = !m.excludeHidden
			return m, nil
		case focusSystem:
			m.excludeSystem = !m.excludeSystem
			return m, nil
		case focusXLSX:
			m.createXLSX = !m.createXLSX
			if m.createXLSX {
				m.preserveZeros = true
				m.deleteCSV = true
			} else {
				m.preserveZeros = false
				m.deleteCSV = false
			}
			m.syncSettingsFocus()
			return m, nil
		case focusPreserveZeros:
			if m.createXLSX {
				m.preserveZeros = !m.preserveZeros
			}
			return m, nil
		case focusDeleteCSV:
			if m.createXLSX {
				m.deleteCSV = !m.deleteCSV
			}
			return m, nil
		case focusFileName, focusExcludeExts:
			m.stepSettingsFocus(1)
			m.syncSettingsFocus()
			return m, textinput.Blink
		case focusStart:
			filename := strings.TrimSpace(m.outputInput.Value())
			if filename == "" {
				filename = defaultOutputFilename(m.sourceDir)
			}
			outputPath := filepath.Join(m.outputDir, filename)
			if strings.ToLower(filepath.Ext(outputPath)) != ".csv" {
				m.err = fmt.Errorf("output file must end with .csv")
				return m, nil
			}

			excludedMap, err := parseExcludedExtensions(m.excludeInput.Value())
			if err != nil {
				m.err = err
				return m, nil
			}

			exists, err := ensureOutputPath(outputPath)
			if err != nil {
				m.err = err
				return m, nil
			}

			m.pendingPath = outputPath
			m.err = nil
			if exists {
				m.stage = stageConfirmOverwrite
				return m, nil
			}

			return m.beginScan(outputPath, scanOptions{
				HashAlgorithm:    m.hashAlgorithm,
				ExcludeHidden:    m.excludeHidden,
				ExcludeSystem:    m.excludeSystem,
				CreateXLSX:       m.createXLSX && !m.foldersOnly,
				PreserveZeros:    m.preserveZeros,
				DeleteCSV:        m.deleteCSV,
				ExcludedExts:     excludedMap,
				ExcludedExtsText: strings.TrimSpace(m.excludeInput.Value()),
				FoldersOnly:      m.foldersOnly,
				FolderDepth:      parseFolderDepth(m.depthInput.Value()),
			})
		}
	}

	var cmd tea.Cmd
	switch m.settingsFocus {
	case focusFileName:
		m.outputInput, cmd = m.outputInput.Update(msg)
	case focusExcludeExts:
		m.excludeInput, cmd = m.excludeInput.Update(msg)
	case focusFolderDepth:
		m.depthInput, cmd = m.depthInput.Update(msg)
	}
	return m, cmd
}

func (m model) updateConfirmOverwriteStage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		excludedMap, err := parseExcludedExtensions(m.excludeInput.Value())
		if err != nil {
			m.err = err
			m.stage = stageSetOutput
			return m, nil
		}
		return m.beginScan(m.pendingPath, scanOptions{
			HashAlgorithm:    m.hashAlgorithm,
			ExcludeHidden:    m.excludeHidden,
			ExcludeSystem:    m.excludeSystem,
			CreateXLSX:       m.createXLSX && !m.foldersOnly,
			PreserveZeros:    m.preserveZeros,
			DeleteCSV:        m.deleteCSV,
			ExcludedExts:     excludedMap,
			ExcludedExtsText: strings.TrimSpace(m.excludeInput.Value()),
			FoldersOnly:      m.foldersOnly,
		})
	case "n", "N", "esc":
		m.stage = stageSetOutput
		m.pendingPath = ""
		m.syncSettingsFocus()
		return m, textinput.Blink
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) beginScan(outputPath string, options scanOptions) (tea.Model, tea.Cmd) {
	m.stage = stageScanning
	m.activeFlow = flowScan
	m.err = nil
	m.pendingPath = ""
	m.outputPath = outputPath
	m.scanStartedAt = time.Now()
	return m, tea.Batch(
		m.spinner.Tick,
		startScan(m.sourceDir, outputPath, options),
		waitForProgress(),
	)
}

func (m model) beginEmailCopy() (tea.Model, tea.Cmd) {
	m.stage = stageEmailCopying
	m.activeFlow = flowEmailCopy
	m.err = nil
	m.scanStartedAt = time.Now()
	return m, tea.Batch(
		m.spinner.Tick,
		startEmailCopy(m.emailSourceDir, m.emailDestDir),
	)
}

func (m *model) syncSettingsFocus() {
	if !m.isSettingsFocusVisible(m.settingsFocus) {
		if m.foldersOnly {
			m.settingsFocus = focusFoldersOnly
		} else {
			m.settingsFocus = focusXLSX
		}
	}
	if m.settingsFocus == focusFileName {
		m.outputInput.Focus()
	} else {
		m.outputInput.Blur()
	}
	if m.settingsFocus == focusExcludeExts {
		m.excludeInput.Focus()
	} else {
		m.excludeInput.Blur()
	}
	if m.settingsFocus == focusFolderDepth {
		m.depthInput.Focus()
	} else {
		m.depthInput.Blur()
	}
}

func (m *model) isSettingsFocusVisible(focus int) bool {
	if m.foldersOnly {
		switch focus {
		case focusExcludeExts, focusHashAlgorithm, focusSystem, focusXLSX, focusPreserveZeros, focusDeleteCSV:
			return false
		}
	} else {
		if focus == focusFolderDepth {
			return false
		}
	}
	if focus == focusDeleteCSV && !m.createXLSX {
		return false
	}
	return true
}

func (m *model) stepSettingsFocus(delta int) {
	next := m.settingsFocus
	for i := 0; i < focusCount; i++ {
		next = (next + delta + focusCount) % focusCount
		if m.isSettingsFocusVisible(next) {
			m.settingsFocus = next
			return
		}
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	switch m.stage {
	case stageHome:
		return m.viewHome()
	case stagePickSource:
		return m.viewSourcePicker()
	case stagePickOutputDir:
		return m.viewOutputDirPicker()
	case stageSetOutput:
		return m.viewOutputForm()
	case stageConfirmOverwrite:
		return m.viewConfirmOverwrite()
	case stageScanning:
		return m.viewScanning()
	case stageEmailPickSource:
		return m.viewEmailSourcePicker()
	case stageEmailPickDest:
		return m.viewEmailDestPicker()
	case stageEmailCopying:
		return m.viewEmailCopying()
	case stageDone:
		if m.activeFlow == flowEmailCopy {
			return m.viewEmailDone()
		}
		return m.viewDone()
	case stageFailed:
		return m.viewError()
	default:
		return ""
	}
}

func (m model) viewHome() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleDoc(m.glamourIntro),
		"",
		styleHint("Choose a workflow. Press enter to open it."),
		"",
		renderListWithFade(m.list.View()),
	)
	return styleFrame(body, m.width)
}

func (m model) viewSourcePicker() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleDoc(m.glamourIntro),
		"",
		styleHint("Navigate folders with arrows. Press enter to open a folder. Press space to choose the current folder."),
		"",
		renderListWithFade(m.list.View()),
		"",
		styleHint(fmt.Sprintf("Current source folder: %s", m.sourceDir)),
	)
	return styleFrame(body, m.width)
}

func (m model) viewOutputDirPicker() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("Choose Output Folder"),
		"",
		styleLabel(fmt.Sprintf("Source folder: %s", m.sourceDir)),
		styleHint("Navigate folders with arrows. Press enter to open a folder. Press space to choose the current folder."),
		"",
		renderListWithFade(m.list.View()),
		"",
		styleHint(fmt.Sprintf("Current output folder: %s", m.outputDir)),
	)
	return styleFrame(body, m.width)
}

func (m model) viewOutputForm() string {
	lines := []string{
		styleTitle("Output Settings"),
		"",
		styleLabel(fmt.Sprintf("Source folder: %s", m.sourceDir)),
		styleLabel(fmt.Sprintf("Output folder: %s", m.outputDir)),
		"",
		focusedLine(m.settingsFocus == focusFileName, "File name", m.outputInput.View()),
		focusedInfo(false, "Format", "Scan writes CSV first. XLSX can be created afterward as a spreadsheet copy."),
		focusedInfo(false, "Large scans", fmt.Sprintf("CSV splits every %d rows and names parts as [name]-001.csv, [name]-002.csv, and so on.", defaultMaxRowsPerCSV)),
		focusedToggle(m.settingsFocus == focusFoldersOnly, "Folders only (no files)", m.foldersOnly),
	}
	if !m.foldersOnly {
		lines = append(lines,
			focusedLine(m.settingsFocus == focusExcludeExts, "Exclude extensions", m.excludeInput.View()),
			focusedChoice(m.settingsFocus == focusHashAlgorithm, "Verification hash", m.hashAlgorithm.OptionLabel()),
			focusedToggle(m.settingsFocus == focusHidden, "Exclude hidden files", m.excludeHidden),
			focusedToggle(m.settingsFocus == focusSystem, "Exclude common system files", m.excludeSystem),
			focusedToggle(m.settingsFocus == focusXLSX, "Create XLSX after scan", m.createXLSX),
			focusedToggle(m.settingsFocus == focusPreserveZeros, "Preserve leading zeros in XLSX", m.preserveZeros && m.createXLSX),
		)
		if m.createXLSX {
			lines = append(lines, focusedToggle(m.settingsFocus == focusDeleteCSV, "Delete CSV after XLSX is created", m.deleteCSV))
		}
	} else {
		depthLabel := "all levels"
		if d := parseFolderDepth(m.depthInput.Value()); d > 0 {
			depthLabel = fmt.Sprintf("%d level(s)", d)
		}
		lines = append(lines,
			focusedToggle(m.settingsFocus == focusHidden, "Exclude hidden folders", m.excludeHidden),
			focusedLine(m.settingsFocus == focusFolderDepth, fmt.Sprintf("Max depth (%s)", depthLabel), m.depthInput.View()),
		)
	}
	lines = append(
		lines,
		focusedAction(m.settingsFocus == focusStart, "Start scan"),
		"",
		styleHint("Tab or arrows move between controls. Space or enter changes the focused setting."),
		styleHint(fmt.Sprintf("Preview: %s", filepath.Join(m.outputDir, valueOrDefault(strings.TrimSpace(m.outputInput.Value()), defaultOutputFilename(m.sourceDir))))),
	)

	if m.err != nil {
		lines = append(lines, styleError(m.err.Error()))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return styleFrame(body, m.width)
}

func (m model) viewScanning() string {
	phaseTitle := fmt.Sprintf("%s %s...", m.spinner.View(), m.progress.phase)
	if strings.TrimSpace(m.progress.phase) == "" {
		phaseTitle = fmt.Sprintf("%s Working...", m.spinner.View())
	}
	fileStat := formatUint(m.progress.files)
	if m.progress.totalFiles > 0 {
		fileStat = fmt.Sprintf("%s / %s", formatUint(m.progress.files), formatUint(m.progress.totalFiles))
	}
	directoryStat := formatUint(m.progress.directories)
	if m.progress.totalDirectories > 0 {
		directoryStat = fmt.Sprintf("%s / %s", formatUint(m.progress.directories), formatUint(m.progress.totalDirectories))
	}
	byteStat := humanBytes(m.progress.bytes)
	if m.progress.totalBytes > 0 {
		byteStat = fmt.Sprintf("%s / %s", humanBytes(m.progress.bytes), humanBytes(m.progress.totalBytes))
	}
	var progressStat string
	if m.progress.phase == progressPhaseLabel(progressPhaseScanning) {
		progressStat = formatPercent(m.progress.percent)
	} else {
		progressStat = fmt.Sprintf("%s files, %s dirs", formatUint(m.progress.files), formatUint(m.progress.directories))
	}
	etaStat := "calculating"
	if m.progress.phase != progressPhaseLabel(progressPhaseScanning) {
		etaStat = "after count"
	} else if m.progress.eta > 0 {
		etaStat = m.progress.eta.Round(time.Second).String()
	}
	currentFileStat := valueOrDefault(m.progress.currentItem, "waiting for first file")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle(phaseTitle),
		"",
		styleLabel(fmt.Sprintf("Source: %s", m.sourceDir)),
		styleLabel(fmt.Sprintf("Output: %s", m.outputPath)),
		styleLabel(fmt.Sprintf("Verification hash: %s", m.hashAlgorithm.OptionLabel())),
		styleLabel(fmt.Sprintf("Exclude hidden: %s", onOff(m.excludeHidden))),
		styleLabel(fmt.Sprintf("Exclude system: %s", onOff(m.excludeSystem))),
		styleLabel(fmt.Sprintf("Create XLSX: %s", onOff(m.createXLSX))),
		styleLabel(fmt.Sprintf("Preserve zeros in XLSX: %s", onOff(m.preserveZeros && m.createXLSX))),
		styleLabel(fmt.Sprintf("Delete CSV after XLSX: %s", onOff(m.deleteCSV && m.createXLSX))),
		styleLabel(fmt.Sprintf("Excluded exts: %s", valueOrDefault(strings.TrimSpace(m.excludeInput.Value()), "none"))),
		"",
		styleStat("Progress", progressStat),
		styleStat("Files", fileStat),
		styleStat("Directories", directoryStat),
		styleStat("Bytes", byteStat),
		styleStat("Filtered out", formatUint(m.progress.filtered)),
		styleStat("ETA", etaStat),
		styleStat("Current file", currentFileStat),
		styleStat("Elapsed", m.progress.elapsed.Round(time.Second).String()),
		"",
		styleHint("Press s or esc to stop the scan. The app counts first so progress and ETA are based on the real file total."),
	)
	return styleFrame(body, m.width)
}

func (m model) viewEmailSourcePicker() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("Choose Email Copy Source Folder"),
		"",
		styleHint("Navigate folders with arrows. Press enter to open a folder. Press space to choose the current folder."),
		"",
		renderListWithFade(m.list.View()),
		"",
		styleHint(fmt.Sprintf("Current source folder: %s", m.emailSourceDir)),
	)
	return styleFrame(body, m.width)
}

func (m model) viewEmailDestPicker() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("Choose Email Copy Destination"),
		"",
		styleLabel(fmt.Sprintf("Source folder: %s", m.emailSourceDir)),
		styleHint("Navigate folders with arrows. Press enter to open a folder. Press space to choose the current folder as the destination."),
		"",
		renderListWithFade(m.list.View()),
		"",
		styleHint(fmt.Sprintf("Current destination: %s", m.emailDestDir)),
	)
	return styleFrame(body, m.width)
}

func (m model) viewEmailCopying() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle(fmt.Sprintf("%s Copying Email Files...", m.spinner.View())),
		"",
		styleLabel(fmt.Sprintf("Source: %s", m.emailSourceDir)),
		styleLabel(fmt.Sprintf("Destination: %s", m.emailDestDir)),
		styleLabel(fmt.Sprintf("Extensions: %s", strings.Join(sortedEmailExtensions(), ", "))),
		"",
		styleHint("Relative folders from the source root are preserved in the destination."),
	)
	return styleFrame(body, m.width)
}

func (m model) viewConfirmOverwrite() string {
	firstPartPath := csvOutputPathForPart(m.pendingPath, 1)
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("File Already Exists"),
		"",
		styleLabel("The first CSV output file already exists:"),
		styleLabel(firstPartPath),
		"",
		styleHint("Press y to overwrite it, or n to go back and choose another name."),
	)
	return styleFrame(body, m.width)
}

func (m model) viewDone() string {
	if m.done.foldersOnly {
		body := lipgloss.JoinVertical(
			lipgloss.Left,
			styleTitle("Folder List Complete"),
			"",
			styleStat("Output", m.done.outputPath),
			styleStat("Report", valueOrDefault(m.done.reportPath, "not created")),
			styleStat("Folders in CSV", formatUint(m.done.directories)),
			styleStat("Folders skipped", formatUint(m.done.filtered)),
			styleStat("Elapsed", m.done.elapsed.Round(time.Millisecond).String()),
			styleStat("Selected folder", valueOrDefault(m.done.sourceName, "unknown")),
			styleStat("First folder in CSV", valueOrDefault(m.done.firstCSVItem, "none")),
			styleStat("Last folder in CSV", valueOrDefault(m.done.lastCSVItem, "none")),
			"",
			styleHint("Press enter to exit."),
		)
		return styleFrame(body, m.width)
	}

	countSummary := renderSummaryList("Top extensions by file count", m.done.topByCount, func(entry summaryEntry) string {
		return fmt.Sprintf("%s files, %s", formatUint(entry.Count), humanBytes(entry.Bytes))
	})
	sizeSummary := renderSummaryList("Top extensions by total size", m.done.topBySize, func(entry summaryEntry) string {
		return fmt.Sprintf("%s, %s files", humanBytes(entry.Bytes), formatUint(entry.Count))
	})

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("Scan Complete"),
		"",
		styleStat("Output", m.done.outputPath),
		styleStat("CSV files", fmt.Sprintf("%d", m.done.csvPartCount)),
		styleStat("Rows per CSV max", fmt.Sprintf("%d", m.done.maxRowsPerCSV)),
		styleStat("CSV parts", summarizeOutputParts(m.done.outputPaths)),
		styleStat("XLSX copy", valueOrDefault(m.done.xlsxPath, "not created")),
		styleStat("XLSX files", fmt.Sprintf("%d", m.done.xlsxPartCount)),
		styleStat("XLSX parts", summarizeOutputParts(m.done.xlsxPaths)),
		styleStat("Report", valueOrDefault(m.done.reportPath, "not created")),
		styleStat("Files", formatUint(m.done.files)),
		styleStat("Directories", formatUint(m.done.directories)),
		styleStat("Bytes", humanBytes(m.done.bytes)),
		styleStat("Filtered out", formatUint(m.done.filtered)),
		styleStat("Hidden filtered", formatUint(m.done.filteredHidden)),
		styleStat("System filtered", formatUint(m.done.filteredSystem)),
		styleStat("Extension filtered", formatUint(m.done.filteredExts)),
		styleStat("Errors skipped", formatUint(m.done.errors)),
		styleStat("Elapsed", m.done.elapsed.Round(time.Millisecond).String()),
		styleStat("Verification hash", m.done.hashAlgorithm.OptionLabel()),
		styleStat("Selected folder", valueOrDefault(m.done.sourceName, "unknown")),
		styleStat("First file in CSV", valueOrDefault(m.done.firstCSVItem, "none")),
		styleStat("Last file in CSV", valueOrDefault(m.done.lastCSVItem, "none")),
		styleStat("Hash workers", fmt.Sprintf("%d", m.done.hashWorkers)),
		styleStat("Create XLSX", onOff(m.done.createXLSX)),
		styleStat("Preserve zeros", onOff(m.done.preserveZeros)),
		styleStat("Delete CSV after XLSX", onOff(m.done.deleteCSV && m.done.createXLSX)),
		styleStat("CSV removed after XLSX", onOff(m.done.csvDeleted)),
		"",
		countSummary,
		"",
		sizeSummary,
		"",
		renderFilteredSamples(m.done.filteredSamples),
		"",
		styleHint("Press enter to exit."),
	)
	return styleFrame(body, m.width)
}

func (m model) viewEmailDone() string {
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle("Email Copy Complete"),
		"",
		styleStat("Source", m.emailDone.sourceDir),
		styleStat("Destination", m.emailDone.destDir),
		styleStat("Manifest", m.emailDone.manifestPath),
		styleStat("Copied", formatUint(m.emailDone.copied)),
		styleStat("Elapsed", m.emailDone.elapsed.Round(time.Millisecond).String()),
		"",
		styleHint("Press enter to return to the main menu."),
	)
	return styleFrame(body, m.width)
}

func (m model) viewError() string {
	hint := "Press enter to exit."
	title := "Scan Failed"
	if m.activeFlow == flowEmailCopy {
		title = "Email Copy Failed"
		hint = "Press enter to return to the main menu."
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		styleTitle(title),
		"",
		styleError(m.err.Error()),
		"",
		styleHint(hint),
	)
	return styleFrame(body, m.width)
}

func startScan(sourceDir, outputPath string, options scanOptions) tea.Cmd {
	return func() tea.Msg {
		done, err := runScanWithContext(context.Background(), sourceDir, outputPath, options)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return scanCanceledMsg{}
			}
			return scanErrorMsg{err: err}
		}
		return done
	}
}

func startEmailCopy(sourceDir, destDir string) tea.Cmd {
	return func() tea.Msg {
		startedAt := time.Now()
		manifestPath, copied, err := copyEmailFiles(sourceDir, destDir)
		if err != nil {
			return scanErrorMsg{err: err}
		}
		return emailCopyDoneMsg{
			sourceDir:    sourceDir,
			destDir:      destDir,
			manifestPath: manifestPath,
			copied:       copied,
			elapsed:      time.Since(startedAt),
		}
	}
}

func waitForProgress() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		stats := currentProgress()
		return scanProgressMsg{
			phase:            progressPhaseLabel(stats.phase),
			files:            stats.files,
			directories:      stats.directories,
			bytes:            stats.bytes,
			filtered:         stats.filtered,
			totalFiles:       stats.totalFiles,
			totalDirectories: stats.totalDirectories,
			totalBytes:       stats.totalBytes,
			currentItem:      stats.currentItem,
			percent:          progressFraction(stats),
			eta:              progressETA(stats, t),
			elapsed:          time.Since(stats.startedAt),
		}
	})
}

func ensureOutputPath(outputPath string) (bool, error) {
	parent := filepath.Dir(outputPath)
	if parent == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return false, err
		}
		parent = cwd
	}

	finalCSVPath := csvOutputPathForPart(outputPath, 1)
	info, err := os.Stat(finalCSVPath)
	if err == nil && !info.IsDir() {
		return true, nil
	}
	if err == nil && info.IsDir() {
		return false, fmt.Errorf("output path is a directory: %s", finalCSVPath)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, err
	}

	testFile := filepath.Join(parent, fmt.Sprintf(".content-list-generator-%d.tmp", time.Now().UnixNano()))
	f, err := os.Create(testFile)
	if err != nil {
		return false, fmt.Errorf("output folder is not writable: %s", parent)
	}
	f.Close()
	return false, os.Remove(testFile)
}

func isSupportedOutputPath(outputPath string) bool {
	return strings.ToLower(filepath.Ext(outputPath)) == ".csv"
}

func parseFolderDepth(input string) int {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseExcludedExtensions(input string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if strings.TrimSpace(input) == "" {
		return result, nil
	}

	for _, raw := range strings.Split(input, ",") {
		part := strings.TrimSpace(strings.ToLower(raw))
		part = strings.TrimPrefix(part, ".")
		if part == "" {
			continue
		}
		if strings.ContainsAny(part, `/\ `) {
			return nil, fmt.Errorf("excluded extensions should be comma-separated values like tmp,log")
		}
		result[part] = struct{}{}
	}
	return result, nil
}

func shouldSkipFile(path, name string, options scanOptions) (string, bool) {
	if isAlwaysExcludedFile(name) {
		return "os noise", true
	}
	if options.ExcludeHidden && hasHiddenComponent(path) {
		return "hidden path", true
	}
	if options.ExcludeSystem && isSystemFile(name) {
		return "system file", true
	}
	if _, ok := options.ExcludedExts[normalizeExt(filepath.Ext(name))]; ok {
		return "excluded extension", true
	}
	return "", false
}

// isAlwaysExcludedDir reports whether a directory should always be skipped
// regardless of user settings. These are OS infrastructure directories that
// are never archival content.
func isAlwaysExcludedDir(name string) bool {
	switch strings.ToLower(name) {
	case "$recycle.bin",
		"system volume information",
		".spotlight-v100",
		".trashes",
		".fseventsd",
		".temporaryitems",
		".documentrevisions-v100":
		return true
	}
	return false
}

// isAlwaysExcludedFile reports whether a file should always be skipped
// regardless of user settings. Covers OS noise: temp files, thumbnails,
// Windows swap files, macOS resource forks, Office/LibreOffice lock files.
func isAlwaysExcludedFile(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "pagefile.sys", "hiberfil.sys", "swapfile.sys",
		"thumbs.db", "ehthumbs.db", "desktop.ini", ".ds_store":
		return true
	}
	if strings.HasPrefix(lower, "~$") ||
		strings.HasPrefix(lower, ".~lock.") ||
		strings.HasPrefix(name, "._") {
		return true
	}
	if strings.HasSuffix(lower, ".tmp") {
		return true
	}
	return false
}

func hasHiddenComponent(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if isHiddenName(part) {
			return true
		}
	}
	return false
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

func isSystemFile(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db", "desktop.ini", "ehthumbs.db":
		return true
	default:
		return false
	}
}

func normalizeExt(ext string) string {
	return strings.TrimPrefix(strings.ToLower(ext), ".")
}

func summaryKey(ext string) string {
	if ext == "" {
		return "[no extension]"
	}
	return "." + ext
}

func appendFilteredSample(samples []string, value string) []string {
	if len(samples) >= 8 {
		return samples
	}
	return append(samples, value)
}

func summarizeByCount(entries map[string]summaryEntry, limit int) []summaryEntry {
	out := make([]summaryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b summaryEntry) int {
		if a.Count != b.Count {
			if a.Count > b.Count {
				return -1
			}
			return 1
		}
		if a.Bytes != b.Bytes {
			if a.Bytes > b.Bytes {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Label, b.Label)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func summarizeBySize(entries map[string]summaryEntry, limit int) []summaryEntry {
	out := make([]summaryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b summaryEntry) int {
		if a.Bytes != b.Bytes {
			if a.Bytes > b.Bytes {
				return -1
			}
			return 1
		}
		if a.Count != b.Count {
			if a.Count > b.Count {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Label, b.Label)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func defaultOutputPath(sourceDir string) string {
	return filepath.Join(sourceDir, defaultOutputFilename(sourceDir))
}

func defaultOutputFilename(sourceDir string) string {
	stamp := time.Now().Format("2006-01-02T15-04-05")
	name := filepath.Base(sourceDir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "content-list"
	}
	return fmt.Sprintf("%s-content-list-%s.csv", name, stamp)
}

func folderDisplayName(path string) string {
	name := filepath.Base(filepath.Clean(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}

func humanBytes(bytes uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", bytes, units[idx])
	}
	if value >= 10 {
		return fmt.Sprintf("%.1f %s", value, units[idx])
	}
	return fmt.Sprintf("%.2f %s", value, units[idx])
}

func formatUint(v uint64) string {
	return fmt.Sprintf("%d", v)
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.0f%%", value*100)
}

func renderMarkdown(input string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return input
	}
	out, err := renderer.Render(input)
	if err != nil {
		return input
	}
	return strings.TrimSpace(out)
}

func renderSummaryList(title string, items []summaryEntry, formatter func(summaryEntry) string) string {
	lines := []string{styleTitle(title)}
	if len(items) == 0 {
		lines = append(lines, styleHint("No files were written."))
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	for _, item := range items {
		lines = append(lines, styleStat(item.Label, formatter(item)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderFilteredSamples(items []string) string {
	lines := []string{styleTitle("Filtered examples")}
	if len(items) == 0 {
		lines = append(lines, styleHint("Nothing was filtered."))
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	for _, item := range items {
		lines = append(lines, styleHint(item))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func renderListWithFade(content string) string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  . . ."),
		content,
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  . . ."),
	)
}

func focusedLine(focused bool, label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		focusPrefix(focused),
		lipgloss.JoinVertical(
			lipgloss.Left,
			styleLabel(label),
			value,
		),
	)
}

func focusedInfo(focused bool, label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		focusPrefix(focused),
		lipgloss.JoinVertical(
			lipgloss.Left,
			styleLabel(label),
			styleHint(value),
		),
	)
}

func focusedChoice(focused bool, label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		focusPrefix(focused),
		styleLabel(fmt.Sprintf("%s: %s", label, value)),
	)
}

func focusedToggle(focused bool, label string, value bool) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		focusPrefix(focused),
		styleLabel(fmt.Sprintf("%s: [%s]", label, checkbox(value))),
	)
}

func focusedAction(focused bool, label string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		focusPrefix(focused),
		styleTitle(label),
	)
}

func focusPrefix(focused bool) string {
	if focused {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("> ")
	}
	return "  "
}

func checkbox(v bool) string {
	if v {
		return "x"
	}
	return " "
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func boundedListHeight(windowHeight int) int {
	height := windowHeight - 28
	if height < 8 {
		return 8
	}
	if height > 14 {
		return 14
	}
	return height
}

func styleFrame(body string, width int) string {
	w := width - 4
	if w < 40 {
		w = 40
	}
	return lipgloss.NewStyle().
		Padding(1, 2).
		Width(w).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Render(body)
}

func styleDoc(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(s)
}

// safeTerminalText renders filesystem and error text without allowing control
// sequences or invisible direction markers to alter the terminal display.
func safeTerminalText(value string) string {
	var result strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			if r <= 0xff {
				fmt.Fprintf(&result, "\\x%02X", r)
			} else {
				fmt.Fprintf(&result, "\\u%04X", r)
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func styleTitle(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render(s)
}

func styleLabel(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(safeTerminalText(s))
}

func styleHint(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(safeTerminalText(s))
}

func styleError(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(safeTerminalText(s))
}

func styleStat(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(22).Render(safeTerminalText(label)+":"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(safeTerminalText(value)),
	)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
