// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/widget"
)

// The firmware-upgrade takeover — the one screen in Verso meant to fill the
// display with no menu around it. When a person commits to installing new
// firmware, the app gets out of the way: the sidebar, the top bar, every other
// affordance disappears, and what remains is a single calm surface that watches
// the upgrade and says the one thing that matters — keep the power on. There is
// nothing else to click because there is nothing else to do; that absence is the
// "you can't wander off" the flow needs.
//
// It is shell-owned and chrome-less (like login and "restore complete"), rendered
// straight through the page-template set rather than renderPage, so it carries no
// nav. And it is server-authoritative: while the job is in flight — or failed and
// unacknowledged — handleSystemMaintenance renders this instead of the ordinary
// page, so a reload, a second tab, and the back button all land back here. The
// only way out is the upgrade finishing or the person releasing a failure.
//
// The states are honest about what is known. While owut works — a build on the
// update server, a download, then the flash — there is no true percentage to
// show, so the surface pulses (the empty widget's accent icon) rather than lies.
// When the router goes down, that is success arriving, not an error: the client
// keeps polling, and reaching the server again on the new build is the signal to
// show "done". A failure stops the motion and opens a door back.

// firmwareTakeoverActive reports whether the firmware upgrade owns the screen:
// under way, spawned-and-restarting, or failed and not yet acknowledged. A clean
// "done" holds the screen too — on real hardware the flash takes this process
// down moments later, so a reload in that window must still land on the takeover
// rather than flash the ordinary page an instant before the router disappears.
func firmwareTakeoverActive() bool {
	// A person who left after the countdown ran out is not dragged back, even if
	// the background run is still going — the screen must never be a trap.
	if firmwareTakeoverReleased.Load() {
		return false
	}
	switch firmwareUpgrade.phase() {
	case jobRunning, jobDone, jobFailed:
		return true
	default:
		return false
	}
}

// upgradingView is the takeover template's model: each state pre-rendered to
// trusted HTML server-side (so the first paint is already the right state, before
// any JS), the id of the one to show first, and the localized JS-string blob the
// shell's client reads. The client swaps [hidden] between the working states as
// its poll of the status endpoint moves; a failure it reaches by reload, so the
// server renders that surface with the tool's own words already in place.
type upgradingView struct {
	Lang       string
	CSS        template.CSS
	JSStrings  template.JS
	Initial    string // "preparing" | "installing" | "failed"
	Preparing  template.HTML
	Installing template.HTML
	Restarting template.HTML
	Stalled    template.HTML
	Done       template.HTML
	Failed     template.HTML
}

