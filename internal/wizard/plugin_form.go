// ExitBox - Multi-Agent Container Sandbox
// Copyright (C) 2026 Cloud Exit B.V.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package wizard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloud-exit/exitbox/internal/config"
	"github.com/cloud-exit/exitbox/internal/plugin"
)

// PluginFormResult carries the outcome of a plugin configuration menu.
type PluginFormResult struct {
	Enabled  bool
	Settings map[string]string
	Saved    bool
}

// PluginForm is the configuration menu of a single plugin. It is used by
// `exitbox plugins edit` and by the plugins step of the setup wizard.
type PluginForm struct {
	agent    string
	plugin   *plugin.Plugin
	enabled  bool
	settings map[string]string

	cursor  int
	saved   bool
	quit    bool
	status  string
	editing bool
	buffer  string          // key of the setting being edited
	text    string          // text buffer while editing a value
	choices []string        // option list while editing an enum/multi value
	chosen  map[string]bool // selected options while editing a multi value

	cursorInChoices int
}

// NewPluginForm builds the menu for one plugin and its stored entry.
func NewPluginForm(agent string, p *plugin.Plugin, entry config.PluginEntry) *PluginForm {
	settings := map[string]string{}
	for k, v := range p.DefaultSettings() {
		settings[k] = v
	}
	for k, v := range entry.Settings {
		settings[k] = v
	}
	return &PluginForm{
		agent:    agent,
		plugin:   p,
		enabled:  entry.Enabled,
		settings: settings,
	}
}

// Done reports whether the menu has finished, either saved or cancelled.
func (f *PluginForm) Done() bool { return f.saved || f.quit }

// Result returns the configuration the menu ended on.
func (f *PluginForm) Result() PluginFormResult {
	return PluginFormResult{Enabled: f.enabled, Settings: f.settings, Saved: f.saved}
}

// RunPluginForm shows the configuration menu for a plugin and returns the
// edited configuration. Saved is false when the user cancelled.
func RunPluginForm(agent string, p *plugin.Plugin, entry config.PluginEntry) (PluginFormResult, error) {
	form := NewPluginForm(agent, p, entry)
	prog := tea.NewProgram(form)
	model, err := prog.Run()
	if err != nil {
		return PluginFormResult{}, err
	}
	if done, ok := model.(*PluginForm); ok {
		return done.Result(), nil
	}
	return form.Result(), nil
}

// visibleSettings returns the settings that currently apply, so a provider
// key field only shows up once its provider is selected.
func (f *PluginForm) visibleSettings() []plugin.Setting {
	var out []plugin.Setting
	for _, s := range f.plugin.Settings {
		if s.Visible(f.settings) {
			out = append(out, s)
		}
	}
	return out
}

// rows returns the menu rows: the enable toggle followed by every visible
// setting.
func (f *PluginForm) rows() []plugin.Setting {
	return append([]plugin.Setting{{Key: "", Label: "Enabled", Type: plugin.TypeBool}}, f.visibleSettings()...)
}

func (f *PluginForm) Init() tea.Cmd { return nil }

func (f *PluginForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	if f.editing {
		return f.updateEditing(key)
	}
	return f.updateBrowsing(key)
}

func (f *PluginForm) updateBrowsing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := f.rows()
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		f.quit = true
		return f, tea.Quit
	case "up", "k":
		if f.cursor > 0 {
			f.cursor--
		}
	case "down", "j":
		if f.cursor < len(rows)-1 {
			f.cursor++
		}
	case "s", "ctrl+s":
		f.saved = true
		return f, tea.Quit
	case "enter", " ":
		if f.cursor >= len(rows) {
			return f, nil
		}
		row := rows[f.cursor]
		if row.Key == "" {
			f.enabled = !f.enabled
			return f, nil
		}
		f.beginEdit(row)
	}
	return f, nil
}

// beginEdit opens the right editor for a setting: booleans toggle in place,
// lists open a picker, and everything else opens a text field.
func (f *PluginForm) beginEdit(s plugin.Setting) {
	switch s.Type {
	case plugin.TypeBool:
		if f.settings[s.Key] == "false" {
			f.settings[s.Key] = "true"
		} else {
			f.settings[s.Key] = "false"
		}
	case plugin.TypeEnum, plugin.TypeMulti:
		f.editing = true
		f.choices = s.Options
		f.chosen = map[string]bool{}
		for _, item := range plugin.SplitMulti(f.settings[s.Key]) {
			f.chosen[item] = true
		}
		f.buffer = s.Key
		f.cursorInChoices = 0
	default:
		f.editing = true
		f.choices = nil
		f.buffer = s.Key
		f.text = f.displayValue(s)
	}
	f.status = ""
}

func (f *PluginForm) updateEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	setting := f.plugin.Setting(f.buffer)
	if setting == nil {
		f.editing = false
		return f, nil
	}
	if f.choices != nil {
		return f.updateChoice(msg, setting)
	}
	return f.updateText(msg, setting)
}

