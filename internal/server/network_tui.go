package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// NetworkMode controls how the control plane is exposed to the outside world.
type NetworkMode int

const (
	NetworkModePort    NetworkMode = iota // direct host port binding
	NetworkModeTraefik                    // routed through a Traefik reverse proxy
)

// NetworkConfig holds the resolved networking choice for the compose file.
type NetworkConfig struct {
	Mode         NetworkMode
	NetworkName  string // traefik: external Docker network to join
	Host         string // traefik: hostname for the routing rule
	CertResolver string // traefik: TLS certresolver name (e.g. "le", "letsencrypt")
	Port         int    // port: host port to bind
	CreateNet    bool   // traefik: network must be created before compose up
}

// AuthentikMode controls whether/how Authentik SSO is configured.
type AuthentikMode int

const (
	AuthentikSkip     AuthentikMode = iota // no SSO
	AuthentikExisting                      // user already has Authentik running
	AuthentikInstall                       // add Authentik to the compose stack
)

// AuthentikConfig holds the resolved Authentik SSO choice.
type AuthentikConfig struct {
	Mode        AuthentikMode
	ExistingURL string // AuthentikExisting: base URL of the running instance
}

// isInteractive returns true when stdin is attached to a real terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptNetworkConfig shows the TUI and returns the user's choices.
// Only call when isInteractive() is true.
func promptNetworkConfig(networks []string) (*NetworkConfig, *AuthentikConfig, error) {
	p := tea.NewProgram(newNetworkModel(networks), tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return nil, nil, err
	}
	m := final.(networkModel)
	if m.cancelled {
		return nil, nil, fmt.Errorf("installation cancelled")
	}
	return m.config, m.authentikCfg, nil
}

// ─── styles ──────────────────────────────────────────────────────────────────

var (
	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	sAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	sSubtle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	sNorm   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	sBox    = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 3).
		Width(58)
)

// ─── model ───────────────────────────────────────────────────────────────────

type netState int

const (
	sPickMode netState = iota
	sPickNet
	sCreateNet
	sEnterHost
	sEnterCertResolver
	sEnterPort
	sPickAuth
	sEnterAuthURL
)

type networkModel struct {
	state        netState
	cursor       int
	networks     []string
	ti           textinput.Model
	mode         NetworkMode
	netName      string
	host         string // traefik: pending host before certresolver is entered
	config       *NetworkConfig
	authentikCfg *AuthentikConfig
	cancelled    bool
}

func newNetworkModel(networks []string) networkModel {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.PromptStyle = sAccent
	ti.TextStyle = sNorm
	return networkModel{networks: networks, ti: ti}
}

func (m networkModel) Init() tea.Cmd { return textinput.Blink }

func (m networkModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if m.state >= sCreateNet && m.state != sPickAuth {
			var cmd tea.Cmd
			m.ti, cmd = m.ti.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.state {
	case sPickMode:
		return m.updatePickMode(key)
	case sPickNet:
		return m.updatePickNet(key)
	case sPickAuth:
		return m.updatePickAuth(key)
	default:
		return m.updateInput(key)
	}
}

func (m networkModel) updatePickMode(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 1 {
			m.cursor++
		}
	case "enter", " ":
		if m.cursor == 0 {
			m.mode = NetworkModeTraefik
			m.cursor = 0
			m.state = sPickNet
		} else {
			m.mode = NetworkModePort
			m.ti.Placeholder = "3000"
			m.ti.SetValue("3000")
			m.ti.Focus()
			m.state = sEnterPort
			return m, textinput.Blink
		}
	case "ctrl+c", "q":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m networkModel) updatePickNet(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxIdx := len(m.networks) // last item is "Create new"
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < maxIdx {
			m.cursor++
		}
	case "enter", " ":
		if m.cursor == maxIdx {
			m.ti.Placeholder = "traefik-network"
			m.ti.SetValue("")
			m.ti.Focus()
			m.state = sCreateNet
			return m, textinput.Blink
		}
		m.netName = m.networks[m.cursor]
		m.ti.Placeholder = "deckplane.example.com"
		m.ti.SetValue("")
		m.ti.Focus()
		m.state = sEnterHost
		return m, textinput.Blink
	case "esc":
		m.cursor = 0
		m.state = sPickMode
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m networkModel) updatePickAuth(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
	case "enter", " ":
		switch m.cursor {
		case 0: // Skip
			m.authentikCfg = &AuthentikConfig{Mode: AuthentikSkip}
			return m, tea.Quit
		case 1: // Existing
			m.ti.Placeholder = "https://auth.example.com"
			m.ti.SetValue("")
			m.ti.Focus()
			m.state = sEnterAuthURL
			return m, textinput.Blink
		case 2: // Install
			m.authentikCfg = &AuthentikConfig{Mode: AuthentikInstall}
			return m, tea.Quit
		}
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m networkModel) updateInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	case "esc":
		m.ti.Blur()
		switch m.state {
		case sCreateNet, sEnterHost:
			m.cursor = 0
			m.state = sPickNet
		case sEnterCertResolver:
			m.ti.SetValue(m.host)
			m.state = sEnterHost
		case sEnterPort:
			m.cursor = 1
			m.state = sPickMode
		case sEnterAuthURL:
			m.cursor = 1
			m.state = sPickAuth
		}
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.ti.Value())
		switch m.state {
		case sCreateNet:
			if val == "" {
				val = "traefik-network"
			}
			m.netName = val
			m.ti.Placeholder = "deckplane.example.com"
			m.ti.SetValue("")
			m.state = sEnterHost
			return m, textinput.Blink
		case sEnterHost:
			m.host = val
			m.ti.Placeholder = "le"
			m.ti.SetValue("")
			m.state = sEnterCertResolver
			return m, textinput.Blink
		case sEnterCertResolver:
			certResolver := val
			if certResolver == "" {
				certResolver = "le"
			}
			m.config = &NetworkConfig{
				Mode:         NetworkModeTraefik,
				NetworkName:  m.netName,
				Host:         m.host,
				CertResolver: certResolver,
				CreateNet:    !m.netExists(m.netName),
			}
			m.ti.Blur()
			m.cursor = 0
			m.state = sPickAuth
			return m, nil
		case sEnterPort:
			port := 3000
			fmt.Sscanf(val, "%d", &port)
			if port <= 0 {
				port = 3000
			}
			m.config = &NetworkConfig{Mode: NetworkModePort, Port: port}
			m.ti.Blur()
			m.cursor = 0
			m.state = sPickAuth
			return m, nil
		case sEnterAuthURL:
			if val == "" {
				val = "https://auth.example.com"
			}
			m.authentikCfg = &AuthentikConfig{Mode: AuthentikExisting, ExistingURL: val}
			return m, tea.Quit
		}
	default:
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(k)
		return m, cmd
	}
	return m, nil
}

