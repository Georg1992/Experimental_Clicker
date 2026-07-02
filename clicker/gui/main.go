//go:build windows

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest app.manifest -o rsrc.syso

// Package main is the walk-based Windows GUI for the clicker. It is
// the topmost layer of a three-layer architecture; see README.md in
// this directory for the full layering rules and the import boundary.
//
// Quick rule: this package must only import `experimental-clicker/runner`
// (the public facade). Never import `runner/autopot`, `runner/statusui`,
// `runner/internal/...`, or `runner/platform/...` directly — add the
// missing surface to `runner` first, then consume it here.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"experimental-clicker/runner"

	"github.com/lxn/walk"
)

type guiApp struct {
	mainWindow *walk.MainWindow
	logList    *walk.ListBox
	logItems   []string

	// Clicker tab
	clickerSlots           [runner.ClickerSlotCount]clickerSlotWidgets
	clickerTriggerVKs      [runner.ClickerSlotCount][]int32
	clickerBindingSlot     int
	clickerLastLoggedDelay [runner.ClickerSlotCount]int
	startBtn               *walk.PushButton
	stopBtn                *walk.PushButton
	statusBadge            *statusBadge

	// Timer keys (clicker tab)
	timerSlots        [runner.TimerKeySlotCount]timerSlotWidgets
	timerKeyVKs       [runner.TimerKeySlotCount]int32
	timerVisibleCount int
	timerAddBtn       *walk.PushButton
	timerBindingSlot  int

	// AutoPot tab
	hpEnabledCB     *walk.CheckBox
	spEnabledCB     *walk.CheckBox
	hpThresholdEdit *walk.LineEdit
	spThresholdEdit *walk.LineEdit
	hpKeyLabel      *walk.Label
	spKeyLabel      *walk.Label
	hpBindBtn       *walk.PushButton
	hpClearBtn      *walk.PushButton
	spBindBtn       *walk.PushButton
	spClearBtn      *walk.PushButton

	// KeyChain tab
	keyChainSlots       [runner.KeyChainSlotCount]keyChainSlotWidgets
	keyChainKeyVKs      [runner.KeyChainSlotCount]int32
	keyChainClearBtn    *walk.PushButton
	keyChainBindingSlot int

	mu             sync.Mutex
	shutdownOnce   sync.Once
	logFile        *os.File
	// starting is true while onStart's background goroutine is wiring
	// up the viper server + session + runners. It is set on the GUI
	// thread inside onStart and cleared by either the goroutine on
	// completion (success or fail) or by onStop. While it is true,
	// isStarted reports true so sync*Settings don't trigger secondary
	// runner startups that would race with the main one. The startup
	// goroutine also re-checks this flag after every slot publish so
	// a Stop click during startup cancels any not-yet-published work.
	starting       bool
	startupCancel  context.CancelFunc // cancels in-flight startInBackground on Stop
	runner         *runner.Runner
	autopotRunner  *runner.AutoPotRunner
	timerKeyRunner *runner.TimerKeyRunner
	keyChainRunner *runner.KeyChainRunner
	inputSession   *runner.ViiperSession
	hpKeyVK        int32
	spKeyVK        int32
	hpThreshold    int
	spThreshold    int
	autopotBinding bool
	overlay        *statusOverlay
}

func main() {
	app := &guiApp{timerBindingSlot: -1, keyChainBindingSlot: -1, clickerBindingSlot: -1}
	defer app.shutdown()

	// Open a persistent log file next to the executable so diagnostics
	// survive GUI close. Best-effort — if the file can't be created,
	// logging still works in-memory via the GUI list box.
	if exe, err := os.Executable(); err == nil {
		logPath := filepath.Join(filepath.Dir(exe), "clicker.log")
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			app.logFile = f
			// Stamp the first entry so the user knows where to look.
			_, _ = f.WriteString(fmt.Sprintf("[%s] Log file: %s\n", time.Now().Format("15:04:05"), logPath))
		}
	}

	if err := app.createWindow(); err != nil {
		walk.MsgBox(nil, "BELARUS CHAMP CLICKER", err.Error(), walk.MsgBoxIconError)
	}
}

