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
	"github.com/ZacharyZhang-NY/MujicaUI/display"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	"github.com/ZacharyZhang-NY/MujicaUI/input"
	"github.com/ZacharyZhang-NY/MujicaUI/media"
	"github.com/ZacharyZhang-NY/MujicaUI/navigation"
	"github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// SubtitleTrack represents one subtitle stream or file.
type SubtitleTrack struct {
	Name string
	Cues []SubtitleCue
}

type app struct {
	win          *mygo.Window
	engine       *PlaybackEngine
	frame        *ui.Bitmap
	position     time.Duration
	duration     time.Duration
	playing      bool
	rate         float64
	volume       float64
	muted        bool
	subtitlesOn  bool
	fullscreen   bool
	pip          bool
	loopMode     media.LoopMode
	shuffle      bool
	showDrawer   bool
	info         *VideoInfo
	playlist     []media.PlaylistItem
	currentID    string
	// Subtitle tracks (embedded + side-car files)
	subtitleTracks []SubtitleTrack
	activeTrackIdx int // index into subtitleTracks; -1 = none selected
	// File picking / drag-drop
	pickedPaths  []string
	droppedFiles []input.DroppedFile
	selectedTab  int
	darkMode     bool
	statusMsg    string
	// Native menu items we need to update at runtime
	menuSubtitlesItem *mygo.MenuItem
	menuLoopItem      *mygo.MenuItem
	menuFullscreenItem *mygo.MenuItem
}

func newApp() *app {
	a := &app{
		rate:           1.0,
		volume:         0.8,
		subtitlesOn:    false, // off until a real subtitle track is found
		activeTrackIdx: -1,
		showDrawer:     false,
		darkMode:       true,
		statusMsg:      "Ready – use File > Open to load a video",
	}

	a.engine = NewPlaybackEngine(
		func(img image.Image, pos time.Duration) {
			bm := ui.NewBitmap(img)
			if a.win != nil {
				a.win.Update(func() {
					a.frame = bm
					a.position = pos
				})
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

// extractEmbeddedSubtitles uses ffmpeg to extract all soft subtitle streams
// from the video file into in-memory SRT cues.
func extractEmbeddedSubtitles(videoPath string) []SubtitleTrack {
	// Ask ffprobe for the subtitle stream metadata
	probe := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "s",
		videoPath,
	)
	probe.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := probe.Output()

	var tracks []SubtitleTrack
	if err == nil && len(out) > 0 {
		// Count subtitle streams by counting "codec_type":"subtitle"
		idx := 0
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, `"codec_type": "subtitle"`) || strings.Contains(line, `"codec_type":"subtitle"`) {
				// Extract this stream as SRT via a temp file
				tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("myvideo_sub_%d_%d.srt", time.Now().UnixNano(), idx))
				cmd := exec.Command("ffmpeg",
					"-y",
					"-i", videoPath,
					"-map", fmt.Sprintf("0:s:%d", idx),
					"-f", "srt",
					tmpFile,
				)
				cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
				if err := cmd.Run(); err == nil {
					cues, parseErr := ParseSRTFile(tmpFile)
					_ = os.Remove(tmpFile)
					if parseErr == nil && len(cues) > 0 {
						name := fmt.Sprintf("Track %d", idx+1)
						tracks = append(tracks, SubtitleTrack{Name: name, Cues: cues})
					}
				} else {
					_ = os.Remove(tmpFile)
				}
				idx++
			}
		}
	}
	return tracks
}

// buildSubtitleTracks discovers subtitle tracks: first embedded streams,
// then adjacent .srt/.vtt side-car files. Returns empty slice (not nil)
// so the UI can show "No subtitles found" when empty.
func buildSubtitleTracks(videoPath string) []SubtitleTrack {
	var tracks []SubtitleTrack

	// 1. Embedded (soft) subtitle streams
	if videoPath != "" {
		tracks = append(tracks, extractEmbeddedSubtitles(videoPath)...)
	}

	// 2. Side-car files adjacent to the video
	if videoPath != "" {
		base := videoPath[:len(videoPath)-len(filepath.Ext(videoPath))]
		candidates := []string{
			base + ".srt",
			base + ".vtt",
			base + ".en.srt",
			base + ".eng.srt",
		}
		seen := map[string]bool{}
		for _, path := range candidates {
			if seen[path] {
				continue
			}
			seen[path] = true
			cues, err := ParseSRTFile(path)
			if err == nil && len(cues) > 0 {
				tracks = append(tracks, SubtitleTrack{
					Name: filepath.Base(path),
					Cues: cues,
				})
			}
		}
	}

	return tracks
}

