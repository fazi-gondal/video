package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	"github.com/ZacharyZhang-NY/MujicaUI/input"
	"github.com/ZacharyZhang-NY/MujicaUI/media"
	"github.com/ZacharyZhang-NY/MujicaUI/navigation"
	"github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// SubtitleTrack is one subtitle stream or sidecar file.
type SubtitleTrack struct {
	Name string
	Cues []SubtitleCue
}

type app struct {
	win    *mygo.Window
	engine *PlaybackEngine

	// Playback state
	frame    *ui.Bitmap
	position time.Duration
	duration time.Duration
	playing  bool
	rate     float64
	volume   float64
	muted    bool

	// Subtitle state
	subtitlesOn    bool
	subtitleTracks []SubtitleTrack
	activeTrackIdx int // -1 = none

	// Window / UI state
	fullscreen bool
	pip        bool
	loopMode   media.LoopMode
	showPanel  bool // right-side panel (playlist + subtitles)
	panelTab   int  // 0=playlist, 1=subtitles
	darkMode   bool
	statusMsg  string

	// Current media
	info      *VideoInfo
	playlist  []media.PlaylistItem
	currentID string

	// Drag-drop
	droppedFiles []input.DroppedFile

	// Live menu items
	menuSubtitlesItem  *mygo.MenuItem
	menuFullscreenItem *mygo.MenuItem
	menuPanelItem      *mygo.MenuItem
	menuLoopItem       *mygo.MenuItem
	menuDarkItem       *mygo.MenuItem
}

func newApp() *app {
	a := &app{
		rate:           1.0,
		volume:         0.8,
		subtitlesOn:    false,
		activeTrackIdx: -1,
		showPanel:      false,
		darkMode:       true,
		statusMsg:      "Open a video with File > Open… (Ctrl+O)",
	}
	a.engine = NewPlaybackEngine(
		func(img image.Image, pos time.Duration) {
			bm := ui.NewBitmap(img)
			if a.win != nil {
				a.win.Update(func() { a.frame = bm; a.position = pos })
			} else {
				a.frame = bm
				a.position = pos
			}
		},
		func() {
			if a.win != nil {
				a.win.Update(func() { a.onPlaybackEnded() })
			} else {
				a.onPlaybackEnded()
			}
		},
	)
	return a
}

// ── Subtitle helpers ─────────────────────────────────────────────────────────

func (a *app) activeCues() []SubtitleCue {
	if !a.subtitlesOn || a.activeTrackIdx < 0 || a.activeTrackIdx >= len(a.subtitleTracks) {
		return nil
	}
	return a.subtitleTracks[a.activeTrackIdx].Cues
}

func (a *app) currentSubtitle() string {
	return GetSubtitleAt(a.activeCues(), a.position)
}

// extractEmbeddedSubtitles uses ffmpeg to pull soft subtitle streams out of
// the container into temporary SRT files, then parses them.
func extractEmbeddedSubtitles(videoPath string) []SubtitleTrack {
	probe := exec.Command("ffprobe",
		"-v", "quiet", "-print_format", "json",
		"-show_streams", "-select_streams", "s",
		videoPath,
	)
	probe.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := probe.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	var tracks []SubtitleTrack
	idx := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, `"codec_type"`) || !strings.Contains(line, `subtitle`) {
			continue
		}
		tmp := filepath.Join(os.TempDir(),
			fmt.Sprintf("myvideo_sub_%d_%d.srt", time.Now().UnixNano(), idx))
		cmd := exec.Command("ffmpeg", "-y", "-i", videoPath,
			"-map", fmt.Sprintf("0:s:%d", idx), "-f", "srt", tmp)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := cmd.Run(); err == nil {
			if cues, err := ParseSRTFile(tmp); err == nil && len(cues) > 0 {
				tracks = append(tracks, SubtitleTrack{
					Name: fmt.Sprintf("Track %d", idx+1),
					Cues: cues,
				})
			}
		}
		_ = os.Remove(tmp)
		idx++
	}
	return tracks
}

