package main

import (
	"fmt"
	"image"
	"log"
	"path/filepath"
	"slices"
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
	loop         bool
	info         *VideoInfo
	playlist     []media.PlaylistItem
	currentID    string
	cues         []SubtitleCue
	pickedPaths  []string
	droppedFiles []input.DroppedFile
	selectedTab  int
	darkMode     bool
	statusMsg    string
}

func newApp() *app {
	a := &app{
		rate:        1.0,
		volume:      0.8,
		subtitlesOn: true,
		fullscreen:  true,
		darkMode:    true,
		statusMsg:   "Ready",
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
				a.win.Update(func() {
					a.onPlaybackEnded()
				})
			} else {
				a.onPlaybackEnded()
			}
		},
	)

	return a
}

func (a *app) loadFile(path string, autoplay bool) {
	if path == "" {
		return
	}
	a.statusMsg = "Loading: " + filepath.Base(path)

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
		bm := ui.NewBitmap(img)
		cues := LoadSubtitles(path)

		apply := func() {
			a.info = info
			a.duration = info.Duration
			a.position = 0
			a.frame = bm
			a.cues = cues
			a.currentID = path
			a.statusMsg = "Playing: " + info.Title

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
}

func (a *app) onPiP(on bool) {
	a.pip = on
	if a.win != nil {
		a.win.SetAlwaysOnTop(on)
	}
	if on {
		a.statusMsg = "Picture-in-Picture (Pinned on top)"
	} else {
		a.statusMsg = "PiP Disabled"
	}
}

func (a *app) onPlaybackEnded() {
	if a.loop {
		a.onSeek(0)
		a.onPlay()
		return
	}
	// Advance to next playlist item if available
	if len(a.playlist) > 1 && a.currentID != "" {
		for i, it := range a.playlist {
			if it.ID == a.currentID && i+1 < len(a.playlist) {
				a.playTrackByID(a.playlist[i+1].ID)
				return
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
	if len(a.playlist) <= 1 {
		return
	}
	for i, it := range a.playlist {
		if it.ID == a.currentID {
			nextIdx := (i + 1) % len(a.playlist)
			a.playTrackByID(a.playlist[nextIdx].ID)
			return
		}
	}
	a.playTrackByID(a.playlist[0].ID)
}

func (a *app) prevTrack() {
	if len(a.playlist) <= 1 {
		return
	}
	for i, it := range a.playlist {
		if it.ID == a.currentID {
			prevIdx := (i - 1 + len(a.playlist)) % len(a.playlist)
			a.playTrackByID(a.playlist[prevIdx].ID)
			return
		}
	}
	a.playTrackByID(a.playlist[0].ID)
}

func (a *app) view(c *ui.Context) {
	mode := core.Dark
	if !a.darkMode {
		mode = core.Light
	}
	core.Use(c, core.Settings{Mode: mode})

	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	// Outer main container with scroll and padding
	ui.Column(c).Fill().Background(k.Background).Padding(u*2, u*3, u*3, u*3).Gap(u * 2).Children(func() {
		// 1. Top Header Bar
		ui.Row(c).Gap(u * 2).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("play")).Size(20, 20).TextColor(k.Accent)
			ui.Text(c, "MyGo Video Player").FontSize(core.FontSize(c, theme.H2Size)).Bold().TextColor(k.Text)
			display.Badge(c, display.BadgeOptions{Text: "MujicaUI", Tone: display.BadgeAccent, Position: display.BadgeInline}, nil)

			ui.Box(c).Grow(1)

			// File Picker Component
			picker := input.FilePicker(c, &a.pickedPaths, input.FilePickerOptions{
				Title: "Open Video File",
				Label: "Open File…",
				Filters: []mygo.FileFilter{
					{Name: "Video Files", Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v"}},
					{Name: "All Files", Extensions: []string{"*"}},
				},
			})
			if picker.Changed() && len(a.pickedPaths) > 0 {
				a.loadFile(a.pickedPaths[len(a.pickedPaths)-1], true)
			}

			// Quick Demo Clip Button
			if input.Button(c, "Load Demo", input.ButtonOptions{Variant: input.Secondary, Icon: icons.Must("play")}).Clicked() {
				a.loadFile("sample.mp4", true)
			}

			// Theme Toggle
			themeIcon := icons.Must("sun")
			themeLabel := "Light Mode"
			if !a.darkMode {
				themeIcon = icons.Must("moon")
				themeLabel = "Dark Mode"
			}
			if input.Button(c, "", input.ButtonOptions{Variant: input.Ghost, Icon: themeIcon, Label: themeLabel}).Clicked() {
				a.darkMode = !a.darkMode
			}
		})

		// 2. Main Stage Video Player
		var frameSrc ui.ImageSource
		if a.frame != nil {
			frameSrc = a.frame
		}

		currentSub := ""
		if a.subtitlesOn {
			currentSub = GetSubtitleAt(a.cues, a.position)
		}

		rate := a.rate
		if rate <= 0 {
			rate = 1.0
		}

		st := media.VideoState{
			Frame:       frameSrc,
			Position:    a.position,
			Duration:    a.duration,
			Playing:     a.playing,
			Rate:        rate,
			Subtitle:    currentSub,
			SubtitlesOn: a.subtitlesOn,
			Fullscreen:  a.fullscreen,
			PiP:         a.pip,
		}

		opts := media.VideoPlayerOptions{
			OnPlay:       a.onPlay,
			OnPause:      a.onPause,
			OnSeek:       a.onSeek,
			OnRate:       a.onRate,
			OnSubtitles:  a.onSubtitles,
			OnFullscreen: a.onFullscreen,
			OnPiP:        a.onPiP,
			Label:        "Main Video Player",
		}

		media.VideoPlayer(c, st, opts)

		// 3. Quick Playback Controls & Volume Bar
		ui.Row(c).Padding(u, u*2, u, u*2).Background(k.Surface).Radius(theme.ControlRadius).
			Border(theme.BorderWidth, k.Border).AlignItems(ui.Center).Gap(u * 2).Children(func() {
			// Volume Control
			volRes := media.VolumeControl(c, &a.volume, &a.muted, media.VolumeControlOptions{})
			if volRes.Changed() {
				a.engine.SetVolume(a.volume, a.muted)
			}

			ui.Box(c).Width(1).Height(20).Background(k.Border)

			// Step Back 5s
			if input.Button(c, "-5s", input.ButtonOptions{Variant: input.Ghost, Icon: icons.Must("rotate-ccw")}).Clicked() {
				a.onSeek(a.position - 5*time.Second)
			}

			// Restart (t=0)
			if input.Button(c, "", input.ButtonOptions{Variant: input.Ghost, Icon: core.MediaPrev, Label: "Restart"}).Clicked() {
				a.onSeek(0)
			}

			// Step Forward 5s
			if input.Button(c, "+5s", input.ButtonOptions{Variant: input.Ghost, Icon: icons.Must("refresh-cw")}).Clicked() {
				a.onSeek(a.position + 5*time.Second)
			}

			// Repeat Toggle
			repeatVariant := input.Ghost
			if a.loop {
				repeatVariant = input.Secondary
			}
			if input.Button(c, "Loop", input.ButtonOptions{Variant: repeatVariant, Icon: core.MediaRepeat}).Clicked() {
				a.loop = !a.loop
				if a.loop {
					a.statusMsg = "Loop: Enabled"
				} else {
					a.statusMsg = "Loop: Disabled"
				}
			}

			// Prev / Next Playlist Items
			if len(a.playlist) > 1 {
				if input.Button(c, "", input.ButtonOptions{Variant: input.Ghost, Icon: core.MediaPrev, Label: "Previous Video"}).Clicked() {
					a.prevTrack()
				}
				if input.Button(c, "", input.ButtonOptions{Variant: input.Ghost, Icon: core.MediaNext, Label: "Next Video"}).Clicked() {
					a.nextTrack()
				}
			}

			ui.Box(c).Grow(1)

			// Status message and current video info
			if a.info != nil {
				display.Badge(c, display.BadgeOptions{
					Text:     fmt.Sprintf("%s • %dx%d • %.0ffps", a.info.VideoCodec, a.info.Width, a.info.Height, a.info.FPS),
					Tone:     display.BadgeNeutral,
					Position: display.BadgeInline,
				}, nil)
			}
			ui.Text(c, a.statusMsg).FontSize(core.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
		})

		// 4. Feature Tabs: Playlist, Details, Drop Zone, Subtitles
		tabs := []navigation.Tab{
			{
				Label: fmt.Sprintf("Playlist (%d)", len(a.playlist)),
				Icon:  icons.Must("list"),
				Panel: func() {
					a.viewPlaylistTab(c)
				},
			},
			{
				Label: "Video Details",
				Icon:  icons.Must("info"),
				Panel: func() {
					a.viewInfoTab(c)
				},
			},
			{
				Label: "Drop Zone / Import",
				Icon:  icons.Must("upload"),
				Panel: func() {
					a.viewDropZoneTab(c)
				},
			},
			{
				Label: fmt.Sprintf("Subtitles (%d)", len(a.cues)),
				Icon:  core.MediaCaptions,
				Panel: func() {
					a.viewSubtitlesTab(c)
				},
			},
		}

		navigation.Tabs(c, &a.selectedTab, tabs, navigation.TabsOptions{Label: "Player Sections"})
	})
}

func (a *app) viewPlaylistTab(c *ui.Context) {
	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	ui.Column(c).Gap(u * 2).Padding(u).Children(func() {
		if len(a.playlist) == 0 {
			ui.Row(c).Padding(u * 3).Center().Children(func() {
				ui.Text(c, "No videos in playlist. Click 'Open File…' or 'Load Demo' above to add tracks.").
					TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.BodySize))
			})
			return
		}

		res := media.Playlist(c, &a.playlist, media.PlaylistOptions{
			Current: a.currentID,
			Height:  160,
		})
		if res.Played != "" {
			a.playTrackByID(res.Played)
		}
		if res.Removed != "" {
			a.removeTrackByID(res.Removed)
		}
	})
}

func (a *app) viewInfoTab(c *ui.Context) {
	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	ui.Column(c).Padding(u * 2).Gap(u * 2).Children(func() {
		if a.info == nil {
			ui.Text(c, "No video currently loaded.").TextColor(k.TextMuted)
			return
		}

		ui.Row(c).Gap(u * 4).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(u).Children(func() {
				ui.Text(c, "File: "+a.info.Title).Bold()
				ui.Text(c, "Path: "+a.info.Path).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				sizeMB := float64(a.info.FileSize) / (1024 * 1024)
				ui.Text(c, fmt.Sprintf("Size: %.2f MB", sizeMB)).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			})

			ui.Column(c).Gap(u).Children(func() {
				ui.Text(c, fmt.Sprintf("Resolution: %d × %d", a.info.Width, a.info.Height)).Bold()
				ui.Text(c, fmt.Sprintf("Frame Rate: %.2f FPS", a.info.FPS)).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				ui.Text(c, fmt.Sprintf("Duration: %s", core.MediaClock(a.info.Duration))).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			})

			ui.Column(c).Gap(u).Children(func() {
				ui.Text(c, "Video Codec: "+a.info.VideoCodec).Bold()
				ui.Text(c, "Audio Codec: "+a.info.AudioCodec).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
				speedText := fmt.Sprintf("%.2f×", a.rate)
				ui.Text(c, "Speed: "+speedText).TextColor(k.TextMuted).FontSize(core.FontSize(c, theme.CaptionSize))
			})
		})
	})
}