func (f *PluginForm) updateChoice(msg tea.KeyMsg, s *plugin.Setting) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		f.quit = true
		return f, tea.Quit
	case "esc":
		f.closeEditor()
	case "up", "k":
		if f.cursorInChoices > 0 {
			f.cursorInChoices--
		}
	case "down", "j":
		if f.cursorInChoices < len(f.choices)-1 {
			f.cursorInChoices++
		}
	case " ":
		if s.Type == plugin.TypeMulti && f.cursorInChoices < len(f.choices) {
			option := f.choices[f.cursorInChoices]
			f.chosen[option] = !f.chosen[option]
		}
	case "enter":
		if s.Type == plugin.TypeEnum {
			if f.cursorInChoices < len(f.choices) {
				f.settings[s.Key] = f.choices[f.cursorInChoices]
			}
		} else {
			var selected []string
			for _, option := range f.choices {
				if f.chosen[option] {
					selected = append(selected, option)
				}
			}
			f.settings[s.Key] = strings.Join(selected, ",")
		}
		f.closeEditor()
	}
	return f, nil
}

func (f *PluginForm) updateText(msg tea.KeyMsg, s *plugin.Setting) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		f.quit = true
		return f, tea.Quit
	case "esc":
		f.closeEditor()
	case "enter":
		value := s.Normalize(f.text)
		if err := s.Validate(value); err != nil {
			f.status = err.Error()
			return f, nil
		}
		if value == "" {
			delete(f.settings, s.Key)
		} else {
			f.settings[s.Key] = value
		}
		f.closeEditor()
	case "backspace", "ctrl+h":
		if len(f.text) > 0 {
			f.text = f.text[:len(f.text)-1]
		}
	default:
		if len(msg.String()) == 1 {
			f.text += msg.String()
		}
	}
	return f, nil
}

func (f *PluginForm) closeEditor() {
	f.editing = false
	f.choices = nil
	f.chosen = nil
	f.text = ""
	f.buffer = ""
	f.cursorInChoices = 0
}

// displayValue renders a setting's current value for the menu. Secrets show
// their vault key, never a secret value: none is ever stored here.
func (f *PluginForm) displayValue(s plugin.Setting) string {
	value := f.settings[s.Key]
	if value == "" {
		return ""
	}
	if s.Type == plugin.TypeSecret {
		if key, ok := plugin.VaultKey(value); ok {
			return key
		}
	}
	return value
}

func (f *PluginForm) View() string {
	if f.quit || f.saved {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s settings (%s)", f.plugin.Display, f.agent)) + "\n")
	b.WriteString(subtitleStyle.Render(f.plugin.Description) + "\n\n")

	if f.editing {
		b.WriteString(f.viewEditor())
		return b.String()
	}

	for i, row := range f.rows() {
		cursor := "  "
		if i == f.cursor {
			cursor = cursorStyle.Render("> ")
		}
		label := row.Label
		value := ""
		switch {
		case row.Key == "":
			value = boolLabel(f.enabled)
		case row.Type == plugin.TypeSecret:
			shown := f.displayValue(row)
			if shown == "" {
				value = dimStyle.Render("(not set)")
			} else {
				value = "vault: " + shown
			}
		default:
			shown := f.displayValue(row)
			if shown == "" {
				value = dimStyle.Render("(unset)")
			} else {
				value = shown
			}
		}
		b.WriteString(fmt.Sprintf("%s%-24s %s\n", cursor, label, value))
		if i == f.cursor && row.Help != "" {
			b.WriteString(dimStyle.Render("    "+row.Help) + "\n")
		}
	}

	if f.status != "" {
		b.WriteString("\n" + warnStyle.Render(f.status) + "\n")
	}
	b.WriteString(helpStyle.Render("↑/↓ move · enter edit · s save · esc cancel"))
	return b.String()
}

func (f *PluginForm) viewEditor() string {
	setting := f.plugin.Setting(f.buffer)
	if setting == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(selectedStyle.Render(setting.Label) + "\n")
	if setting.Help != "" {
		b.WriteString(dimStyle.Render(setting.Help) + "\n")
	}
	b.WriteString("\n")

	if f.choices != nil {
		for i, option := range f.choices {
			cursor := "  "
			if i == f.cursorInChoices {
				cursor = cursorStyle.Render("> ")
			}
			mark := "[ ]"
			if setting.Type == plugin.TypeEnum {
				mark = "( )"
				if f.settings[setting.Key] == option {
					mark = "(•)"
				}
			} else if f.chosen[option] {
				mark = "[x]"
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, mark, option))
		}
		hint := "↑/↓ move · space toggle · enter confirm · esc cancel"
		if setting.Type == plugin.TypeEnum {
			hint = "↑/↓ move · enter select · esc cancel"
		}
		b.WriteString(helpStyle.Render(hint))
		return b.String()
	}

	prompt := "Value"
	if setting.Type == plugin.TypeSecret {
		prompt = "Vault key"
	}
	b.WriteString(fmt.Sprintf("%s: %s\n", prompt, f.text+"_"))
	if setting.Type == plugin.TypeSecret {
		hint := "Stores only the key name; set the secret with: exitbox vault set <KEY> <VALUE>"
		if setting.VaultKey != "" {
			hint = "Suggested key: " + setting.VaultKey + " · " + hint
		}
		b.WriteString(dimStyle.Render(hint) + "\n")
	}
	if f.status != "" {
		b.WriteString("\n" + warnStyle.Render(f.status) + "\n")
	}
	b.WriteString(helpStyle.Render("enter save · esc cancel"))
	return b.String()
}

func boolLabel(v bool) string {
	if v {
		return successStyle.Render("yes")
	}
	return dimStyle.Render("no")
}