func (a *guiApp) shutdown() {
	a.shutdownOnce.Do(func() {
		a.mu.Lock()
		r := a.runner
		ap := a.autopotRunner
		tk := a.timerKeyRunner
		kc := a.keyChainRunner
		session := a.inputSession
		a.runner = nil
		a.autopotRunner = nil
		a.timerKeyRunner = nil
		a.keyChainRunner = nil
		a.inputSession = nil
		if a.startupCancel != nil {
			a.startupCancel()
			a.startupCancel = nil
		}
		a.mu.Unlock()

		if a.logFile != nil {
			_ = a.logFile.Close()
			a.logFile = nil
		}

		if r != nil {
			r.Stop()
			r.Wait()
		}
		if ap != nil {
			ap.Stop()
			ap.Wait()
		}
		if tk != nil {
			tk.Stop()
			tk.Wait()
		}
		if kc != nil {
			kc.Stop()
			kc.Wait()
		}
		if session != nil {
			session.Close()
			stopViiperServerIfStarted()
		}

		if a.overlay != nil {
			a.overlay.Destroy()
			a.overlay = nil
		}
	})
}

func (a *guiApp) createWindow() error {
	mw, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	a.mainWindow = mw

	// Create the HP/SP overlay window (hidden until autopot produces values).
	if ovl, ovlErr := newStatusOverlay(); ovlErr == nil {
		a.overlay = ovl
	}

	if err := mw.SetTitle("BELARUS CHAMP CLICKER"); err != nil {
		return err
	}
	if err := mw.SetMinMaxSize(walk.Size{Width: 780, Height: 600}, walk.Size{}); err != nil {
		return err
	}
	if err := mw.SetSize(walk.Size{Width: 780, Height: 600}); err != nil {
		return err
	}

	root := walk.NewVBoxLayout()
	root.SetMargins(walk.Margins{HNear: 10, VNear: 10, HFar: 10, VFar: 10})
	root.SetSpacing(10)
	if err := mw.SetLayout(root); err != nil {
		return err
	}

	icon, err := walk.NewIconFromImage(belarusFlagImage())
	if err != nil {
		return err
	}
	if err := mw.SetIcon(icon); err != nil {
		return err
	}

	if err := addBelarusHeader(mw); err != nil {
		return err
	}

	if err := a.buildControlPanel(mw); err != nil {
		return err
	}

	tabs, err := walk.NewTabWidget(mw)
	if err != nil {
		return err
	}

	clickerPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := clickerPage.SetTitle("Clicker"); err != nil {
		return err
	}
	if err := a.buildClickerTab(clickerPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(clickerPage); err != nil {
		return err
	}

	autopotPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := autopotPage.SetTitle("AutoPot"); err != nil {
		return err
	}
	if err := a.buildAutoPotTab(autopotPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(autopotPage); err != nil {
		return err
	}

	keyChainPage, err := walk.NewTabPage()
	if err != nil {
		return err
	}
	if err := keyChainPage.SetTitle("KeyChain"); err != nil {
		return err
	}
	if err := a.buildKeyChainTab(keyChainPage); err != nil {
		return err
	}
	if err := tabs.Pages().Add(keyChainPage); err != nil {
		return err
	}

	tabs.CurrentIndexChanged().Attach(a.finishThresholdInput)

	mw.Deactivating().Attach(a.finishThresholdInput)

	logLabel, err := walk.NewLabel(mw)
	if err != nil {
		return err
	}
	if err := logLabel.SetText("Logs"); err != nil {
		return err
	}

	a.logList, err = walk.NewListBox(mw)
	if err != nil {
		return err
	}
	if err := a.logList.SetMinMaxSize(walk.Size{Width: 0, Height: 140}, walk.Size{}); err != nil {
		return err
	}
	a.logItems = []string{}
	if err := a.logList.SetModel(a.logItems); err != nil {
		return err
	}
	a.wireThresholdBlurOnClick(mw)
	a.setStarted(false)

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.shutdown()
	})

	mw.Show()
	mw.Run()
	return nil
}