// buildSubtitleTracks discovers embedded streams then sidecar files.
// Never injects demo cues.
func buildSubtitleTracks(videoPath string) []SubtitleTrack {
	if videoPath == "" {
		return nil
	}
	tracks := extractEmbeddedSubtitles(videoPath)

	base := videoPath[:len(videoPath)-len(filepath.Ext(videoPath))]
	for _, cand := range []string{base + ".srt", base + ".vtt", base + ".en.srt"} {
		if cues, err := ParseSRTFile(cand); err == nil && len(cues) > 0 {
			tracks = append(tracks, SubtitleTrack{Name: filepath.Base(cand), Cues: cues})
		}
	}
	return tracks
}

// ── File loading ─────────────────────────────────────────────────────────────

func (a *app) openFileDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "Open Video File",
			Filters: []mygo.FileFilter{
				{Name: "Video Files", Extensions: []string{
					"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v", "ts", "mts"}},
				{Name: "All Files", Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.loadFile(paths[0], true)
	}()
}

func (a *app) loadFile(path string, autoplay bool) {
	if path == "" {
		return
	}
	if a.win != nil {
		a.win.Update(func() { a.statusMsg = "Loading " + filepath.Base(path) + "…" })
	}
	go func() {
		info, err := ProbeVideo(path)
		if err != nil {
			info = &VideoInfo{Path: path, Title: filepath.Base(path),
				Duration: 10 * time.Second, Width: 640, Height: 360, FPS: 25}
		}
		img, _ := ExtractSingleFrame(path, 0)
		var bm *ui.Bitmap
		if img != nil {
			bm = ui.NewBitmap(img)
		}
		tracks := buildSubtitleTracks(path)

		apply := func() {
			a.info = info
			a.duration = info.Duration
			a.position = 0
			a.frame = bm
			a.subtitleTracks = tracks
			if len(tracks) > 0 {
				a.activeTrackIdx = 0
				a.subtitlesOn = true
			} else {
				a.activeTrackIdx = -1
				a.subtitlesOn = false
			}
			a.currentID = path
			a.statusMsg = info.Title
			if len(tracks) == 0 {
				a.statusMsg += "  •  no subtitles"
			}

			if a.menuSubtitlesItem != nil {
				a.menuSubtitlesItem.SetEnabled(len(tracks) > 0)
				a.menuSubtitlesItem.SetChecked(a.subtitlesOn)
			}

			found := false
			for _, it := range a.playlist {
				if it.ID == path {
					found = true
					break
				}
			}
			if !found {
				a.playlist = append(a.playlist, media.PlaylistItem{
					ID:       path,
					Title:    info.Title,
					Subtitle: fmt.Sprintf("%dx%d · %s", info.Width, info.Height, core.MediaClock(info.Duration)),
					Duration: info.Duration,
				})
			}
			a.engine.Load(path, info.Duration)
			a.engine.SetResolution(info.Width, info.Height)
			if autoplay {
				a.onPlay()
			} else {
				a.playing = false
			}
		}
		if a.win != nil {
			a.win.Update(apply)
		} else {
			apply()
		}
	}()
}

// ── Playback ──────────────────────────────────────────────────────────────────

