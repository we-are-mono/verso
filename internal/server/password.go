// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/units"
	"github.com/we-are-mono/verso/internal/widget"
)

const minPasswordLen = 8

// credentialVerifier is narrower than Authenticator. Login retains an rpcd sid;
// password confirmation only needs a yes/no answer and destroys its probe sid.
type credentialVerifier interface {
	Verify(context.Context, string, string) error
}

// Access is shell-owned (ADR-009 §3). It changes the credential that gates
// Verso and manages Verso's sessions. Both actions are immediate and never
// create UCI stage entries; an unrelated existing global stage may still appear.

func accessForm(hasPassword bool, username string, fieldErrs map[string]string, formErr string) *widget.Form {
	fields := make([]widget.Widget, 0, 4)
	// The account the password belongs to, for the browser's password manager.
	fields = append(fields, &widget.Field{Name: "username", Kind: "hidden", Value: username, Autocomplete: "username"})
	if hasPassword {
		fields = append(fields, &widget.Field{Name: "current_password", Label: "Current password", Kind: "password", Autocomplete: "current-password", Error: fieldErrs["current_password"]})
	}
	fields = append(fields, &widget.Grid{Style: "form", Columns: 2, Label: "New password", Children: []widget.Widget{
		&widget.Field{Name: "password", Label: "New password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["password"]},
		&widget.Field{Name: "confirm", Label: "Repeat new password", Kind: "password", Autocomplete: "new-password", Error: fieldErrs["confirm"]},
	}})
	label := "Set password"
	if hasPassword {
		label = "Change password"
	}
	return &widget.Form{Style: "settings", Action: "/system/access", Submit: label, Error: formErr, Fields: fields}
}

// accessBody is the password form over the sessions signed in now, kept to
// those g lets the session see: where rpcd did not answer for another
// account's, the list says the permissions could not be checked.
func accessBody(hasPassword bool, username string, fieldErrs map[string]string, formErr string, sessions []accessSession, g *sessionGrants, tr func(string) string) *widget.Stack {
	table := accessSessionsTable(sessions, tr)
	return &widget.Stack{Children: []widget.Widget{
		&widget.Section{Title: "Router password", Children: []widget.Widget{accessForm(hasPassword, username, fieldErrs, formErr)}},
		&widget.Section{Title: "Signed in now", Hairline: true, Children: append(uncheckedNotice(g, table), table)},
	}}
}

type accessSession struct {
	ID, Browser, Address, Since, LastActive string
	Current                                 bool
}

// accessSessionsTable lists who is signed in: where from, with the browser
// beside the address, and when last seen, with when it started beside that.
// As a list, each detail rides its value's line where there is room and
// wraps under it where there is not, so a full IPv6 address and the way to
// revoke still fit a phone. Every session revokes the same way and asks
// first; revoking this browser's own is logging out, and the question says so.
func accessSessionsTable(sessions []accessSession, tr func(string) string) *widget.Table {
	rows := make([]widget.TableRow, 0, len(sessions))
	for _, session := range sessions {
		source := widget.TableCell{Text: session.Address, Detail: session.Browser}
		// The act's words are the walk's to translate, like any table act's.
		action := widget.TableCell{
			Actions:       []widget.TableRowAct{{Icon: "log-out", Title: "Revoke session", Name: "_action", Value: "end-session:" + session.ID}},
			ConfirmTitle:  "Revoke this session?",
			Confirm:       "That browser is signed out at once and has to sign in again.",
			ConfirmCancel: "Keep session",
		}
		if session.Current {
			source.Tag = tr("this browser")
			source.TagVariant = "success"
			action.Confirm = "This browser is signed out at once, the same as Log out."
		}
		// The times were worded in the reader's language as they were composed
		// (accessSessions), so the walk leaves them.
		seen := widget.TableCell{Text: session.LastActive, Detail: session.Since, Verbatim: true}
		rows = append(rows, widget.TableRow{ID: session.ID, Cells: []widget.TableCell{source, seen, action}})
	}
	// Who is signed in is a few facts about this router, not a listing of its
	// config, so it stands in the card's inset as a list.
	return &widget.Table{Style: "list", Columns: []widget.TableColumn{{Label: "Source", Kind: "reference"}, {Label: "Last seen", Kind: "text"}, {Kind: "actions"}}, Rows: rows}
}

// accessSessions is the sessions signed in now that g lets the request's
// session list: its own account's, and another's under rpcd's session list.
func (s *Server) accessSessions(r *http.Request, g *sessionGrants) []accessSession {
	current, _ := s.currentSession(r)
	now := time.Now()
	items := slices.DeleteFunc(s.sessions.list(), func(sess session) bool {
		return !mayManageSession(r.Context(), g, current, sess, sessionListGrant)
	})
	sort.Slice(items, func(i, j int) bool {
		if items[i].id == current.id {
			return true
		}
		if items[j].id == current.id {
			return false
		}
		return items[i].lastSeen.After(items[j].lastSeen)
	})
	// The sessions table sets these in machine-kind columns the schema walk
	// leaves verbatim, and they compose prose with data — so they localize
	// here, at the point of composition.
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	out := make([]accessSession, 0, len(items))
	for _, sess := range items {
		address := sess.address
		if address == "" {
			address = "—"
		}
		out = append(out, accessSession{
			ID: sess.id, Browser: browserLabel(tr, sess.agent), Address: address,
			Since: sessionSince(tr, sess.created, now), LastActive: relativeSessionTime(tr, sess.lastSeen, now),
			Current: sess.id == current.id,
		})
	}
	return out
}

func browserLabel(tr func(string) string, ua string) string {
	browser := tr("Unknown browser")
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	platform := ""
	switch {
	case strings.Contains(ua, "iPhone"):
		platform = "iPhone"
	case strings.Contains(ua, "iPad"):
		platform = "iPad"
	case strings.Contains(ua, "Android"):
		platform = "Android"
	case strings.Contains(ua, "Windows"):
		platform = "Windows"
	case strings.Contains(ua, "Macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "Linux"):
		platform = "Linux"
	}
	if platform != "" {
		return browser + " · " + platform
	}
	return browser
}

// sessionSince says when a session started, as the line under when it was
// last seen: today by the clock alone, any other day by its date.
func sessionSince(tr func(string) string, created, now time.Time) string {
	created, now = clock.Local(created), clock.Local(now)
	cy, cm, cd := created.Date()
	ny, nm, nd := now.Date()
	if cy == ny && cm == nm && cd == nd {
		return fmt.Sprintf(tr("since today, %s"), created.Format("15:04"))
	}
	return fmt.Sprintf(tr("since %s"), created.Format("2 Jan, 15:04"))
}

// relativeSessionTime words a session's age, as every time ago is said
// (internal/units).
func relativeSessionTime(tr func(string) string, last, now time.Time) string {
	return units.For(tr).Ago(int64(now.Sub(last).Seconds()))
}

func (s *Server) handlePasswordForm(w http.ResponseWriter, r *http.Request) {
	// Access is where a change of the web interface's ports sends the
	// browser: reached on the new listeners, the old ones close.
	s.webArrived(r)
	s.renderAccess(w, r, http.StatusOK, nil, "")
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {

	if err := r.ParseForm(); err != nil {
		s.renderAccess(w, r, http.StatusBadRequest, nil, "Could not read the form.")
		return
	}
	if action := r.PostForm.Get("_action"); strings.HasPrefix(action, "end-session:") {
		s.handleEndSession(w, r, strings.TrimPrefix(action, "end-session:"))
		return
	}
	// The web interface's form posts here, so a refusal stays at Access.
	if r.PostForm.Has(webHTTPSField) {
		s.handleWebListeners(w, r)
		return
	}

	if q := r.URL.Query(); q.Get("plugin") != "" || q.Get(contributionQuery) != "" {
		post, ok := s.accessPost(r)
		if !ok {
			s.renderAccess(w, r, http.StatusBadRequest, nil, "These settings are no longer available. Reload the page.")
			return
		}
		s.renderAccess(w, post, http.StatusOK, nil, "")
		return
	}

	hasPassword, err := s.backend.RootHasPassword(r.Context(), s.sessionSID(r))
	if err != nil {
		s.renderAccess(w, r, http.StatusInternalServerError, nil, "Couldn’t check the administrator account just now.")
		return
	}
	fieldErrs := validatePassword(r.PostForm.Get("password"), r.PostForm.Get("confirm"))
	if hasPassword {
		current := r.PostForm.Get("current_password")
		switch verifier := s.auth.(type) {
		case credentialVerifier:
			if current == "" || verifier.Verify(r.Context(), s.sessionUser(r), current) != nil {
				fieldErrs["current_password"] = "Current password is incorrect."
			}
		default:
			fieldErrs["current_password"] = "Current password could not be verified."
		}
	}
	if len(fieldErrs) > 0 {
		s.renderAccess(w, r, http.StatusUnprocessableEntity, fieldErrs, "")
		return
	}

	password := r.PostForm.Get("password")
	// A session ended since the sign-in check, by another browser's change say,
	// names no account to change the password of.
	current, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, loginRedirect(r), http.StatusSeeOther)
		return
	}
	if err := s.backend.SetPassword(r.Context(), current.sid, current.username, password); err != nil {
		log.Printf("verso: set password failed: %v", err)
		s.renderAccess(w, r, http.StatusInternalServerError, nil, "The password couldn’t be changed just now. Try again in a moment.")
		return
	}
	// A browser signed in with the password just replaced is signed out, its
	// rpcd sid with it; the browser that changed it stays (ADR-007 §7).
	s.sessions.destroyAccountExcept(current.username, current.id)
	// Access is read again rather than drawn as the answer to the post, so a
	// reload never sends the password a second time; how it went arrives with
	// it, as the notification.
	s.flash(r, "success", "Password updated.")
	http.Redirect(w, r, "/system/access", http.StatusSeeOther)
}

// Another account's sessions are rpcd's to manage (ADR-007): a session lists
// and ends its own account's, and another account's only under the grants
// rpcd's own session object needs to list and to destroy sessions.
var (
	sessionListGrant    = aclProbe{"ubus", "session", "list"}
	sessionDestroyGrant = aclProbe{"ubus", "session", "destroy"}
)

// mayManageSession reports whether current may act on sess under grant: any
// session of its own account, another's only as rpcd grants.
func mayManageSession(ctx context.Context, g *sessionGrants, current, sess session, grant aclProbe) bool {
	return sess.username == current.username || g.may(ctx, grant)
}

// handleEndSession revokes one session. This browser's own is logging out, so
// it ends the way Log out does: the cookie goes and the browser lands on
// sign-in. Another account's session it may not end answers as one not there.
func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request, id string) {
	current, _ := s.currentSession(r)
	if id != "" && id == current.id {
		s.handleLogout(w, r)
		return
	}
	target, found := s.sessions.byID(id)
	if id == "" || !found || !mayManageSession(r.Context(), s.grantsFor(current.sid), current, target, sessionDestroyGrant) ||
		!s.sessions.destroyID(id) {
		s.renderAccess(w, r, http.StatusBadRequest, nil, "That session could not be ended.")
		return
	}
	s.flash(r, "success", "Session ended.")
	http.Redirect(w, r, "/system/access", http.StatusSeeOther)
}