// ── File loading ─────────────────────────────────────────────────────────────

// openFileDialog opens the system file picker and loads the chosen video.
func (a *app) openFileDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "Open Video File",
			Filters: []mygo.FileFilter{
				{Name: "Video Files", Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v", "ts", "mts"}},
				{Name: "All Files", Extensions: []string{"*"}},
			},
			Multiple: false,
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
		a.win.Update(func() { a.statusMsg = "Loading: " + filepath.Base(path) })
	}

	go func() {
		info, err := ProbeVideo(path)
		if err != nil {
			info = &VideoInfo{
				Path:     path,
				Title:    filepath.Base(path),
				Duration: 10 * time.Second,
				Width:    640,
				Height:   360,
				FPS:      25,
			}
		}

		img, _ := ExtractSingleFrame(path, 0)
		var bm *ui.Bitmap
		if img != nil {
			bm = ui.NewBitmap(img)
		}

		// Subtitle discovery (embedded + sidecar, NO demo cues)
		tracks := buildSubtitleTracks(path)

		apply := func() {
			a.info = info
			a.duration = info.Duration
			a.position = 0
			a.frame = bm
			a.subtitleTracks = tracks
			// Auto-select first track if available, else disable subtitles
			if len(tracks) > 0 {
				a.activeTrackIdx = 0
				a.subtitlesOn = true
			} else {
				a.activeTrackIdx = -1
				a.subtitlesOn = false
			}
			a.currentID = path
			a.statusMsg = "Loaded: " + info.Title
			if len(tracks) == 0 {
				a.statusMsg += " (no subtitles)"
			}

			// Update menu items
			if a.menuSubtitlesItem != nil {
				a.menuSubtitlesItem.SetEnabled(len(tracks) > 0)
				a.menuSubtitlesItem.SetChecked(a.subtitlesOn)
			}

			// Add to playlist if not already present
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
					Subtitle: fmt.Sprintf("%dx%d • %s", info.Width, info.Height, core.MediaClock(info.Duration)),
					Duration: info.Duration,
				})
			}

			a.engine.Load(path, info.Duration)
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

// ── Playback actions ─────────────────────────────────────────────────────────

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
	a.statusMsg = fmt.Sprintf("Speed: %.2f×", r)
}

func (a *app) onSubtitles(on bool) {
	a.subtitlesOn = on
	if a.menuSubtitlesItem != nil {
		a.menuSubtitlesItem.SetChecked(on)
	}
	if on {
		a.statusMsg = "Subtitles On"
	} else {
		a.statusMsg = "Subtitles Off"
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
	if on {
		a.statusMsg = "Picture-in-Picture"
	} else {
		a.statusMsg = "PiP Off"
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
	a.statusMsg = "Playback ended"
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
	a.playlist = slices.DeleteFunc(a.playlist, func(it media.PlaylistItem) bool {
		return it.ID == id
	})
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
	// Subtitle toggle (disabled until a video with subtitles is loaded)
	subtitlesItem := &mygo.MenuItem{
		ID:          "subtitles",
		Label:       "Show Subtitles",
		Type:        mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+T",
		Disabled:    true, // enabled after load
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onSubtitles(it.IsChecked()) })
			}
		},
	}
	a.menuSubtitlesItem = subtitlesItem

	fullscreenItem := &mygo.MenuItem{
		ID:          "fullscreen",
		Label:       "Full Screen",
		Type:        mygo.MenuItemCheckbox,
		Accelerator: "F11",
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onFullscreen(it.IsChecked()) })
			}
		},
	}
	a.menuFullscreenItem = fullscreenItem

	loopItem := &mygo.MenuItem{
		ID:          "loop",
		Label:       "Loop Playback",
		Type:        mygo.MenuItemCheckbox,
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

	return mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "File",
			Submenu: []*mygo.MenuItem{
				{
					ID:          "open",
					Label:       "Open Video…",
					Accelerator: "CmdOrCtrl+O",
					Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
						a.openFileDialog()
					},
				},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			},
		},
		{
			Label: "Playback",
			Submenu: []*mygo.MenuItem{
				{
					ID:          "playpause",
					Label:       "Play / Pause",
					Accelerator: "Space",
					Click: func(_ *mygo.MenuItem, win *mygo.Window) {
						if win != nil {
							win.Update(func() { a.togglePlay() })
						}
					},
				},
				{
					Label:       "Seek Back 5s",
					Accelerator: "Left",
					Click: func(_ *mygo.MenuItem, win *mygo.Window) {
						if win != nil {
							win.Update(func() { a.onSeek(a.position - 5*time.Second) })
						}
					},
				},
				{
					Label:       "Seek Forward 5s",
					Accelerator: "Right",
					Click: func(_ *mygo.MenuItem, win *mygo.Window) {
						if win != nil {
							win.Update(func() { a.onSeek(a.position + 5*time.Second) })
						}
					},
				},
				mygo.Separator(),
				loopItem,
			},
		},
		{
			Label: "View",
			Submenu: []*mygo.MenuItem{
				subtitlesItem,
				mygo.Separator(),
				fullscreenItem,
				{Role: mygo.RoleToggleFullScreen},
			},
		},
	})
}