func (a *app) onPlay() {
	if a.info == nil && len(a.playlist) > 0 {
		a.playTrackByID(a.playlist[0].ID)
		return
	}
	a.playing = true
	a.engine.Play()
	a.statusMsg = "Playing"
}
func (a *app) onPause() {
	a.playing = false
	a.engine.Pause()
	a.statusMsg = "Paused"
}
func (a *app) togglePlay() {
	if a.playing {
		a.onPause()
	} else {
		a.onPlay()
	}
}
func (a *app) onSeek(pos time.Duration) {
	if a.duration > 0 {
		pos = min(max(0, pos), a.duration)
	} else {
		pos = 0
	}
	a.position = pos
	a.engine.Seek(pos)
}
func (a *app) onRate(r float64) {
	if r <= 0 {
		r = 1.0
	}
	a.rate = r
	a.engine.SetRate(r)
}
func (a *app) onSubtitles(on bool) {
	a.subtitlesOn = on
	if a.menuSubtitlesItem != nil {
		a.menuSubtitlesItem.SetChecked(on)
	}
}
func (a *app) onFullscreen(on bool) {
	a.fullscreen = on
	if a.win != nil {
		a.win.SetFullScreen(on)
	}
	if a.menuFullscreenItem != nil {
		a.menuFullscreenItem.SetChecked(on)
	}
}
func (a *app) onPiP(on bool) {
	a.pip = on
	if a.win != nil {
		a.win.SetAlwaysOnTop(on)
	}
}
func (a *app) onPlaybackEnded() {
	if a.loopMode == media.LoopOne {
		a.onSeek(0)
		a.onPlay()
		return
	}
	if len(a.playlist) > 1 {
		for i, it := range a.playlist {
			if it.ID == a.currentID {
				next := (i + 1) % len(a.playlist)
				if a.loopMode == media.LoopAll || next != 0 {
					a.playTrackByID(a.playlist[next].ID)
					return
				}
			}
		}
	}
	a.playing = false
	a.position = a.duration
	a.statusMsg = "Finished"
}
func (a *app) playTrackByID(id string) {
	for _, it := range a.playlist {
		if it.ID == id {
			a.loadFile(it.ID, true)
			return
		}
	}
}
func (a *app) removeTrackByID(id string) {
	a.playlist = slices.DeleteFunc(a.playlist, func(it media.PlaylistItem) bool { return it.ID == id })
	if a.currentID == id {
		if len(a.playlist) > 0 {
			a.playTrackByID(a.playlist[0].ID)
		} else {
			a.engine.Stop()
			a.playing = false
			a.frame = nil
			a.info = nil
			a.position = 0
			a.duration = 0
			a.currentID = ""
		}
	}
}
func (a *app) nextTrack() {
	if len(a.playlist) == 0 {
		return
	}
	for i, it := range a.playlist {
		if it.ID == a.currentID {
			a.playTrackByID(a.playlist[(i+1)%len(a.playlist)].ID)
			return
		}
	}
	a.playTrackByID(a.playlist[0].ID)
}
func (a *app) prevTrack() {
	if len(a.playlist) == 0 {
		return
	}
	for i, it := range a.playlist {
		if it.ID == a.currentID {
			a.playTrackByID(a.playlist[(i-1+len(a.playlist))%len(a.playlist)].ID)
			return
		}
	}
	a.playTrackByID(a.playlist[0].ID)
}

// ── Native Menu Bar ───────────────────────────────────────────────────────────