func (a *guiApp) appendLog(line string) {
	stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line)

	// Write to persistent log file (best-effort — file may be missing).
	if a.logFile != nil {
		_, _ = a.logFile.WriteString(stamped + "\n")
	}

	if a.logList == nil {
		return
	}
	a.logItems = append(a.logItems, stamped)
	// UI update errors are not critical; log display may fail but log entry is recorded
	_ = a.logList.SetModel(a.logItems)
	if len(a.logItems) > 0 {
		_ = a.logList.SetCurrentIndex(len(a.logItems) - 1)
	}
}

func (a *guiApp) isStarted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.starting {
		return true
	}
	if a.runner != nil && a.runner.Running() {
		return true
	}
	if a.autopotRunner != nil && a.autopotRunner.Running() {
		return true
	}
	return false
}

func (a *guiApp) autopotWanted() runner.AutoPotConfig {
	cfg := a.autopotConfig()
	cfg.HPEnabled = cfg.HPEnabled && cfg.HPKeyVK != 0
	cfg.SPEnabled = cfg.SPEnabled && cfg.SPKeyVK != 0
	return cfg
}

func (a *guiApp) startAutoPotRunner(cfg runner.AutoPotConfig, log func(string)) {
	take, store := makeLifecycleSlot[*runner.AutoPotRunner](&a.mu, &a.autopotRunner)
	startLifecycle(
		take, store,
		"AutoPot",
		log,
		func() runner.InputSession {
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.inputSession
		},
		func() bool { return cfg.HPEnabled || cfg.SPEnabled },
		func(sess runner.InputSession) *runner.AutoPotRunner {
			cfg.Session = sess
			cfg.Log = log
			return runner.NewAutoPot(cfg)
		},
	)
}

func (a *guiApp) setStarted(started bool) {
	a.startBtn.SetEnabled(!started)
	a.stopBtn.SetEnabled(started)
	a.setClickerConfigEnabled(started)
	a.setAutoPotConfigEnabled(started)
	a.setTimerKeyConfigEnabled(started)
	a.setKeyChainConfigEnabled(started)
	if started {
		a.setClickerStatus(clickerStatusRunning)
	} else {
		a.setClickerStatus(clickerStatusStopped)
	}
}

// setClickerStatus updates the status badge. MUST be called on the
// GUI thread. Off-thread callers must marshal through
// mainWindow.Synchronize themselves; see pauseFn in startInBackground.
//
// Earlier versions wrapped Synchronize here. That marshal was the
// dead-end of the original "stuck at Starting..." path: Synchronize
// posts a window message and blocks the caller until the GUI message
// pump processes it. Calling it from the GUI thread (e.g. from
// setStarted inside an onStart click handler) deadlocks the pump on
// itself, and calling it from inside another Synchronize's posted
// function deadlocks for the same reason. Marshaling is owned at the
// call site now so the threading model is explicit, not implicit.
func (a *guiApp) setClickerStatus(status clickerStatus) {
	if a.statusBadge != nil {
		a.statusBadge.SetStatus(status)
	}
}

func (a *guiApp) syncRunnerSettings() {
	cfg := a.clickerConfig()
	a.mu.Lock()
	r := a.runner
	a.mu.Unlock()

	if r != nil && r.Running() {
		r.UpdateSettings(cfg.Slots)
	}
}

