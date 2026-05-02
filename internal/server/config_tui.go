package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ─── styles (reuse palette from network_tui.go) ──────────────────────────────

var (
	scTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	scAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	scSubtle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	scSel     = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	scNorm    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	scDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	scSet     = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	scSecret  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	scGroup   = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).MarginTop(1)
	scBox     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3).
			Width(64)
	scInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(1, 3).
			Width(64)
)

// ─── config field definitions ─────────────────────────────────────────────────

type configField struct {
	group    string
	label    string
	envKey   string
	hint     string
	isSecret bool
	value    string // current / edited value
}

var configFields = []configField{
	{
		group:    "Encryption",
		label:    "Encryption Key",
		envKey:   "ENCRYPTION_KEY",
		hint:     "64-char hex key  ·  openssl rand -hex 32",
		isSecret: true,
	},
	{
		group:  "GitHub OAuth",
		label:  "Client ID",
		envKey: "GITHUB_CLIENT_ID",
		hint:   "GitHub OAuth App → Client ID",
	},
	{
		group:    "GitHub OAuth",
		label:    "Client Secret",
		envKey:   "GITHUB_CLIENT_SECRET",
		hint:     "GitHub OAuth App → Client Secret",
		isSecret: true,
	},
	{
		group:  "GitHub OAuth",
		label:  "Callback URL",
		envKey: "GITHUB_CALLBACK_URL",
		hint:   "https://your-domain.com/api/v1/github/callback",
	},
	{
		group:  "GitHub OAuth",
		label:  "Frontend URL",
		envKey: "FRONTEND_URL",
		hint:   "deckplane://auth/callback",
	},
	{
		group:  "GitHub OAuth",
		label:  "Public Base URL",
		envKey: "PUBLIC_BASE_URL",
		hint:   "https://your-domain.com  (required for webhooks)",
	},
	{
		group:  "Google Drive OAuth",
		label:  "Client ID",
		envKey: "GOOGLE_CLIENT_ID",
		hint:   "Google Cloud Console → OAuth 2.0 Client ID",
	},
	{
		group:    "Google Drive OAuth",
		label:    "Client Secret",
		envKey:   "GOOGLE_CLIENT_SECRET",
		hint:     "Google Cloud Console → Client Secret",
		isSecret: true,
	},
	{
		group:  "Google Drive OAuth",
		label:  "Callback URL",
		envKey: "GOOGLE_CALLBACK_URL",
		hint:   "https://your-domain.com/api/v1/customers/google-drive/callback",
	},
}

// extra virtual items appended after the fields
const (
	actionSave        = "Save"
	actionSaveRestart = "Save & Restart"
)

// ─── model ────────────────────────────────────────────────────────────────────

type cfgState int

const (
	cfgList cfgState = iota
	cfgEdit
)

type configModel struct {
	state     cfgState
	fields    []configField
	cursor    int // index into fields + 2 actions
	ti        textinput.Model
	editIdx   int
	saved     bool
	restart   bool
	cancelled bool
}

func newConfigModel(existing map[string]string) configModel {
	fields := make([]configField, len(configFields))
	copy(fields, configFields)
	for i, f := range fields {
		if v, ok := existing[f.envKey]; ok {
			fields[i].value = v
		}
	}

	ti := textinput.New()
	ti.CharLimit = 512
	ti.PromptStyle = scAccent
	ti.TextStyle = scNorm

	return configModel{fields: fields, ti: ti}
}

func (m configModel) totalItems() int { return len(m.fields) + 2 }

func (m configModel) Init() tea.Cmd { return nil }

func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case cfgList:
		return m.updateList(msg)
	case cfgEdit:
		return m.updateEdit(msg)
	}
	return m, nil
}