func (a *app) buildMenu() *mygo.Menu {
	subtitlesItem := &mygo.MenuItem{
		ID: "subs", Label: "Show Subtitles", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+T", Disabled: true,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onSubtitles(it.IsChecked()) })
			}
		},
	}
	a.menuSubtitlesItem = subtitlesItem

	fullscreenItem := &mygo.MenuItem{
		ID: "fs", Label: "Full Screen", Type: mygo.MenuItemCheckbox, Accelerator: "F11",
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onFullscreen(it.IsChecked()) })
			}
		},
	}
	a.menuFullscreenItem = fullscreenItem

	panelItem := &mygo.MenuItem{
		ID: "panel", Label: "Show Side Panel", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+P",
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.showPanel = it.IsChecked() })
			}
		},
	}
	a.menuPanelItem = panelItem

	loopItem := &mygo.MenuItem{
		ID: "loop", Label: "Loop Playback", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+L",
		Click: func(it *mygo.MenuItem, _ *mygo.Window) {
			if it.IsChecked() {
				a.loopMode = media.LoopAll
			} else {
				a.loopMode = media.LoopOff
			}
		},
	}
	a.menuLoopItem = loopItem

	darkItem := &mygo.MenuItem{
		ID: "dark", Label: "Dark Mode", Type: mygo.MenuItemCheckbox, Checked: true,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.darkMode = it.IsChecked() })
			}
		},
	}
	a.menuDarkItem = darkItem

	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open Video…", Accelerator: "CmdOrCtrl+O",
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.openFileDialog() }},
			mygo.Separator(),
			{Label: "Show Playlist", Type: mygo.MenuItemCheckbox, Accelerator: "CmdOrCtrl+1",
				Click: func(it *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() {
							a.showPanel = it.IsChecked()
							a.panelTab = 0
							if a.menuPanelItem != nil {
								a.menuPanelItem.SetChecked(a.showPanel)
							}
						})
					}
				}},
			{Label: "Show Subtitles Panel", Type: mygo.MenuItemCheckbox, Accelerator: "CmdOrCtrl+2",
				Click: func(it *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() {
							a.showPanel = it.IsChecked()
							a.panelTab = 1
							if a.menuPanelItem != nil {
								a.menuPanelItem.SetChecked(a.showPanel)
							}
						})
					}
				}},
			{Label: "Show Drop Zone", Type: mygo.MenuItemCheckbox, Accelerator: "CmdOrCtrl+3",
				Click: func(it *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() {
							a.showPanel = it.IsChecked()
							a.panelTab = 2
							if a.menuPanelItem != nil {
								a.menuPanelItem.SetChecked(a.showPanel)
							}
						})
					}
				}},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "Playback", Submenu: []*mygo.MenuItem{
			{Label: "Play / Pause", Accelerator: "Space",
				Click: func(_ *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() { a.togglePlay() })
					}
				}},
			{Label: "Seek Back 5 s", Accelerator: "Left",
				Click: func(_ *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() { a.onSeek(a.position - 5*time.Second) })
					}
				}},
			{Label: "Seek Forward 5 s", Accelerator: "Right",
				Click: func(_ *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() { a.onSeek(a.position + 5*time.Second) })
					}
				}},
			{Label: "Previous Track", Accelerator: "CmdOrCtrl+Left",
				Click: func(_ *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() { a.prevTrack() })
					}
				}},
			{Label: "Next Track", Accelerator: "CmdOrCtrl+Right",
				Click: func(_ *mygo.MenuItem, win *mygo.Window) {
					if win != nil {
						win.Update(func() { a.nextTrack() })
					}
				}},
			mygo.Separator(),
			loopItem,
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			subtitlesItem,
			mygo.Separator(),
			panelItem,
			mygo.Separator(),
			fullscreenItem,
			{Role: mygo.RoleToggleFullScreen},
			mygo.Separator(),
			darkItem,
		}},
	})
}

// ── Main view ─────────────────────────────────────────────────────────────────

func (a *app) view(c *ui.Context) {
	mode := core.Dark
	if !a.darkMode {
		mode = core.Light
	}
	core.Use(c, core.Settings{Mode: mode})

	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	// Root: full-window column, no padding — VLC style
	ui.Column(c).Fill().Background(ui.Hex("#0B0A0B")).Gap(0).Children(func() {

		// ── Middle: video + optional side panel ─────────────────────────
		ui.Row(c).Grow(1).FillWidth().Gap(0).Children(func() {
			// Video stage
			a.viewStage(c, k, u)
			// Side panel (appears to the right, never overlaps video)
			if a.showPanel {
				a.viewPanel(c, k, u)
			}
		})

		// ── Scrubber ────────────────────────────────────────────────────
		ui.Box(c).FillWidth().Padding(0, u, 0, u).Children(func() {
			if a.duration > 0 {
				pos := min(max(0, a.position), a.duration)
				if media.VideoScrubber(c, &pos, media.VideoScrubberOptions{
					Duration: a.duration,
				}).Changed() {
					a.onSeek(pos)
				}
			}
		})

		// ── Controls bar ────────────────────────────────────────────────
		a.viewControls(c, k, u)
	})
}