// renderUpgrading paints the takeover. The initial surface is the job's current
// phase — running is still-preparing, a spawned install is restarting-soon, a
// failure is the failure — and the client takes it from there.
func (s *Server) renderUpgrading(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	csrf := s.sessionCSRF(r)
	phase := firmwareUpgrade.phase()

	version := s.upgradeTargetVersion()

	initial := "preparing"
	switch phase {
	case jobDone:
		initial = "installing"
	case jobFailed:
		initial = "failed"
	}

	surface := func(wdg widget.Widget, useCSRF bool) template.HTML {
		token := ""
		if useCSRF {
			token = csrf
		}
		var buf strings.Builder
		if err := s.widgets.RenderWithToken(&buf, wdg, token, lang, t); err != nil {
			// A surface that will not render is a real shell failure, but the
			// takeover must still stand: let the state show empty rather than
			// 500 a person mid-upgrade off their only screen.
			return ""
		}
		return template.HTML(buf.String())
	}

	view := upgradingView{
		Lang:       langAttr(lang),
		CSS:        s.currentCSS(),
		JSStrings:  jsCatalog(tr),
		Initial:    initial,
		Preparing:  surface(upgradingPreparing(), false),
		Installing: surface(upgradingInstalling(), false),
		Restarting: surface(upgradingRestarting(), false),
		Stalled:    surface(upgradingStalled(), true),
		Done:       surface(upgradingDone(tr, version), false),
	}
	if phase == jobFailed {
		view.Failed = surface(upgradingFailed(firmwareUpgrade.peekFailure()), true)
	}

	var body strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "upgrading.html.tmpl", view); err != nil {
		http.Error(w, "upgrading page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body.String())
}

// upgradeTargetVersion is the build the router is moving to — the available build
// the check recorded, named once on the success surface. After the reboot the
// router is running exactly this, which is why it can be pre-rendered now and
// shown when the client reconnects. Only the recorded check can name it: the
// board reports the build the router is running, which before the reboot is the
// old one, so naming that on "done" would state the wrong version. Without a
// recorded target the surface stays version-less ("You're on the new firmware")
// rather than confidently wrong.
func (s *Server) upgradeTargetVersion() string {
	if truth, known := s.updateTruth(); known && truth.Firmware.To != "" {
		return truth.Firmware.To
	}
	return ""
}

// reassurance is the quiet safety net under the working states — true on this
// hardware, because the A/B slots return to the known-good version on their own
// if a flash goes bad. The act looks irreversible, and it is not.
func reassurance() widget.Widget {
	return &widget.Text{Markdown: "If anything goes wrong, the router returns to its current version on its own."}
}

// upgradingPreparing: owut is building and downloading the image. No true
// percentage exists, so the icon pulses rather than lies.
func upgradingPreparing() widget.Widget {
	return &widget.Empty{Icon: "download",
		Title:    "Preparing your new firmware",
		Body:     "This takes a few minutes. **Keep the router powered.**",
		Children: []widget.Widget{reassurance()},
	}
}

// upgradingInstalling: the image is here and the flash has begun; the router will
// restart itself when it is done.
func upgradingInstalling() widget.Widget {
	return &widget.Empty{Icon: "hard-drive",
		Title:    "Installing your new firmware",
		Body:     "Your router will restart itself when it's done. **Keep it powered.**",
		Children: []widget.Widget{reassurance()},
	}
}

// upgradingRestarting: the router went away, and that is success arriving. This
// screen is the client's own state — it reconnects on its own — so it carries no
// reassurance child; there is nothing left to reassure, only to wait.
func upgradingRestarting() widget.Widget {
	return &widget.Empty{Icon: "refresh-cw",
		Title: "Your router is restarting",
		Body:  "Almost there — it'll be back in a moment. This screen reconnects on its own.",
	}
}

// upgradingDone: back on the new build, the loop closed. The success tone icon
// says the state has arrived; the one door leads back to the router. The title
// composes the running build into a translatable format at the point of use, so
// the version rides as data and the surrounding prose still localizes.
func upgradingDone(tr func(string) string, version string) widget.Widget {
	title := tr("You're on the new firmware")
	if version != "" {
		title = fmt.Sprintf(tr("You're on Mono OpenWrt %s"), version)
	}
	return &widget.Empty{Icon: "circle-check", Variant: "success",
		Title: title,
		Body:  "The upgrade is complete and your settings came across untouched.",
		Children: []widget.Widget{
			&widget.Link{Style: "button", Label: "Back to your router", Href: maintenancePath},
		},
	}
}

// upgradingStalled: the whole upgrade has run past its budget — the build never
// finished, or the flash spawned and the router never came back. The client
// reaches this by its own countdown, not by any server signal, because the server
// has nothing new to say. Rather than hold the person on "keep the power on"
// indefinitely, the surface says so plainly and opens the one honest door: leave
// the takeover and return to Maintenance, which will show whatever the router
// actually is now. The act is a real post — it releases the takeover for any
// phase and clears a finished outcome, so CSRF-gated — the same dismiss the
// failure surface uses.
func upgradingStalled() widget.Widget {
	return &widget.Card{Children: []widget.Widget{&widget.Stack{Loose: true, Children: []widget.Widget{
		&widget.Callout{Variant: "warning",
			Title: "This is taking longer than expected",
			Body:  "The upgrade hasn't finished in the time it usually takes. It may still be working in the background — return to Maintenance to check where it stands.",
		},
		&widget.Form{Action: firmwareInstallPath + "/dismiss", NoSubmit: true, Fields: []widget.Widget{
			&widget.Button{Label: "Back to Maintenance", Style: "secondary", Name: "action", Value: "dismiss"},
		}},
	}}}}
}

// upgradingFailed: owut failed during the background run. The pulse is gone —
// nothing is working — and the surface is honest instead of hopeful. It names the
// outcome plainly, shows the tool's own words for whoever takes them to a forum,
// and offers the two ways out: releasing the failure back to Maintenance, or
// trying again. Both are real posts (state changes, so CSRF-gated); the error is
// rendered as text through the code widget's own escaping, never as markup.
func upgradingFailed(err error) widget.Widget {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	return &widget.Card{Children: []widget.Widget{&widget.Stack{Loose: true, Children: []widget.Widget{
		&widget.Callout{Variant: "danger",
			Title: "The upgrade couldn't start",
			Body:  "Nothing was changed — your router is still on the firmware it started with."},
		&widget.Code{Label: "Error, given by the update server", Value: message, Copy: true},
		&widget.Stack{Inline: true, Children: []widget.Widget{
			&widget.Form{Action: firmwareInstallPath + "/dismiss", NoSubmit: true, Fields: []widget.Widget{
				&widget.Button{Label: "Back to Maintenance", Style: "secondary", Name: "action", Value: "dismiss"},
			}},
			&widget.Form{Action: firmwareInstallPath, NoSubmit: true, Fields: []widget.Widget{
				&widget.Button{Label: "Try again", Name: "action", Value: "install"},
			}},
		}},
	}}}}
}

// handleUpdatesFirmwareStatus is the small truth the takeover's client polls:
// what the job is doing, and — on a failure — the tool's own words. It is gated
// by the same session middleware as every route, so a browser that has lost its
// session gets the login redirect instead, which is exactly the signal the
// client reads as "the router came back on the new build".
func (s *Server) handleUpdatesFirmwareStatus(w http.ResponseWriter, r *http.Request) {
	state := "idle"
	message := ""
	switch firmwareUpgrade.phase() {
	case jobRunning:
		state = "working"
	case jobDone:
		state = "done"
	case jobFailed:
		state = "failed"
		if err := firmwareUpgrade.peekFailure(); err != nil {
			message = strings.TrimSpace(err.Error())
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		State   string `json:"state"`
		Message string `json:"message"`
		Version string `json:"version"`
	}{State: state, Message: message, Version: s.upgradeTargetVersion()})
}

// handleUpdatesFirmwareDismiss releases a failed upgrade — the takeover's "Back
// to Maintenance". It consumes the outcome, so the next render of the maintenance
// area is the ordinary page again, and sends the browser there. A run still under
// way is untouched: acknowledge only releases something finished.
func (s *Server) handleUpdatesFirmwareDismiss(w http.ResponseWriter, r *http.Request) {
	// Release the takeover regardless of phase. A finished run (done or failed) is
	// also consumed, so its outcome does not linger; a run still under way keeps
	// going in the background, but the released flag lets the screen stand down so
	// a stalled build or a router that never returned cannot hold the person.
	firmwareTakeoverReleased.Store(true)
	firmwareUpgrade.acknowledge()
	http.Redirect(w, r, maintenancePath, http.StatusSeeOther)
}