func (m configModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	total := m.totalItems()
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < total-1 {
			m.cursor++
		}
	case "enter", " ":
		if m.cursor < len(m.fields) {
			// Edit field
			m.editIdx = m.cursor
			m.ti.Placeholder = m.fields[m.cursor].hint
			m.ti.SetValue(m.fields[m.cursor].value)
			if m.fields[m.cursor].isSecret {
				m.ti.EchoMode = textinput.EchoPassword
				m.ti.EchoCharacter = '•'
			} else {
				m.ti.EchoMode = textinput.EchoNormal
			}
			m.ti.Focus()
			m.state = cfgEdit
			return m, textinput.Blink
		}
		// Actions
		actionIdx := m.cursor - len(m.fields)
		if actionIdx == 0 {
			m.saved = true
			m.restart = false
		} else {
			m.saved = true
			m.restart = true
		}
		return m, tea.Quit
	case "ctrl+c", "q":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m configModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "esc":
		m.ti.Blur()
		m.state = cfgList
		return m, nil
	case "enter":
		m.fields[m.editIdx].value = strings.TrimSpace(m.ti.Value())
		m.ti.Blur()
		m.state = cfgList
		return m, nil
	default:
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(key)
		return m, cmd
	}
}

func (m configModel) View() string {
	if m.state == cfgEdit {
		return m.viewEdit()
	}
	return m.viewList()
}

func (m configModel) viewList() string {
	var b strings.Builder
	b.WriteString(scTitle.Render("Deckplane — Server Configuration") + "\n")
	b.WriteString(scSubtle.Render("↑/↓ navigate  ·  enter edit  ·  q quit") + "\n\n")

	prevGroup := ""
	for i, f := range m.fields {
		if f.group != prevGroup {
			prevGroup = f.group
			b.WriteString(scGroup.Render("── " + f.group + " ") + "\n")
		}

		cursor := "  "
		labelStyle := scNorm
		if i == m.cursor {
			cursor = "▸ "
			labelStyle = scSel
		}

		val := f.value
		var valStr string
		if val == "" {
			valStr = scDim.Render("not set")
		} else if f.isSecret {
			valStr = scSecret.Render("••••••••")
		} else {
			if len(val) > 28 {
				val = val[:25] + "..."
			}
			valStr = scSet.Render(val)
		}

		line := fmt.Sprintf("%s%-20s  %s", cursor, f.label, valStr)
		b.WriteString(labelStyle.Render(line) + "\n")
	}

	// Action buttons
	b.WriteString("\n")
	actions := []string{actionSave, actionSaveRestart}
	for i, a := range actions {
		idx := len(m.fields) + i
		cursor := "  "
		style := scNorm
		if idx == m.cursor {
			cursor = "▸ "
			style = scAccent
		}
		b.WriteString(style.Render(cursor+a) + "\n")
	}

	return scBox.Render(b.String())
}

func (m configModel) viewEdit() string {
	f := m.fields[m.editIdx]
	var b strings.Builder
	b.WriteString(scTitle.Render(f.group+" · "+f.label) + "\n\n")
	b.WriteString(m.ti.View() + "\n\n")
	b.WriteString(scSubtle.Render(f.hint) + "\n\n")
	b.WriteString(scSubtle.Render("enter confirm  ·  esc cancel"))
	return scInputBox.Render(b.String())
}

// ─── entry point ─────────────────────────────────────────────────────────────

// promptConfig opens the interactive config editor pre-filled with existing
// values. Returns the updates map and whether to restart. Returns nil if the
// user cancelled without saving.
func promptConfig(existing map[string]string) (updates map[string]string, restart bool, err error) {
	p := tea.NewProgram(newConfigModel(existing), tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return nil, false, err
	}
	m := final.(configModel)
	if m.cancelled || !m.saved {
		return nil, false, nil
	}

	updates = map[string]string{}
	for _, f := range m.fields {
		if f.value != "" {
			updates[f.envKey] = f.value
		}
	}

	// Auto-generate ENCRYPTION_KEY if GitHub fields are being set but key is missing.
	if _, hasKey := updates["ENCRYPTION_KEY"]; !hasKey {
		if _, hasID := updates["GITHUB_CLIENT_ID"]; hasID {
			key, err := randomHex(32)
			if err != nil {
				return nil, false, err
			}
			updates["ENCRYPTION_KEY"] = key
		}
	}

	return updates, m.restart, nil
}