// viewStage: focusable video canvas, fills all remaining space.
func (a *app) viewStage(c *ui.Context, k theme.Tokens, u float32) {
	stage := ui.Box(c).Grow(1).FillWidth().
		Background(ui.Hex("#0B0A0B")).Center().Focusable().
		Role(ui.RoleImage).Label("Video Stage")

	// Keyboard shortcuts
	switch {
	case stage.Shortcut(0, ui.KeySpace), stage.Shortcut(0, ui.KeyK):
		a.togglePlay()
	case stage.Shortcut(0, ui.KeyLeft):
		a.onSeek(a.position - 5*time.Second)
	case stage.Shortcut(0, ui.KeyRight):
		a.onSeek(a.position + 5*time.Second)
	case stage.Shortcut(0, ui.KeyF):
		a.onFullscreen(!a.fullscreen)
	case stage.Shortcut(0, ui.KeyC) && len(a.subtitleTracks) > 0:
		a.onSubtitles(!a.subtitlesOn)
	}
	if stage.Clicked() {
		a.togglePlay()
	}

	stage.Children(func() {
		if a.frame == nil {
			// Empty state — VLC-style dark placeholder
			ui.Column(c).Center().Gap(u * 2).Children(func() {
				ui.Icon(c, core.MediaRect).Size(64, 64).TextColor(ui.Hex("#3A3A3A"))
				ui.Text(c, "No media loaded").
					TextColor(ui.Hex("#5A5A5A")).FontSize(core.FontSize(c, theme.H3Size))
				ui.Text(c, "File > Open Video… or Ctrl+O").
					TextColor(ui.Hex("#3A3A3A")).FontSize(core.FontSize(c, theme.BodySize))
			})
		} else {
			// Video frame — Contain = letterbox, preserves aspect ratio
			ui.Image(c, a.frame).Fit(ui.Contain).Fill()
		}

		// Subtitle overlay
		if sub := a.currentSubtitle(); a.subtitlesOn && sub != "" {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(u * 3).
				Center().PassThrough().Children(func() {
				ui.Text(c, sub).
					TextColor(ui.Hex("#FFFFFF")).
					Background(ui.RGBA(0, 0, 0, 0.8)).
					Padding(u*0.5, u*2, u*0.5, u*2).
					Radius(4).TextAlign(ui.Center).MaxLines(4).
					FontSize(core.FontSize(c, theme.BodySize))
			})
		}
	})
}