// ── Main view ────────────────────────────────────────────────────────────────

func (a *app) view(c *ui.Context) {
	mode := core.Dark
	if !a.darkMode {
		mode = core.Light
	}
	core.Use(c, core.Settings{Mode: mode})

	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	// Root column — fills the entire native window
	ui.Column(c).Fill().Background(k.Background).
		Padding(u, u*2, u, u*2).Gap(u).Children(func() {

		// ── 1. Compact Header ─────────────────────────────────────────
		a.viewHeader(c, k, u)

		// ── 2. Video Stage (grows to fill remaining space) ─────────────
		a.viewStage(c, k, u)

		// ── 3. Progress Scrubber ───────────────────────────────────────
		a.viewScrubber(c, u)

		// ── 4. Compact Controls Bar ────────────────────────────────────
		a.viewControls(c, k, u)

		// ── 5. Collapsible Drawer ──────────────────────────────────────
		if a.showDrawer {
			a.viewDrawer(c, k, u)
		}
	})
}

func (a *app) viewHeader(c *ui.Context, k theme.Tokens, u float32) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u).
		Background(k.Surface).Radius(theme.ControlRadius).
		Border(theme.BorderWidth, k.Border).
		Padding(u*0.5, u*2, u*0.5, u*2).Children(func() {

		ui.Icon(c, core.MediaPlay).Size(16, 16).TextColor(k.Accent)
		ui.Text(c, "MyGo Player").
			FontSize(core.FontSize(c, theme.BodySize)).Bold().TextColor(k.Text)

		// Current file badge
		if a.info != nil {
			display.Badge(c, display.BadgeOptions{
				Text:     a.info.Title,
				Tone:     display.BadgeNeutral,
				Position: display.BadgeInline,
			}, nil)
		}

		ui.Box(c).Grow(1)

		// Open file (uses system dialog — see also File menu)
		if core.IconAction(c, icons.Must("folder"), "Open File (Ctrl+O)").Clicked() {
			a.openFileDialog()
		}

		// Drawer toggle
		drawerIcon := icons.Must("panel-left")
		if input.Button(c, fmt.Sprintf("(%d)", len(a.playlist)), input.ButtonOptions{
			Variant: func() input.ButtonVariant {
				if a.showDrawer {
					return input.Secondary
				}
				return input.Ghost
			}(),
			Icon: drawerIcon,
		}).Clicked() {
			a.showDrawer = !a.showDrawer
		}

		// Theme toggle
		themeIcon := icons.Must("sun")
		if !a.darkMode {
			themeIcon = icons.Must("moon")
		}
		if core.IconAction(c, themeIcon, "Toggle Theme").Clicked() {
			a.darkMode = !a.darkMode
		}
	})
}