// accessContribution is the plugin's part of Access: its contribution to
// Access's blocks, a system_access part among them.
func accessContribution(m plugin.Manifest) (plugin.Contribution, bool) {
	for _, c := range m.Contributions() {
		if c.Slot == plugin.AccessBlocks {
			return c, true
		}
	}
	return plugin.Contribution{}, false
}

// accessPost names, on a post to Access, the contribution it goes to. A part
// written before slots posts naming its plugin (?plugin=<id>), which is read
// as that plugin's part of Access. It reports false for a post to a part that
// is not there.
func (s *Server) accessPost(r *http.Request) (*http.Request, bool) {
	q := r.URL.Query()
	if id := q.Get("plugin"); id != "" && q.Get(contributionQuery) == "" {
		m, ok := s.manifestByID(id)
		c, part := accessContribution(m)
		if !ok || !part {
			return r, false
		}
		q.Set(contributionQuery, c.Name(id))
		r = r.Clone(r.Context())
		r.URL.RawQuery = q.Encode()
	}
	name := q.Get(contributionQuery)
	if name == "" {
		return r, true
	}
	return r, slices.ContainsFunc(s.slotFills(plugin.AccessBlocks), func(p slotPlacement) bool { return p.Name() == name })
}

func validatePassword(password, confirm string) map[string]string {
	errs := map[string]string{}
	switch {
	case password == "":
		errs["password"] = "Enter a new password."
	case len(password) < minPasswordLen:
		errs["password"] = "Use at least 8 characters."
	case confirm != password:
		errs["confirm"] = "Passwords do not match."
	}
	return errs
}