// viewControls: single compact row — all icon buttons, never wraps.
func (a *app) viewControls(c *ui.Context, k theme.Tokens, u float32) {
	bg := ui.Hex("#161616")
	if !a.darkMode {
		bg = k.Surface
	}

	// Single compact controls row
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u*0.5).
		Background(bg).
		Padding(u*0.5, u, u*0.5, u).Children(func() {

		// ── Left cluster: transport controls ───────────────────────────
		if len(a.playlist) > 1 {
			if core.IconAction(c, core.MediaPrev, "Previous (Ctrl+←)").Clicked() {
				a.prevTrack()
			}
		}
		if core.IconAction(c, core.MediaRotateLeft, "Back 5 s (←)").Clicked() {
			a.onSeek(a.position - 5*time.Second)
		}

		// Play / Pause button — slightly larger, primary colour
		playIcon := core.MediaPlay
		if a.playing {
			playIcon = core.MediaPause
		}
		b := core.IconAction(c, playIcon, "Play/Pause (Space)")
		b.Background(k.Accent).TextColor(k.AccentText).Radius(99).Width(32).Height(32)
		if b.Clicked() {
			a.togglePlay()
		}

		if core.IconAction(c, core.MediaRotate, "Forward 5 s (→)").Clicked() {
			a.onSeek(a.position + 5*time.Second)
		}
		if len(a.playlist) > 1 {
			if core.IconAction(c, core.MediaNext, "Next (Ctrl+→)").Clicked() {
				a.nextTrack()
			}
		}

		// ── Time ───────────────────────────────────────────────────────
		ui.Text(c, core.MediaClock(a.position)+" / "+core.MediaClock(a.duration)).
			FontSize(core.FontSize(c, theme.CaptionSize)).
			TextColor(ui.Hex("#888888")).FontFeatures("tnum").SingleLine()

		// ── Spacer ─────────────────────────────────────────────────────
		ui.Box(c).Grow(1)

		// ── Volume: manual mute icon + slider in the same row ─────────
		// Using raw Slider with ShowValue:false avoids the tooltip bubble
		// that inflates element height and misaligns siblings.
		muteIcon := core.MediaVolume
		if a.muted || a.volume == 0 {
			muteIcon = core.MediaMute
		}
		if core.IconAction(c, muteIcon, "Mute (M)").Clicked() {
			a.muted = !a.muted
			a.engine.SetVolume(a.volume, a.muted)
		}
		volSlider := input.Slider(c, &a.volume, input.SliderOptions{
			Min: 0, Max: 1, Step: 0.05,
			ShowValue: false,
			Label:     "Volume",
		})
		volSlider.Element.Width(90)
		if volSlider.Changed() {
			if a.muted && a.volume > 0 {
				a.muted = false
			}
			a.engine.SetVolume(a.volume, a.muted)
		}

		// Speed
		rate := a.rate
		if rate <= 0 {
			rate = 1.0
		}
		if media.PlaybackSpeedControl(c, &rate, media.PlaybackSpeedControlOptions{}).Changed() {
			a.onRate(rate)
		}

		// Subtitles CC — only when tracks exist
		if len(a.subtitleTracks) > 0 {
			ccBtn := media.MediaToggleAction(c, core.MediaCaptions, "Subtitles (C)", a.subtitlesOn)
			if ccBtn.Clicked() {
				a.onSubtitles(!a.subtitlesOn)
			}
		}

		// Loop (cycles: off → all → one)
		loopIcon := core.MediaRepeat
		if a.loopMode == media.LoopOne {
			loopIcon = core.MediaRepeatOne
		}
		loopBtn := media.MediaToggleAction(c, loopIcon, "Loop", a.loopMode != media.LoopOff)
		if loopBtn.Clicked() {
			switch a.loopMode {
			case media.LoopOff:
				a.loopMode = media.LoopAll
			case media.LoopAll:
				a.loopMode = media.LoopOne
			default:
				a.loopMode = media.LoopOff
			}
			if a.menuLoopItem != nil {
				a.menuLoopItem.SetChecked(a.loopMode != media.LoopOff)
			}
		}

		// PiP
		pipBtn := media.MediaToggleAction(c, core.MediaPiP, "Picture-in-Picture", a.pip)
		if pipBtn.Clicked() {
			a.onPiP(!a.pip)
		}

		// Side panel toggle
		panelBtn := media.MediaToggleAction(c, icons.Must("panel-left"), "Side Panel (Ctrl+P)", a.showPanel)
		if panelBtn.Clicked() {
			a.showPanel = !a.showPanel
			if a.menuPanelItem != nil {
				a.menuPanelItem.SetChecked(a.showPanel)
			}
		}

		// Fullscreen
		fsBtn := media.MediaToggleAction(c, core.MediaFullscreen, "Fullscreen (F / F11)", a.fullscreen)
		if fsBtn.Clicked() {
			a.onFullscreen(!a.fullscreen)
		}
	})
}