func (a *app) viewStage(c *ui.Context, k theme.Tokens, u float32) {
	stage := ui.Box(c).Grow(1).FillWidth().
		Background(ui.Hex("#0B0A0B")).
		Radius(theme.CardRadius).
		Border(theme.BorderWidth, k.Border).
		Center().Focusable().
		Role(ui.RoleImage).Label("Video Player Stage")

	// Keyboard shortcuts on focused stage
	if stage.Shortcut(0, ui.KeySpace) || stage.Shortcut(0, ui.KeyK) {
		a.togglePlay()
	}
	if stage.Shortcut(0, ui.KeyLeft) {
		a.onSeek(a.position - 5*time.Second)
	}
	if stage.Shortcut(0, ui.KeyRight) {
		a.onSeek(a.position + 5*time.Second)
	}
	if stage.Shortcut(0, ui.KeyF) {
		a.onFullscreen(!a.fullscreen)
	}
	if stage.Shortcut(0, ui.KeyC) {
		if len(a.subtitleTracks) > 0 {
			a.onSubtitles(!a.subtitlesOn)
		}
	}
	if stage.Clicked() {
		a.togglePlay()
	}

	stage.Children(func() {
		if a.frame == nil {
			ui.Column(c).Center().Gap(u * 2).Children(func() {
				ui.Icon(c, core.MediaRect).Size(52, 52).TextColor(k.TextMuted)
				ui.Text(c, "No video loaded").
					TextColor(k.Text).FontSize(core.FontSize(c, theme.H3Size)).Bold()
				ui.Text(c, "File > Open Video… (Ctrl+O) or drag & drop a file").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.BodySize))
			})
		} else {
			ui.Image(c, a.frame).Fit(ui.Contain).Fill()
		}

		// Subtitle overlay
		sub := a.currentSubtitle()
		if a.subtitlesOn && sub != "" {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(u * 2).
				Center().PassThrough().Children(func() {
				ui.Text(c, sub).
					TextColor(ui.Hex("#F2EBDD")).
					Background(ui.RGBA(0, 0, 0, 0.78)).
					Padding(u*0.5, u*2, u*0.5, u*2).
					Radius(theme.ControlRadius).
					TextAlign(ui.Center).MaxLines(3).
					FontSize(core.FontSize(c, theme.BodySize))
			})
		}
	})
}

func (a *app) viewScrubber(c *ui.Context, u float32) {
	if a.duration > 0 {
		pos := min(max(0, a.position), a.duration)
		if media.VideoScrubber(c, &pos, media.VideoScrubberOptions{
			Duration: a.duration,
		}).Changed() {
			a.onSeek(pos)
		}
	}
}

func (a *app) viewControls(c *ui.Context, k theme.Tokens, u float32) {
	// Single compact row — smaller padding than before
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u).
		Background(k.Surface).Radius(theme.ControlRadius).
		Border(theme.BorderWidth, k.Border).
		Padding(u*0.5, u*2, u*0.5, u*2).Children(func() {

		// Prev / Next (only when playlist has more than one)
		if len(a.playlist) > 1 {
			if core.IconAction(c, core.MediaPrev, "Previous Track").Clicked() {
				a.prevTrack()
			}
		}

		// Seek back 5 s
		if core.IconAction(c, core.MediaRotateLeft, "–5 s").Clicked() {
			a.onSeek(a.position - 5*time.Second)
		}

		// Play / Pause  (prominent)
		playIcon := core.MediaPlay
		playLabel := "Play"
		if a.playing {
			playIcon = core.MediaPause
			playLabel = "Pause"
		}
		if input.Button(c, "", input.ButtonOptions{
			Variant: input.Primary,
			Icon:    playIcon,
			Label:   playLabel,
		}).Clicked() {
			a.togglePlay()
		}

		// Seek forward 5 s
		if core.IconAction(c, core.MediaRotate, "+5 s").Clicked() {
			a.onSeek(a.position + 5*time.Second)
		}

		if len(a.playlist) > 1 {
			if core.IconAction(c, core.MediaNext, "Next Track").Clicked() {
				a.nextTrack()
			}
		}

		// Time clock
		ui.Text(c, core.MediaClock(a.position)+" / "+core.MediaClock(a.duration)).
			FontSize(core.FontSize(c, theme.CaptionSize)).
			TextColor(k.TextMuted).FontFeatures("tnum").SingleLine()

		ui.Box(c).Grow(1)

		// Volume
		volRes := media.VolumeControl(c, &a.volume, &a.muted, media.VolumeControlOptions{})
		if volRes.Changed() {
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

		// Subtitles CC (only when subtitles are available)
		if len(a.subtitleTracks) > 0 {
			ccBtn := media.MediaToggleAction(c, core.MediaCaptions, "Subtitles (C)", a.subtitlesOn)
			if ccBtn.Clicked() {
				a.onSubtitles(!a.subtitlesOn)
			}
		}

		// Loop
		loopOn := a.loopMode != media.LoopOff
		loopIcon := core.MediaRepeat
		if a.loopMode == media.LoopOne {
			loopIcon = core.MediaRepeatOne
		}
		loopBtn := media.MediaToggleAction(c, loopIcon, "Loop", loopOn)
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

		// Fullscreen
		fsBtn := media.MediaToggleAction(c, core.MediaFullscreen, "Fullscreen (F)", a.fullscreen)
		if fsBtn.Clicked() {
			a.onFullscreen(!a.fullscreen)
		}
	})
}