func (a *app) viewDropZoneTab(c *ui.Context) {
	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	ui.Column(c).Padding(u * 2).Gap(u * 2).Children(func() {
		ui.Text(c, "Drag and drop any video files from your computer below:").TextColor(k.TextMuted)

		prevLen := len(a.droppedFiles)
		zoneRes := input.FileDropZone(c, &a.droppedFiles, input.FileDropZoneOptions{
			Extensions: []string{"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v"},
			Hint:       "Drop video files here to add them to your player playlist",
		})

		if zoneRes.Changed() && len(a.droppedFiles) > prevLen {
			for i := prevLen; i < len(a.droppedFiles); i++ {
				f := a.droppedFiles[i]
				a.loadFile(f.Path, i == prevLen)
			}
		}
	})
}

func (a *app) viewSubtitlesTab(c *ui.Context) {
	k := core.Tokens(c)
	d := core.Density(c)
	u := d.Unit()

	ui.Column(c).Padding(u * 2).Gap(u * 2).Children(func() {
		ui.Row(c).Gap(u * 2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Closed Captions / Subtitles:").Bold()
			toggleLabel := "Turn Off"
			toggleVariant := input.Secondary
			if !a.subtitlesOn {
				toggleLabel = "Turn On"
				toggleVariant = input.Outline
			}
			if input.Button(c, toggleLabel, input.ButtonOptions{Variant: toggleVariant}).Clicked() {
				a.onSubtitles(!a.subtitlesOn)
			}
		})

		if len(a.cues) == 0 {
			ui.Text(c, "No subtitle cues found for this video.").TextColor(k.TextMuted)
			return
		}

		ui.Scroll(c).MaxHeight(140).Children(func() {
			ui.Column(c).Gap(u).Children(func() {
				for _, cue := range a.cues {
					cueCopy := cue
					isCurrent := a.position >= cue.Start && a.position <= cue.End
					textColor := k.TextMuted
					if isCurrent {
						textColor = k.Accent
					}

					ui.Row(c).Gap(u * 2).AlignItems(ui.Center).Children(func() {
						timeLabel := fmt.Sprintf("[%s - %s]", core.MediaClock(cueCopy.Start), core.MediaClock(cueCopy.End))
						if input.Button(c, timeLabel, input.ButtonOptions{Variant: input.Ghost}).Clicked() {
							a.onSeek(cueCopy.Start)
						}
						ui.Text(c, cueCopy.Text).TextColor(textColor).SingleLine()
					})
				}
			})
		})
	})
}

func main() {
	a := newApp()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "MyGo Video Player",
			Width:           1280,
			Height:          800,
			MinWidth:        640,
			MinHeight:       520,
			FullScreen:      true,
			BackgroundColor: "#191618",
			Content:         ui.View(a.view),
		})
		a.win = win

		win.SetFullScreen(true)

		win.OnEnterFullScreen(func() {
			win.Update(func() {
				a.fullscreen = true
			})
		})
		win.OnLeaveFullScreen(func() {
			win.Update(func() {
				a.fullscreen = false
			})
		})

		// Cleanup processes on window close
		win.OnClosed(func() {
			a.engine.Close()
		})

		// Preload sample.mp4 demo clip on startup
		a.loadFile("sample.mp4", false)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Close halts playback engine.
func (e *PlaybackEngine) Close() {
	e.Stop()
}
