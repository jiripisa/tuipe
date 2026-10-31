package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/ui/text"
	"github.com/jiripisa/tuipe/internal/upgrade"
)

// updateCheckTimeout caps the background query, so a hanging network never
// leaves work behind.
const updateCheckTimeout = 2 * time.Second

// UpdateAvailableMsg reports a release newer than the running one.
type UpdateAvailableMsg struct{ Version string }

// updateCheckedMsg is the end of a check attempt. Tag is empty when it failed.
type updateCheckedMsg struct{ Tag string }

// updateCheck is what the app needs to look for a newer release.
type updateCheck struct {
	client    *upgrade.Client
	isRelease bool
	current   string
}

// WithUpdateCheck sets the release client, whether this binary is a release
// build and the version it is running. Only release builds ever check.
func WithUpdateCheck(c *upgrade.Client, isRelease bool, current string) Option {
	return func(a *App) { a.update = updateCheck{c, isRelease, current} }
}

// UpdateCheckCmd asks for the latest release in the background, giving up
// after 2 seconds. Every failure is silent; it always ends in an updateCheckedMsg
// so the attempt is counted for the day.
func UpdateCheckCmd(a *App) tea.Cmd {
	client := a.update.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		r, err := client.Latest(ctx)
		if err != nil {
			return updateCheckedMsg{}
		}
		return updateCheckedMsg{Tag: r.Tag}
	}
}

// updateEnabled reports whether this run may look for updates at all.
func (a *App) updateEnabled() bool {
	return a.update.isRelease && a.Store.Config().UpdateCheck
}

// initUpdate shows the newest release seen earlier when it is still newer than
// the running one, and starts today's check when it is due.
func (a *App) initUpdate() tea.Cmd {
	if !a.updateEnabled() {
		return nil
	}
	p := a.Store.Profile()
	a.showUpdate(p.LatestSeen)
	if p.LastUpdateCheck == a.today() {
		return nil
	}
	return UpdateCheckCmd(a)
}

// showUpdate sets the home notice when version is newer than the running one.
func (a *App) showUpdate(version string) {
	if version != "" && upgrade.Newer(a.update.current, version) {
		a.UpdateNotice = fmt.Sprintf(text.UpdateNotice, version)
	}
}

// updateChecked records a finished attempt. A failed save stays silent.
func (a *App) updateChecked(m updateCheckedMsg) tea.Cmd {
	p := a.Store.Profile()
	p.LastUpdateCheck = a.today()
	if m.Tag != "" {
		p.LatestSeen = m.Tag
	}
	_ = a.Store.SaveProfile()
	if m.Tag == "" || !upgrade.Newer(a.update.current, m.Tag) {
		return nil
	}
	return func() tea.Msg { return UpdateAvailableMsg{Version: m.Tag} }
}

func defaultUpdateCheck() updateCheck {
	return updateCheck{upgrade.NewClient(), buildinfo.IsRelease(), buildinfo.Effective()}
}