func (a *app) viewDrawer(c *ui.Context, k theme.Tokens, u float32) {
	// Fixed height drawer — small enough not to push content off screen
	ui.Column(c).FillWidth().Height(180).
		Background(k.Surface).Radius(theme.CardRadius).
		Border(theme.BorderWidth, k.Border).
		Padding(u*0.5, u, u*0.5, u).Children(func() {

		tabs := []navigation.Tab{
			{
				Label: fmt.Sprintf("Playlist (%d)", len(a.playlist)),
				Icon:  icons.Must("list"),
				Panel: func() { a.viewPlaylistTab(c, u) },
			},
			{
				Label: fmt.Sprintf("Subtitles (%d)", len(a.subtitleTracks)),
				Icon:  core.MediaCaptions,
				Panel: func() { a.viewSubtitlesTab(c, k, u) },
			},
			{
				Label: "Drop Zone",
				Icon:  icons.Must("upload"),
				Panel: func() { a.viewDropZoneTab(c, k, u) },
			},
			{
				Label: "Video Details",
				Icon:  icons.Must("info"),
				Panel: func() { a.viewInfoTab(c, k, u) },
			},
		}
		navigation.Tabs(c, &a.selectedTab, tabs, navigation.TabsOptions{Label: "Drawer Tabs"})
	})
}

// ── Drawer Tabs ──────────────────────────────────────────────────────────────

func (a *app) viewPlaylistTab(c *ui.Context, u float32) {
	k := core.Tokens(c)
	ui.Column(c).Gap(u * 0.5).Padding(u * 0.5).Children(func() {
		if len(a.playlist) == 0 {
			ui.Text(c, "No videos yet – use File > Open or drop files here.").
				TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			return
		}
		res := media.Playlist(c, &a.playlist, media.PlaylistOptions{
			Current: a.currentID,
			Height:  115,
		})
		if res.Played != "" {
			a.playTrackByID(res.Played)
		}
		if res.Removed != "" {
			a.removeTrackByID(res.Removed)
		}
	})
}