// viewPanel: right-side panel (playlist + subtitles), fixed 260 px wide.
// Never overlaps the controls bar because it sits in the middle Row, not below it.
func (a *app) viewPanel(c *ui.Context, k theme.Tokens, u float32) {
	panelBg := ui.Hex("#111111")
	if !a.darkMode {
		panelBg = k.Surface
	}

	ui.Column(c).Width(260).FillHeight().
		Background(panelBg).
		Border(theme.BorderWidth, k.Border).
		Padding(0).Gap(0).Children(func() {

		tabs := []navigation.Tab{
			{Label: " ", Icon: icons.Must("list"),   Panel: func() { a.viewPanelPlaylist(c, k, u) }},
			{Label: " ", Icon: core.MediaCaptions,   Panel: func() { a.viewPanelSubtitles(c, k, u) }},
			{Label: " ", Icon: icons.Must("upload"),  Panel: func() { a.viewPanelDrop(c, k, u) }},
		}
		navigation.Tabs(c, &a.panelTab, tabs, navigation.TabsOptions{Label: "Side Panel"})
	})
}

func (a *app) viewPanelPlaylist(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Grow(1).Padding(u).Gap(u * 0.5).Children(func() {

		// Section header with icon
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u*0.5).
			Padding(0, 0, u*0.5, 0).Children(func() {
			ui.Icon(c, icons.Must("list")).Size(14, 14).TextColor(k.Accent)
			ui.Text(c, fmt.Sprintf("Playlist  ·  %d items", len(a.playlist))).
				Bold().FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.Text)
		})

		if len(a.playlist) == 0 {
			ui.Column(c).Grow(1).Center().Gap(u).Children(func() {
				ui.Icon(c, icons.Must("list")).Size(32, 32).TextColor(k.TextMuted)
				ui.Text(c, "Playlist is empty").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				ui.Text(c, "Open File > Open Video… or drop files here").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			})
			return
		}
		res := media.Playlist(c, &a.playlist, media.PlaylistOptions{
			Current: a.currentID,
		})
		if res.Played != "" {
			a.playTrackByID(res.Played)
		}
		if res.Removed != "" {
			a.removeTrackByID(res.Removed)
		}
	})
}

func (a *app) viewPanelSubtitles(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Grow(1).Padding(u).Gap(u * 0.5).Children(func() {

		// Section header with icon
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u*0.5).
			Padding(0, 0, u*0.5, 0).Children(func() {
			ui.Icon(c, core.MediaCaptions).Size(14, 14).TextColor(k.Accent)
			nTracks := len(a.subtitleTracks)
			label := "No subtitles"
			if nTracks == 1 {
				label = "1 subtitle track"
			}
			if nTracks > 1 {
				label = fmt.Sprintf("%d subtitle tracks", nTracks)
			}
			ui.Text(c, label).Bold().
				FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.Text)
		})

		if len(a.subtitleTracks) == 0 {
			ui.Column(c).Grow(1).Center().Gap(u).Children(func() {
				ui.Icon(c, core.MediaCaptions).Size(32, 32).TextColor(k.TextMuted)
				ui.Text(c, "No subtitles detected").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.BodySize))
				ui.Text(c, "Embedded streams and .srt/.vtt\nfiles are auto-detected.").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize)).
					TextAlign(ui.Center)
			})
			return
		}

		// Track picker
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u * 0.5).Children(func() {
			ui.Text(c, "Track:").TextColor(k.TextMuted).
				FontSize(core.FontSize(c, theme.CaptionSize))
			offV := input.Ghost
			if !a.subtitlesOn {
				offV = input.Secondary
			}
			if input.Button(c, "Off", input.ButtonOptions{Variant: offV}).Clicked() {
				a.onSubtitles(false)
			}
			for i, tr := range a.subtitleTracks {
				idx, name := i, tr.Name
				v := input.Ghost
				if a.subtitlesOn && a.activeTrackIdx == idx {
					v = input.Primary
				}
				if input.Button(c, name, input.ButtonOptions{Variant: v}).Clicked() {
					a.activeTrackIdx = idx
					a.onSubtitles(true)
					a.statusMsg = "Subtitles: " + name
				}
			}
		})

		// Cue list
		cues := a.activeCues()
		if len(cues) == 0 {
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(u * 0.5).Children(func() {
				for _, cue := range cues {
					cueCopy := cue
					isCurrent := a.position >= cue.Start && a.position <= cue.End
					textColor := k.TextMuted
					if isCurrent {
						textColor = k.Accent
					}
					ui.Row(c).Gap(u * 0.5).AlignItems(ui.Center).Children(func() {
						if isCurrent {
							ui.Icon(c, core.MediaCaptions).Size(10, 10).TextColor(k.Accent)
						}
						ts := core.MediaClock(cueCopy.Start)
						if input.Button(c, ts, input.ButtonOptions{Variant: input.Ghost}).Clicked() {
							a.onSeek(cueCopy.Start)
						}
						ui.Text(c, cueCopy.Text).TextColor(textColor).
							FontSize(core.FontSize(c, theme.CaptionSize))
					})
				}
			})
		})
	})
}