func (m networkModel) netExists(name string) bool {
	for _, n := range m.networks {
		if n == name {
			return true
		}
	}
	return false
}

func (m networkModel) View() string {
	var b strings.Builder

	switch m.state {
	case sPickMode:
		b.WriteString(sTitle.Render("Deckplane — Networking Setup") + "\n\n")
		b.WriteString("How should the control plane be exposed?\n\n")
		opts := []struct{ label, hint string }{
			{"Traefik reverse proxy", "recommended — no host port needed"},
			{"Direct port binding", "exposes a port on this machine"},
		}
		for i, o := range opts {
			cursor := "  "
			style := sNorm
			if i == m.cursor {
				cursor = "▸ "
				style = sSel
			}
			b.WriteString(style.Render(cursor+o.label) + "  " + sSubtle.Render(o.hint) + "\n")
		}
		b.WriteString("\n" + sSubtle.Render("↑/↓  ·  enter  ·  q quit"))

	case sPickNet:
		b.WriteString(sTitle.Render("Select Docker Network") + "\n")
		b.WriteString(sSubtle.Render("Traefik must be attached to this network") + "\n\n")
		for i, net := range m.networks {
			cursor := "  "
			style := sNorm
			if i == m.cursor {
				cursor = "▸ "
				style = sSel
			}
			hint := ""
			if strings.Contains(strings.ToLower(net), "traefik") {
				hint = "  " + sDim.Render("← traefik")
			}
			b.WriteString(style.Render(cursor+net) + hint + "\n")
		}
		cursor := "  "
		style := sDim
		if m.cursor == len(m.networks) {
			cursor = "▸ "
			style = sSel
		}
		b.WriteString(style.Render(cursor+"+ Create new network...") + "\n")
		b.WriteString("\n" + sSubtle.Render("↑/↓  ·  enter  ·  esc back"))

	case sCreateNet:
		b.WriteString(sTitle.Render("New Network Name") + "\n\n")
		b.WriteString("Name: " + m.ti.View() + "\n\n")
		b.WriteString(sSubtle.Render("enter  ·  esc back"))

	case sEnterHost:
		b.WriteString(sTitle.Render("Traefik Hostname") + "\n")
		b.WriteString(sSubtle.Render("Network: "+m.netName) + "\n\n")
		b.WriteString("Host: " + m.ti.View() + "\n\n")
		b.WriteString(sSubtle.Render("enter  ·  esc back"))

	case sEnterCertResolver:
		b.WriteString(sTitle.Render("TLS Cert Resolver") + "\n")
		b.WriteString(sSubtle.Render("Host: "+m.host) + "\n\n")
		b.WriteString("Certresolver: " + m.ti.View() + "\n")
		b.WriteString(sSubtle.Render("The name from your Traefik config — check other services' labels") + "\n\n")
		b.WriteString(sSubtle.Render("enter  ·  esc back"))

	case sEnterPort:
		b.WriteString(sTitle.Render("Direct Port Binding") + "\n\n")
		b.WriteString("Port: " + m.ti.View() + "\n\n")
		b.WriteString(sSubtle.Render("enter  ·  esc back"))

	case sPickAuth:
		b.WriteString(sTitle.Render("Authentik SSO (optional)") + "\n\n")
		b.WriteString("Set up single sign-on with Authentik?\n\n")
		opts := []struct{ label, hint string }{
			{"Skip for now", "no SSO — you can configure it later"},
			{"I have Authentik already", "enter its URL to pre-fill .env"},
			{"Install Authentik for me", "adds Authentik to the Compose stack"},
		}
		for i, o := range opts {
			cursor := "  "
			style := sNorm
			if i == m.cursor {
				cursor = "▸ "
				style = sSel
			}
			b.WriteString(style.Render(cursor+o.label) + "  " + sSubtle.Render(o.hint) + "\n")
		}
		b.WriteString("\n" + sSubtle.Render("↑/↓  ·  enter  ·  ctrl+c quit"))

	case sEnterAuthURL:
		b.WriteString(sTitle.Render("Existing Authentik URL") + "\n\n")
		b.WriteString("URL: " + m.ti.View() + "\n\n")
		b.WriteString(sSubtle.Render("enter  ·  esc back"))
	}

	return sBox.Render(b.String())
}