func (s *Server) renderAccess(w http.ResponseWriter, r *http.Request, status int, fieldErrs map[string]string, formErr string) {
	hasPassword, err := s.backend.RootHasPassword(r.Context(), s.sessionSID(r))
	if err != nil {
		hasPassword = true
		if formErr == "" {
			formErr = "Couldn’t check the administrator account just now."
		}
	}
	username := s.sessionUser(r)
	if username == "" {
		username = "root"
	}
	_, t := s.localize(r)
	g := s.grantsFor(s.sessionSID(r))
	access := accessBody(hasPassword, username, fieldErrs, formErr, s.accessSessions(r, g), g, translatorOrIdentity(t))
	hdr := pageHeader{Heading: "Access", Tone: "neutral"}
	r = withStageMemo(r)
	// The web interface's own subjects: where it answers, opened by whether
	// the internet reaches it, then LuCI's part, where LuCI stands beside it.
	web := s.webSettingsSection(r, fieldErrs)
	if warn := s.exposureCallout(r, translatorOrIdentity(t)); warn != nil {
		if section, ok := web.(*widget.Section); ok {
			section.Children = append([]widget.Widget{warn}, section.Children...)
		} else {
			web = warn
		}
	}
	own := map[string]widget.Widget{"password": access.Children[0], "sessions": access.Children[1], "web": web}
	if luci := s.luciSection(r); luci != nil {
		own["luci"] = luci
	}
	// Access's blocks are a slot (ADR-026): each plugin's part stands where
	// its position puts it, one more block of the page's stack, which stands
	// its first section's rule two cells under the block before it.
	page, call := contributionPost(r)
	call.page = "/system/access"
	access.Children = s.shellSlotBlocks(page, call, plugin.AccessBlocks, own)
	if s.answerContributionPost(w, r, call.outcome) {
		return
	}
	if o := call.outcome; o != nil && o.posted {
		if o.code >= http.StatusBadRequest {
			status = o.code
		}
		hdr.Notice = o.hdr.Notice
	}

	s.renderWidgetPage(w, r, status, hdr, "form", s.sectionPages("System", r.URL.Path), access)
}