func (a *app) viewSubtitlesTab(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Gap(u * 0.5).Padding(u * 0.5).Children(func() {

		if len(a.subtitleTracks) == 0 {
			ui.Text(c, "No subtitles found in this video (no embedded streams or sidecar .srt/.vtt files).").
				TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			return
		}

		// Track selector row
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u).Children(func() {
			ui.Text(c, "Track:").FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)

			// "Off"
			offVar := input.Ghost
			if !a.subtitlesOn {
				offVar = input.Secondary
			}
			if input.Button(c, "Off", input.ButtonOptions{Variant: offVar}).Clicked() {
				a.onSubtitles(false)
			}

			// One button per track
			for i, tr := range a.subtitleTracks {
				idx := i
				trName := tr.Name
				variant := input.Ghost
				if a.subtitlesOn && a.activeTrackIdx == idx {
					variant = input.Primary
				}
				if input.Button(c, trName, input.ButtonOptions{Variant: variant}).Clicked() {
					a.activeTrackIdx = idx
					a.subtitlesOn = true
					a.statusMsg = "Subtitles: " + trName
					if a.menuSubtitlesItem != nil {
						a.menuSubtitlesItem.SetChecked(true)
					}
				}
			}
		})

		// Cue list (compact scroll)
		cues := a.activeCues()
		if len(cues) == 0 {
			return
		}
		ui.Scroll(c).MaxHeight(90).Children(func() {
			ui.Column(c).Gap(u * 0.5).Children(func() {
				for _, cue := range cues {
					cueCopy := cue
					isCurrent := a.position >= cue.Start && a.position <= cue.End
					textColor := k.TextMuted
					if isCurrent {
						textColor = k.Accent
					}
					ui.Row(c).Gap(u).AlignItems(ui.Center).Children(func() {
						ts := fmt.Sprintf("%s–%s", core.MediaClock(cueCopy.Start), core.MediaClock(cueCopy.End))
						if input.Button(c, ts, input.ButtonOptions{Variant: input.Ghost}).Clicked() {
							a.onSeek(cueCopy.Start)
						}
						ui.Text(c, cueCopy.Text).TextColor(textColor).
							FontSize(core.FontSize(c, theme.CaptionSize)).SingleLine()
					})
				}
			})
		})
	})
}

func (a *app) viewDropZoneTab(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Padding(u * 0.5).Gap(u * 0.5).Children(func() {
		ui.Text(c, "Drop videos here to add to playlist:").
			FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)

		prevLen := len(a.droppedFiles)
		// Compact drop zone — MaxHeight limits its size so it doesn't fill the drawer
		ui.Box(c).FillWidth().MaxHeight(90).Children(func() {
			zoneRes := input.FileDropZone(c, &a.droppedFiles, input.FileDropZoneOptions{
				Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v", "ts", "mts"},
				Hint:       "Drop video files here",
			})
			if zoneRes.Changed() && len(a.droppedFiles) > prevLen {
				for i := prevLen; i < len(a.droppedFiles); i++ {
					f := a.droppedFiles[i]
					a.loadFile(f.Path, i == prevLen)
				}
			}
		})
	})
}

func (a *app) viewInfoTab(c *ui.Context, k theme.Tokens, u float32) {
	ui.Column(c).Padding(u * 0.5).Gap(u * 0.5).Children(func() {
		if a.info == nil {
			ui.Text(c, "No video loaded.").TextColor(k.TextMuted).
				FontSize(core.FontSize(c, theme.CaptionSize))
			return
		}
		ui.Row(c).Gap(u * 3).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(u * 0.5).Children(func() {
				ui.Text(c, a.info.Title).Bold().FontSize(core.FontSize(c, theme.BodySize))
				ui.Text(c, a.info.Path).TextColor(k.TextMuted).
					FontSize(core.FontSize(c, theme.CaptionSize))
				if a.info.FileSize > 0 {
					ui.Text(c, fmt.Sprintf("%.2f MB", float64(a.info.FileSize)/(1024*1024))).
						TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				}
			})
			ui.Column(c).Gap(u * 0.5).Children(func() {
				ui.Text(c, fmt.Sprintf("%d×%d", a.info.Width, a.info.Height)).Bold().
					FontSize(core.FontSize(c, theme.BodySize))
				ui.Text(c, fmt.Sprintf("%.2f fps  •  %s", a.info.FPS, core.MediaClock(a.info.Duration))).
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				ui.Text(c, a.info.VideoCodec+" / "+a.info.AudioCodec).
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			})
		})
	})
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	a := newApp()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "MyGo Video Player",
			Width:           1280,
			Height:          800,
			MinWidth:        700,
			MinHeight:       500,
			Maximized:       true,
			FullScreen:      false,
			BackgroundColor: "#0B0A0B",
			Content:         ui.View(a.view),
		})
		a.win = win

		// Install native menu bar
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
		win.OnClosed(func() {
			a.engine.Close()
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Close halts the playback engine.
func (e *PlaybackEngine) Close() {
	e.Stop()
}
