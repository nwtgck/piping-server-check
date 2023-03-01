package ui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fatih/color"
	"github.com/nwtgck/piping-server-check/check"
	"golang.org/x/exp/slices"
	"strings"
)

type Model struct {
	results               []check.Result
	CompromiseResultNames []string
}

type UpdateResultsMsg struct {
	Results []check.Result
}

func (Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	case UpdateResultsMsg:
		m.results = msg.Results
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	var output string
	var lastMainName string
	var lastProtocol check.Protocol
	for _, result := range m.results {
		mainName, subName := splitResultName(result.Name)
		if lastMainName != mainName || lastProtocol != result.Protocol {
			output += fmt.Sprintf("%s %s\n", mainName, result.Protocol)
			lastMainName = mainName
			lastProtocol = result.Protocol
		}
		if len(result.Errors) != 0 {
			if slices.Contains(m.CompromiseResultNames, result.Name) {
				// TODO: show error
				output += color.MagentaString(fmt.Sprintf("  ✖︎ %s\n", subName))
			} else {
				// TODO: show error
				output += color.RedString(fmt.Sprintf("  ✖︎ %s\n", subName))
			}
		} else if len(result.Warnings) != 0 {
			// TODO: show warning
			output += color.YellowString(fmt.Sprintf("  ⚠︎ %s\n", subName))
		} else {
			output += color.GreenString(fmt.Sprintf("  ✔︎ %s\n", subName))
		}
	}
	return output
}

func splitResultName(name string) (string, string) {
	s := strings.SplitN(name, ".", 2)
	if len(s) == 1 {
		return s[0], "result"
	}
	return s[0], s[1]
}