// onStart is the Start button click handler. The blocking portion of
// startup (ensureViiperServer, OpenViiperSession, runner wiring) used
// to run on the GUI thread, holding the walk message pump hostage for
// up to ~30 s while viper.exe can't be pinged. While the pump was
// busy in onStart, any off-thread log call via mainWindow.Synchronize
// (from the pause watcher, the clicker runner, or the autopot runner)
// would deadlock the caller, and the GUI was frozen at "Starting..."
// with no further progress.
//
// The fix keeps only the fast state checks and immediate UI feedback
// on the GUI thread, then spawns a goroutine for everything that
// touches the network or viper. The message pump keeps running, so
// off-thread callbacks can marshal logs and status updates back to
// the GUI thread and the user can still click Stop / close the window
// while startup is in flight.
func (a *guiApp) onStart() {
	a.mu.Lock()
	if (a.runner != nil && a.runner.Running()) || (a.autopotRunner != nil && a.autopotRunner.Running()) {
		a.mu.Unlock()
		return
	}
	if ready, msg := inputDriverReady(); !ready {
		a.mu.Unlock()
		a.appendLog("Input driver not ready — see Setup required dialog.")
		walk.MsgBox(a.mainWindow, "Setup required", msg, walk.MsgBoxIconWarning)
		return
	}
	// Cancel any previous startup goroutine that is still running
	// (e.g. stuck in waitForServer). This ensures the stale goroutine
	// releases serverMu and doesn't write back stale state.
	if a.startupCancel != nil {
		a.startupCancel()
		a.startupCancel = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.startupCancel = cancel
	a.starting = true
	a.mu.Unlock()

	// Immediate UI feedback so the user sees we accepted the click.
	// Safe on the GUI thread: setStarted → setClickerStatus reach walk
	// directly now (no internal Synchronize).
	a.setStarted(true)
	a.appendLog("Starting...")

	go a.startInBackground(ctx)
}

// startInBackground runs the long-running startup work off the GUI
// thread. Every UI mutation goes through a mainWindow.Synchronize
// closure so it lands on the GUI thread; every cfg.Log / OnStatusParsed
// / SetOnPauseChanged wiring uses the same closure pattern so log
// calls and badge updates from off-thread goroutines don't punch walk
// API directly.
func (a *guiApp) startInBackground(ctx context.Context) {
	// logFn: ship a string from off-thread to a.appendLog on the GUI
	// thread (via mainWindow.Synchronize).
	logFn := func(s string) {
		a.mainWindow.Synchronize(func() { a.appendLog(s) })
	}
	// statusFn: ship HP/SP parse results from the autopot goroutine to
	// a.onStatusParsed on the GUI thread.
	statusFn := func(hp, hpMax, sp, spMax, x, y, w, h int) {
		a.mainWindow.Synchronize(func() {
			a.onStatusParsed(hp, hpMax, sp, spMax, x, y, w, h)
		})
	}
	// pauseFn: ship pause/resume from the pause-watcher goroutine to
	// the badge update on the GUI thread. setClickerStatus is
	// GUI-thread-only; synchronization is owned here.
	pauseFn := func(paused bool) {
		a.mainWindow.Synchronize(func() {
			if paused {
				a.setClickerStatus(clickerStatusPaused)
			} else {
				a.setClickerStatus(clickerStatusRunning)
			}
		})
	}
	// isStillStarting reports whether the startup goroutine should
	// keep publishing slots. It checks the context (canceled by
	// onStop) and the starting flag. The goroutine checks this after
	// every slot write so an onStop click during startup cancels any
	// not-yet-published work. Slots already published are not
	// retracted here — onStop has already snapshotted them and its
	// cleanup goroutine is responsible for tearing them down.
	isStillStarting := func() bool {
		if ctx.Err() != nil {
			return false
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.starting
	}
	// finishFailure: restores the UI to "stopped" and clears the
	// starting flag so isStarted() falls back to false on the next
	// call. Only touches state when this goroutine's context is still
	// alive (a newer Start has not canceled us). viper cleanup is the
	// caller's responsibility because the viper server may or may not
	// have been started.
	finishFailure := func() {
		if ctx.Err() != nil {
			return // superseded by a newer startup
		}
		a.mu.Lock()
		if a.starting {
			a.starting = false
		}
		a.mu.Unlock()
		a.mainWindow.Synchronize(func() { a.setStarted(false) })
	}

	logFn("Checking VIIPER server...")
	_, err := ensureViiperServer(ctx, logFn)
	if err != nil {
		if ctx.Err() != nil {
			return // Stop clicked; onStop already handled UI
		}
		logFn(fmt.Sprintf("Start failed: %v", err))
		finishFailure()
		return
	}

	logFn("Opening VIIPER session...")
	session, err := runner.OpenViiperSession(ctx, runner.DefaultAPIAddr, logFn)
	if err != nil {
		logFn(fmt.Sprintf("Start failed: %v", err))
		stopViiperServerIfStarted()
		finishFailure()
		return
	}

	a.mu.Lock()
	a.inputSession = session
	a.mu.Unlock()
	// Cancel-detected after publishing the session but before wiring
	// the clicker runner: clean up our side. onStop may have already
	// snapshotted (nil) session — it will skip server teardown.
	if !isStillStarting() {
		session.Close()
		a.mu.Lock()
		a.inputSession = nil
		a.mu.Unlock()
		stopViiperServerIfStarted()
		return
	}

	session.SetOnPauseChanged(pauseFn)
	session.StartPauseWatcher(ctx, logFn)

	cfg := a.clickerConfig()
	cfg.Session = session
	// Override the Log wiring from a.appendLog (which the cfg getter
	// pre-fills) to logFn, so the clicker goroutine's log dispatches
	// marshal through Synchronize instead of touching walk API off
	// the GUI thread.
	cfg.Log = logFn

	r := runner.New(cfg)
	if err := r.Start(); err != nil {
		session.Close()
		a.mu.Lock()
		a.inputSession = nil
		a.mu.Unlock()
		logFn(fmt.Sprintf("Start failed: %v", err))
		stopViiperServerIfStarted()
		finishFailure()
		return
	}

	a.mu.Lock()
	a.runner = r
	a.mu.Unlock()
	// Cancel-detected after the clicker runner publish. Stop the
	// runner, close the session, tear down the server. onStop
	// already nil-ed its snapshot — this cleanup is ours.
	if !isStillStarting() {
		r.Stop()
		r.Wait()
		session.Close()
		a.mu.Lock()
		a.runner = nil
		a.inputSession = nil
		a.mu.Unlock()
		stopViiperServerIfStarted()
		return
	}

	autopotCfg := a.autopotWanted()
	autopotCfg.Session = session
	autopotCfg.Log = logFn
	autopotCfg.OnStatusParsed = statusFn

	timerCfg := a.timerKeyWanted()
	timerCfg.Session = session
	timerCfg.Log = logFn

	keyChainCfg := a.keyChainConfig()
	keyChainCfg.Session = session
	keyChainCfg.Log = logFn

	a.startAutoPotRunner(autopotCfg, logFn)
	a.startTimerKeyRunner(timerCfg, logFn)
	a.startKeyChainRunner(keyChainCfg, logFn)

	a.mu.Lock()
	// If onStop cleared starting before we reached this point,
	// it has already snapshotted the runners/session for its
	// cleanup goroutine — nothing more for us to do.
	canceled := !a.starting
	if a.starting {
		a.starting = false
	}
	a.mu.Unlock()

	if canceled {
		return
	}
	a.mainWindow.Synchronize(func() { a.setStarted(true) })
	logFn("Started")
}

func (a *guiApp) onStop() {
	// onStop is idempotent against an empty state: every field
	// snapshot below is nil-safe (each runner has its own nil
	// guard inside Stop/Wait, session.Close uses sync.Once, the
	// viper-server teardown is a no-op if never started). So no
	// early return is needed; the cleanup goroutine does nothing on
	// a Stop click that fires before any Start.
	a.mu.Lock()
	r := a.runner
	ap := a.autopotRunner
	tk := a.timerKeyRunner
	kc := a.keyChainRunner
	session := a.inputSession
	a.runner = nil
	a.autopotRunner = nil
	a.timerKeyRunner = nil
	a.keyChainRunner = nil
	a.inputSession = nil
	// Cancel any in-flight startup goroutine's "starting" gate so
	// isStarted() falls back to false immediately, even if the
	// startup goroutine hasn't reached its own clear-yet.
	a.starting = false
	// Cancel the startup context so ensureViiperServer / waitForServer
	// bail out of their polling loops and release serverMu promptly.
	if a.startupCancel != nil {
		a.startupCancel()
		a.startupCancel = nil
	}
	a.mu.Unlock()

	a.setStarted(false)
	a.appendLog("Stopping...")

	go func() {
		if r != nil {
			r.Stop()
			r.Wait()
		}
		if ap != nil {
			ap.Stop()
			ap.Wait()
		}
		if tk != nil {
			tk.Stop()
			tk.Wait()
		}
		if kc != nil {
			kc.Stop()
			kc.Wait()
		}
		if session != nil {
			session.Close()
			stopViiperServerIfStarted()
		}
		a.mainWindow.Synchronize(func() {
			a.appendLog("Clicker stopped — click Start before launching the game")
		})
	}()
}