func (a *app) viewPanelDrop(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Grow(1).Padding(u).Gap(u * 0.5).Children(func() {

		// Section header with icon
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u*0.5).
			Padding(0, 0, u*0.5, 0).Children(func() {
			ui.Icon(c, icons.Must("upload")).Size(14, 14).TextColor(k.Accent)
			ui.Text(c, "Drop Zone").Bold().
				FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.Text)
		})

		prevLen := len(a.droppedFiles)
		zoneRes := input.FileDropZone(c, &a.droppedFiles, input.FileDropZoneOptions{
			Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v", "ts"},
			Hint:       "Drop videos here",
		})
		if zoneRes.Changed() && len(a.droppedFiles) > prevLen {
			for i := prevLen; i < len(a.droppedFiles); i++ {
				a.loadFile(a.droppedFiles[i].Path, i == prevLen)
			}
		}

		// Info section
		if a.info != nil {
			ui.Box(c).FillWidth().Height(1).Background(k.Border)
			ui.Row(c).AlignItems(ui.Center).Gap(u * 0.5).Children(func() {
				ui.Icon(c, core.MediaRect).Size(14, 14).TextColor(k.Accent)
				ui.Text(c, "Now Playing").Bold().
					FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.Text)
			})
			ui.Column(c).Gap(u * 0.5).Children(func() {
				ui.Text(c, a.info.Title).Bold().
					FontSize(core.FontSize(c, theme.BodySize)).TextColor(k.Text)
				ui.Text(c, fmt.Sprintf("%dx%d  ·  %.0f fps  ·  %s / %s",
					a.info.Width, a.info.Height, a.info.FPS,
					a.info.VideoCodec, a.info.AudioCodec)).
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				if a.info.FileSize > 0 {
					ui.Text(c, fmt.Sprintf("%.1f MB  ·  %s",
						float64(a.info.FileSize)/(1024*1024),
						core.MediaClock(a.info.Duration))).
						TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				}
			})
		}
	})
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Video Player",
			Width:           1280,
			Height:          720,
			MinWidth:        640,
			MinHeight:       400,
			Maximized:       true,
			FullScreen:      false,
			BackgroundColor: "#0B0A0B",
			Content:         ui.View(a.view),
		})
		a.win = win
		win.SetMenu(a.buildMenu())

		win.OnEnterFullScreen(func() {
			win.Update(func() {
				a.fullscreen = true
				if a.menuFullscreenItem != nil {
					a.menuFullscreenItem.SetChecked(true)
				}
			})
		})
		win.OnLeaveFullScreen(func() {
			win.Update(func() {
				a.fullscreen = false
				if a.menuFullscreenItem != nil {
					a.menuFullscreenItem.SetChecked(false)
				}
			})
		})
		win.OnClosed(func() { a.engine.Close() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Close stops the playback engine.
func (e *PlaybackEngine) Close() { e.Stop() }
