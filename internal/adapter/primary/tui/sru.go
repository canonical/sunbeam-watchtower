// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	frontend "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/runtime"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

type sruModel struct {
	filter   dto.SRUFilter
	snapshot *dto.SRUSnapshot
	index    int
	err      string
}

func loadSRUCmd(session *runtimeadapter.Session, filter dto.SRUFilter) tea.Cmd {
	return guardSessionAction(session, frontend.ActionSRUList, func() tea.Msg {
		snapshot, err := session.Frontend.SRU().List(context.Background(), filter)
		return sruLoadedMsg{snapshot: snapshot, err: err}
	})
}

func refreshSRUCmd(session *runtimeadapter.Session, filter dto.SRUFilter) tea.Cmd {
	return guardSessionAction(session, frontend.ActionCacheSyncSRU, func() tea.Msg {
		_, err := session.Frontend.Cache().SyncSRU(context.Background())
		if err != nil {
			return sruLoadedMsg{err: err}
		}
		snapshot, err := session.Frontend.SRU().List(context.Background(), filter)
		return sruLoadedMsg{snapshot: snapshot, err: err}
	})
}

func openSRUBugCmd(session *runtimeadapter.Session, rawURL string) tea.Cmd {
	return guardSessionAction(session, frontend.ActionSRUOpen, func() tea.Msg {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "bugs.launchpad.net" || parsed.User != nil || parsed.Port() != "" {
			return browserOpenedMsg{err: fmt.Errorf("invalid Launchpad bug URL")}
		}
		return browserOpenedMsg{err: openBrowser(rawURL)}
	})
}

func (m rootModel) renderSRU() string {
	const gap = 1
	listWidth, detailWidth := splitColumns(m.width, gap)
	header := "SRU monitor  / filters  r reload snapshot  R refresh Launchpad  O open bug"
	if m.sru.snapshot != nil {
		counts := map[string]int{}
		for _, row := range m.sru.snapshot.Rows {
			counts[row.Stage]++
		}
		header += fmt.Sprintf("\nSynced %s  targets=%d  not observed=%d  review=%d  proposed=%d  updates=%d",
			m.sru.snapshot.SyncedAt.Format("2006-01-02 15:04 UTC"), len(m.sru.snapshot.Rows),
			counts["not observed"], counts["unapproved"]+counts["staging"], counts["proposed"], counts["updates"])
	}
	f := m.sru.filter
	header += fmt.Sprintf("\npackage=%s  set=%s  archive=%s  series=%s  stage=%s  status=%s  verification=%s  bug=%s  attention=%t",
		emptyAsAny(f.Package), emptyAsAny(f.Set), emptyAsAny(f.Archive), emptyAsAny(f.Series),
		emptyAsAny(f.Stage), emptyAsAny(f.TaskStatus), emptyAsAny(f.Verification), emptyAsAny(f.BugID), f.NeedsAttention)
	if m.sru.err != "" {
		header += "\n" + m.theme.errorText.Render(m.sru.err)
	}
	var lines []string
	var detail string
	if m.sru.snapshot != nil {
		for i, row := range m.sru.snapshot.Rows {
			line := fmt.Sprintf("#%s %s %s/%s %s %s %s", row.BugID, row.Package, row.Archive,
				row.Series, row.TaskStatus, row.Stage, row.Verification)
			line = fitLine(line, innerPanelWidth(m.theme.panel, listWidth))
			if i == m.sru.index {
				line = m.theme.selectedRow.Render(line)
			}
			lines = append(lines, line)
		}
		if m.sru.index >= 0 && m.sru.index < len(m.sru.snapshot.Rows) {
			row := m.sru.snapshot.Rows[m.sru.index]
			detailLines := []string{
				fmt.Sprintf("Bug #%s: %s", row.BugID, row.Title),
				fmt.Sprintf("Package: %s", row.Package),
				fmt.Sprintf("Package sets: %s", strings.Join(row.PackageSets, ", ")),
				fmt.Sprintf("Target: %s/%s", row.Archive, row.Series),
				fmt.Sprintf("Task: %s", row.TaskStatus),
				fmt.Sprintf("Archive stage: %s  version: %s", row.Stage, row.Version),
				fmt.Sprintf("Verification: %s  tag: %s", row.Verification, row.VerificationTag),
				fmt.Sprintf("Parent: %s  required: %t  ready: %t", row.ParentSeries, row.ParentRequired, row.ParentReady),
				"Bug: " + row.BugURL, "Task: " + row.TaskURL,
			}
			for _, evidence := range row.Evidence {
				detailLines = append(detailLines, fmt.Sprintf("%s %s: %s", evidence.Stage, evidence.Version, evidence.URL))
			}
			if row.Warning != "" {
				detailLines = append(detailLines, "Warning: "+row.Warning)
			}
			detail = strings.Join(detailLines, "\n")
		}
	}
	if len(lines) == 0 {
		lines = []string{"No matching SRU targets."}
	}
	list := strings.Join(lines, "\n")
	if m.width >= 120 {
		left := renderPanel(m.theme.panel, listWidth, m.theme.panelTitle.Render("SRUs"), header+"\n\n"+list)
		right := renderPanel(m.theme.panel, detailWidth, m.theme.panelTitle.Render("Detail"), detail)
		return lipJoin(left, right, gap)
	}
	return lipVertical(m.theme, m.width, header, list, detail, "SRUs", "Detail")
}

func newSRUFilterForm(model sruModel) formModalModel {
	f := model.filter
	return newFormModal("SRU Filters", []fieldDef{
		{placeholder: "package", value: f.Package},
		{placeholder: "package set", value: f.Set},
		{placeholder: "archive", value: f.Archive, suggestions: []string{"ubuntu", "uca"}, kind: fieldKindEnum},
		{placeholder: "series", value: f.Series},
		{placeholder: "stage", value: f.Stage, suggestions: []string{"not observed", "unapproved", "staging", "proposed", "updates"}, kind: fieldKindEnum},
		{placeholder: "task status", value: f.TaskStatus},
		{placeholder: "verification", value: f.Verification, suggestions: []string{"none", "needed", "done", "failed"}, kind: fieldKindEnum},
		{placeholder: "bug ID", value: f.BugID},
		{placeholder: "needs attention", value: strconv.FormatBool(f.NeedsAttention), suggestions: []string{"false", "true"}, kind: fieldKindEnum},
	})
}

func (m rootModel) updateSRUFilterForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd := updateFormModal(msg, &m.sruFilterForm, func(values []string) tea.Cmd {
		attention, err := strconv.ParseBool(strings.TrimSpace(values[8]))
		if err != nil {
			m.sruFilterForm.errorMsg = "needs attention must be true or false"
			return nil
		}
		m.sru.filter = dto.SRUFilter{
			Package: strings.TrimSpace(values[0]), Set: strings.TrimSpace(values[1]),
			Archive: strings.TrimSpace(values[2]), Series: strings.TrimSpace(values[3]),
			Stage: strings.TrimSpace(values[4]), TaskStatus: strings.TrimSpace(values[5]),
			Verification: strings.TrimSpace(values[6]), BugID: strings.TrimSpace(values[7]),
			NeedsAttention: attention,
		}
		m.sru.index = 0
		m.overlay = overlayNone
		return loadSRUCmd(m.session, m.sru.filter)
	}, func() { m.overlay = overlayNone })
	return m, cmd
}
