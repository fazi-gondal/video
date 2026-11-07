package main

import (
	"encoding/json"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// LoopMode represents how playlist and track looping behaves.
type LoopMode int

const (
	LoopOff LoopMode = iota
	LoopOne
	LoopAll
)

// PlaylistItem represents one item in the queue/playlist.
type PlaylistItem struct {
	ID       string
	Title    string
	Subtitle string
	Duration time.Duration
	IsAudio  bool
}

// SubtitleTrack is one subtitle stream or sidecar file.
type SubtitleTrack struct {
	Name string
	Cues []SubtitleCue
}

// Global SVG Icons
var (
	iconPlay = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><polygon points="6,4 20,12 6,20"/></svg>`))
	iconPause = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/></svg>`))
	iconPrev = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><polygon points="19,20 9,12 19,4"/><rect x="5" y="4" width="2.5" height="16" rx="1"/></svg>`))
	iconNext = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><polygon points="5,4 15,12 5,20"/><rect x="16.5" y="4" width="2.5" height="16" rx="1"/></svg>`))
	iconBack5 = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><text x="12" y="15.5" font-size="7" font-family="sans-serif" font-weight="bold" fill="currentColor" stroke="none" text-anchor="middle">5</text></svg>`))
	iconFwd5 = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><text x="12" y="15.5" font-size="7" font-family="sans-serif" font-weight="bold" fill="currentColor" stroke="none" text-anchor="middle">5</text></svg>`))
	iconVolume = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" fill="currentColor"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/></svg>`))
	iconMute = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" fill="currentColor"/><line x1="22" y1="9" x2="16" y2="15"/><line x1="16" y1="9" x2="22" y2="15"/></svg>`))
	iconCaptions = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><path d="M7 15h3a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2H7v6z"/><path d="M14 15h3a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2h-3v6z"/></svg>`))
	iconRepeat = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>`))
	iconRepeatOne = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/><text x="12" y="14.5" font-size="7" font-family="sans-serif" font-weight="bold" fill="currentColor" stroke="none" text-anchor="middle">1</text></svg>`))
	iconPiP = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><rect x="12" y="10" width="8" height="6" rx="1" fill="currentColor"/></svg>`))
	iconFullscreen = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"/></svg>`))
	iconPanel = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M15 3v18"/></svg>`))
	iconList = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>`))
	iconUpload = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>`))
	iconFilm = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="20" rx="2.18"/><line x1="7" y1="2" x2="7" y2="22"/><line x1="17" y1="2" x2="17" y2="22"/><line x1="2" y1="12" x2="22" y2="12"/><line x1="2" y1="7" x2="7" y2="7"/><line x1="2" y1="17" x2="7" y2="17"/><line x1="17" y1="17" x2="22" y2="17"/><line x1="17" y1="7" x2="22" y2="7"/></svg>`))
	iconMusic = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>`))
	iconTrash = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`))
)

func formatClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Round(time.Second).Seconds())
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
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
	subtitlesOn     bool
	subtitleTracks  []SubtitleTrack
	activeTrackIdx  int // -1 = none
	subFontSize     float32
	subBottomOffset float32

	// Window / UI state
	fullscreen bool
	pip        bool
	loopMode   LoopMode
	showPanel  bool // right-side panel (playlist + subtitles)
	panelTab   int  // 0=playlist, 1=subtitles, 2=drop
	darkMode   bool
	statusMsg  string

	// Current media
	info      *VideoInfo
	playlist  []PlaylistItem
	currentID string

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
		subtitlesOn:     false,
		activeTrackIdx:  -1,
		subFontSize:     18,
		subBottomOffset: 28,
		showPanel:       false,
		darkMode:       true,
		statusMsg:      "Open audio or video with File > Open… (Ctrl+O)",
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

func (a *app) loadSubtitleFile(path string) {
	cues, err := ParseSRTFile(path)
	if err != nil || len(cues) == 0 {
		return
	}
	name := filepath.Base(path)
	a.subtitleTracks = append(a.subtitleTracks, SubtitleTrack{Name: name, Cues: cues})
	a.activeTrackIdx = len(a.subtitleTracks) - 1
	a.onSubtitles(true)
	a.statusMsg = "Loaded subtitles: " + name
}

// ── Media Loading & Probing ───────────────────────────────────────────────────

func buildSubtitleTracks(videoPath string) []SubtitleTrack {
	if videoPath == "" {
		return nil
	}
	var tracks []SubtitleTrack

	// 1. Check embedded subtitle streams with ffprobe if present
	if _, err := exec.LookPath("ffprobe"); err == nil {
		type probeStream struct {
			Index int               `json:"index"`
			Codec string            `json:"codec_name"`
			Tags  map[string]string `json:"tags"`
		}
		type probeResult struct {
			Streams []probeStream `json:"streams"`
		}

		cmd := exec.Command("ffprobe", "-v", "quiet",
			"-print_format", "json",
			"-show_streams",
			"-select_streams", "s",
			videoPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, err := cmd.Output(); err == nil {
			var pr probeResult
			if jsonErr := json.Unmarshal(out, &pr); jsonErr == nil {
				for _, st := range pr.Streams {
					lang := st.Tags["language"]
					title := st.Tags["title"]
					name := "Track " + fmt.Sprint(st.Index)
					if title != "" {
						name = title
					} else if lang != "" {
						name = strings.ToUpper(lang)
					}
					cues := extractEmbeddedSubtitles(videoPath, st.Index)
					if len(cues) > 0 {
						tracks = append(tracks, SubtitleTrack{Name: name, Cues: cues})
					}
				}
			}
		}
	}

	// 2. Adjacent sidecar files (.srt, .vtt) — works without ffmpeg!
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	dir := filepath.Dir(videoPath)
	for _, ext := range []string{".srt", ".vtt"} {
		sidecar := base + ext
		if _, err := os.Stat(sidecar); err == nil {
			if cues, err := ParseSRTFile(sidecar); err == nil && len(cues) > 0 {
				tracks = append(tracks, SubtitleTrack{
					Name: filepath.Base(sidecar),
					Cues: cues,
				})
			}
		}
	}
	dirSrt := filepath.Join(dir, "subtitles.srt")
	if _, err := os.Stat(dirSrt); err == nil {
		if cues, err := ParseSRTFile(dirSrt); err == nil && len(cues) > 0 {
			tracks = append(tracks, SubtitleTrack{
				Name: "subtitles.srt",
				Cues: cues,
			})
		}
	}
	return tracks
}

func extractEmbeddedSubtitles(videoPath string, streamIdx int) []SubtitleCue {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}
	cmd := exec.Command("ffmpeg", "-v", "quiet",
		"-i", videoPath,
		"-map", fmt.Sprintf("0:%d", streamIdx),
		"-f", "srt",
		"pipe:1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	cues, _ := ParseSRTBytes(out)
	return cues
}

func (a *app) loadFile(path string, autoplay bool) {
	go func() {
		info, err := ProbeVideo(path)
		if err != nil {
			log.Printf("probe error: %v", err)
			return
		}
		tracks := buildSubtitleTracks(path)

		apply := func() {
			a.info = info
			a.duration = info.Duration
			a.position = 0
			a.currentID = path
			a.subtitleTracks = tracks
			if len(tracks) > 0 {
				a.activeTrackIdx = 0
				a.subtitlesOn = true
			} else {
				a.activeTrackIdx = -1
				a.subtitlesOn = false
			}
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
				subtitleText := fmt.Sprintf("%dx%d · %s", info.Width, info.Height, formatClock(info.Duration))
				if info.IsAudio {
					subtitleText = fmt.Sprintf("%s · %s", info.AudioCodec, formatClock(info.Duration))
				}
				a.playlist = append(a.playlist, PlaylistItem{
					ID:       path,
					Title:    info.Title,
					Subtitle: subtitleText,
					Duration: info.Duration,
					IsAudio:  info.IsAudio,
				})
			}
			a.engine.Load(path, info.Duration, info.IsAudio)
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

// ── Playback Controls ─────────────────────────────────────────────────────────

func (a *app) onPlay() {
	if a.info == nil {
		if len(a.playlist) > 0 {
			a.playTrackByID(a.playlist[0].ID)
		}
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
	if a.info == nil {
		return
	}
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

func (a *app) cycleRate() {
	rates := []float64{0.5, 0.75, 1.0, 1.25, 1.5, 2.0}
	next := 1.0
	for i, r := range rates {
		if a.rate == r && i+1 < len(rates) {
			next = rates[i+1]
			break
		}
	}
	if a.rate >= 2.0 {
		next = 0.5
	}
	a.onRate(next)
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
	if a.loopMode == LoopOne {
		a.onSeek(0)
		a.onPlay()
		return
	}
	if len(a.playlist) > 1 {
		for i, it := range a.playlist {
			if it.ID == a.currentID {
				next := (i + 1) % len(a.playlist)
				if a.loopMode == LoopAll || next != 0 {
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
	a.playlist = slices.DeleteFunc(a.playlist, func(it PlaylistItem) bool { return it.ID == id })
	if a.currentID == id {
		if len(a.playlist) > 0 {
			a.playTrackByID(a.playlist[0].ID)
		} else {
			a.engine.Stop()
			a.info = nil
			a.frame = nil
			a.position = 0
			a.duration = 0
			a.playing = false
			a.currentID = ""
			a.statusMsg = "Playlist empty"
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

func (a *app) activeCues() []SubtitleCue {
	if !a.subtitlesOn || a.activeTrackIdx < 0 || a.activeTrackIdx >= len(a.subtitleTracks) {
		return nil
	}
	return a.subtitleTracks[a.activeTrackIdx].Cues
}

func (a *app) currentSubtitle() string {
	return GetSubtitleAt(a.activeCues(), a.position)
}

// ── Native Menu Bar ───────────────────────────────────────────────────────────

func (a *app) buildMenu() []*mygo.MenuItem {
	subsItem := &mygo.MenuItem{
		ID: "subs", Label: "Show Subtitles", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+T", Disabled: true,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onSubtitles(it.IsChecked()) })
			}
		},
	}
	a.menuSubtitlesItem = subsItem

	fsItem := &mygo.MenuItem{
		ID: "fullscreen", Label: "Full Screen", Type: mygo.MenuItemCheckbox,
		Accelerator: "F11",
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.onFullscreen(it.IsChecked()) })
			}
		},
	}
	a.menuFullscreenItem = fsItem

	panelItem := &mygo.MenuItem{
		ID: "panel", Label: "Toggle Side Panel", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+P", Checked: a.showPanel,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() {
					a.showPanel = it.IsChecked()
				})
			}
		},
	}
	a.menuPanelItem = panelItem

	loopItem := &mygo.MenuItem{
		ID: "loop", Label: "Loop Playlist", Type: mygo.MenuItemCheckbox,
		Accelerator: "CmdOrCtrl+L", Checked: false,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() {
					if it.IsChecked() {
						a.loopMode = LoopAll
					} else {
						a.loopMode = LoopOff
					}
				})
			}
		},
	}
	a.menuLoopItem = loopItem

	darkItem := &mygo.MenuItem{
		ID: "dark", Label: "Dark Theme", Type: mygo.MenuItemCheckbox, Checked: true,
		Click: func(it *mygo.MenuItem, win *mygo.Window) {
			if win != nil {
				win.Update(func() { a.darkMode = it.IsChecked() })
			}
		},
	}
	a.menuDarkItem = darkItem

	return []*mygo.MenuItem{
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open Media…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				go func() {
					path, err := openMediaFileDialog()
					if err != nil || path == "" {
						return
					}
					if win != nil {
						win.Update(func() { a.loadFile(path, true) })
					}
				}()
			}},
			{Label: "Open Audio File…", Accelerator: "CmdOrCtrl+Shift+A", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				go func() {
					path, err := openAudioFileDialog()
					if err != nil || path == "" {
						return
					}
					if win != nil {
						win.Update(func() { a.loadFile(path, true) })
					}
				}()
			}},
			{Label: "Open Video File…", Accelerator: "CmdOrCtrl+Shift+V", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				go func() {
					path, err := openVideoFileDialog()
					if err != nil || path == "" {
						return
					}
					if win != nil {
						win.Update(func() { a.loadFile(path, true) })
					}
				}()
			}},
			{Label: "Open Subtitle File…", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				go func() {
					path, err := openNativeSubtitleDialog()
					if err != nil || path == "" {
						return
					}
					if win != nil {
						win.Update(func() { a.loadSubtitleFile(path) })
					}
				}()
			}},
			mygo.Separator(),
			{Label: "Side Panel: Playlist", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.showPanel = true
						a.panelTab = 0
						if a.menuPanelItem != nil {
							a.menuPanelItem.SetChecked(true)
						}
					})
				}
			}},
			{Label: "Side Panel: Subtitles", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.showPanel = true
						a.panelTab = 1
						if a.menuPanelItem != nil {
							a.menuPanelItem.SetChecked(true)
						}
					})
				}
			}},
			{Label: "Side Panel: Drop Zone", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.showPanel = true
						a.panelTab = 2
						if a.menuPanelItem != nil {
							a.menuPanelItem.SetChecked(true)
						}
					})
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleClose},
			{Role: mygo.RoleQuit},
		}},
		{Label: "Playback", Submenu: []*mygo.MenuItem{
			{Label: "Play / Pause", Accelerator: "Space", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() { a.togglePlay() })
				}
			}},
			mygo.Separator(),
			{Label: "Step Back 5 s", Accelerator: "Left", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() { a.onSeek(a.position - 5*time.Second) })
				}
			}},
			{Label: "Step Forward 5 s", Accelerator: "Right", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() { a.onSeek(a.position + 5*time.Second) })
				}
			}},
			mygo.Separator(),
			{Label: "Previous Track", Accelerator: "CmdOrCtrl+Left", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() { a.prevTrack() })
				}
			}},
			{Label: "Next Track", Accelerator: "CmdOrCtrl+Right", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() { a.nextTrack() })
				}
			}},
			mygo.Separator(),
			loopItem,
		}},
		{Label: "Audio", Submenu: []*mygo.MenuItem{
			{Label: "Mute", Accelerator: "CmdOrCtrl+M", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.muted = !a.muted
						a.engine.SetVolume(a.volume, a.muted)
					})
				}
			}},
			{Label: "Volume Up", Accelerator: "Up", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.volume = min(1.0, a.volume+0.05)
						a.muted = false
						a.engine.SetVolume(a.volume, a.muted)
					})
				}
			}},
			{Label: "Volume Down", Accelerator: "Down", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.volume = max(0.0, a.volume-0.05)
						a.engine.SetVolume(a.volume, a.muted)
					})
				}
			}},
		}},
		{Label: "Subtitle", Submenu: []*mygo.MenuItem{
			subsItem,
			{Label: "Add Subtitle File…", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				go func() {
					path, err := openNativeSubtitleDialog()
					if err != nil || path == "" {
						return
					}
					if win != nil {
						win.Update(func() { a.loadSubtitleFile(path) })
					}
				}()
			}},
			mygo.Separator(),
			{Label: "Increase Subtitle Size", Accelerator: "CmdOrCtrl+=", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						if a.subFontSize < 40 {
							a.subFontSize += 2
						}
					})
				}
			}},
			{Label: "Decrease Subtitle Size", Accelerator: "CmdOrCtrl+-", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						if a.subFontSize > 12 {
							a.subFontSize -= 2
						}
					})
				}
			}},
			mygo.Separator(),
			{Label: "Move Subtitle Up", Accelerator: "Alt+Up", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						if a.subBottomOffset < 300 {
							a.subBottomOffset += 15
						}
					})
				}
			}},
			{Label: "Move Subtitle Down", Accelerator: "Alt+Down", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						if a.subBottomOffset > 10 {
							a.subBottomOffset -= 15
						}
					})
				}
			}},
			{Label: "Reset Subtitle Style", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil {
					win.Update(func() {
						a.subFontSize = 18
						a.subBottomOffset = 28
					})
				}
			}},
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			panelItem,
			mygo.Separator(),
			fsItem,
			{Role: mygo.RoleToggleFullScreen},
			mygo.Separator(),
			darkItem,
		}},
	}
}

// ── Main UI View ──────────────────────────────────────────────────────────────

func (a *app) view(c *ui.Context) {
	th := ui.DarkTheme()
	if !a.darkMode {
		th = ui.LightTheme()
	}
	c.SetTheme(th)

	// Global drag-and-drop listener on root container
	root := ui.Column(c).Fill().Background(th.Background).Gap(0)
	if files := root.DroppedFiles(); len(files) > 0 {
		for i, f := range files {
			a.loadFile(f, i == 0 && a.info == nil)
		}
	}

	root.Children(func() {
		// ── Video Stage & Right Side Panel ─────────────────────────
		ui.Row(c).Grow(1).FillWidth().Gap(0).Children(func() {
			a.viewStage(c, th)
			if a.showPanel {
				a.viewPanel(c, th)
			}
		})

		// ── Scrubber Seek Bar ──────────────────────────────────────
		a.viewScrubber(c, th)

		// ── Controls Toolbar ───────────────────────────────────────
		a.viewControls(c, th)
	})
}

// viewStage: Focusable viewport with audio visualization, video frames, and subtitles overlay
func (a *app) viewStage(c *ui.Context, th *ui.Theme) {
	stage := ui.Box(c).Grow(1).FillWidth().
		Background(ui.Hex("#080808")).Center().Focusable().
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
	if stage.Clicked() && a.info != nil {
		a.togglePlay()
	}

	// Dropped files on stage
	if dropped := stage.DroppedFiles(); len(dropped) > 0 {
		for i, f := range dropped {
			a.loadFile(f, i == 0)
		}
	}

	stage.Children(func() {
		if a.frame == nil {
			// VLC-style empty state
			ui.Column(c).Center().Gap(14).Children(func() {
				ui.Row(c).Gap(12).Center().Children(func() {
					ui.Icon(c, iconFilm).Size(50, 50).TextColor(ui.Hex("#3A3A3A"))
					ui.Icon(c, iconMusic).Size(50, 50).TextColor(ui.Hex("#3A3A3A"))
				})
				ui.Text(c, "No media loaded").
					TextColor(ui.Hex("#777777")).FontSize(20).Bold()
				ui.Text(c, "Open a file or drop audio & video files here").
					TextColor(ui.Hex("#555555")).FontSize(13)
				openBtn := ui.Button(c, "Open Media File…").
					Padding(8, 20, 8, 20).Radius(6)
				if openBtn.Clicked() {
					go func() {
						path, err := openMediaFileDialog()
						if err != nil || path == "" {
							return
						}
						if a.win != nil {
							a.win.Update(func() { a.loadFile(path, true) })
						}
					}()
				}
				ui.Text(c, "MP4, MKV, WebM, AVI, MOV, WMV, MP3, WAV, FLAC, AAC, M4A, OGG, WMA").
					TextColor(ui.Hex("#444444")).FontSize(11)
			})
		} else if a.info != nil && a.info.IsAudio {
			// Audio Player View: Animated audio waveform canvas + track information overlay
			ui.Box(c).Fill().Children(func() {
				ui.Image(c, a.frame).Fit(ui.Cover).Fill()

				// Centered track information overlay
				ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Center().Gap(10).PassThrough().Children(func() {
					ui.Box(c).Center().Padding(16).Radius(99).Background(ui.RGBA(12, 12, 16, 0.8)).Children(func() {
						ui.Icon(c, iconMusic).Size(48, 48).TextColor(th.Accent)
					})

					ui.Text(c, a.info.Title).Bold().FontSize(22).TextColor(ui.Hex("#FFFFFF"))

					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Padding(2, 8, 2, 8).Radius(4).Background(th.Accent.Alpha(0.25)).Children(func() {
							ui.Text(c, a.info.AudioCodec).FontSize(11).Bold().TextColor(th.Accent)
						})
						if a.info.FileSize > 0 {
							ui.Text(c, fmt.Sprintf("%.1f MB", float64(a.info.FileSize)/(1024*1024))).
								FontSize(12).TextColor(ui.Hex("#AAAAAA"))
						}
					})
				})
			})
		} else {
			// Video frame: Contain preserves original aspect ratio without distortion
			ui.Image(c, a.frame).Fit(ui.Contain).Fill()
		}

		// Subtitle overlay
		if sub := a.currentSubtitle(); a.subtitlesOn && sub != "" {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(a.subBottomOffset).
				Center().PassThrough().Children(func() {
				ui.Text(c, sub).
					TextColor(ui.Hex("#FFFFFF")).
					Background(ui.RGBA(0, 0, 0, 0.85)).
					Padding(6, 16, 6, 16).
					Radius(6).TextAlign(ui.Center).MaxLines(4).
					FontSize(a.subFontSize).Bold()
			})
		}
	})
}

// viewScrubber: VLC-style full-width seek bar with progress fill and hover thumb
func (a *app) viewScrubber(c *ui.Context, th *ui.Theme) {
	durSec := a.duration.Seconds()
	if durSec <= 0 {
		durSec = 1
	}
	posSec := min(max(0, a.position.Seconds()), durSec)

	container := ui.Box(c).FillWidth().Height(14).Background(th.Surface).Padding(0, 8, 0, 8).Center()
	container.Children(func() {
		s := ui.SliderBase(c, &posSec, 0, durSec).FillWidth().Height(14).Cursor(ui.CursorPointer)
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			ratio := float32(0)
			if durSec > 0 {
				ratio = float32(posSec / durSec)
				if ratio < 0 {
					ratio = 0
				}
				if ratio > 1 {
					ratio = 1
				}
			}
			trackH := float32(4)
			if s.Hovered() || s.Pressed() {
				trackH = 6
			}
			trackY := r.Y + (r.H-trackH)/2

			// Background track
			p.Fill(ui.Rect{X: r.X, Y: trackY, W: r.W, H: trackH}, ui.Hex("#333333"), trackH/2)
			// Filled progress
			p.Fill(ui.Rect{X: r.X, Y: trackY, W: r.W * ratio, H: trackH}, th.Accent, trackH/2)

			// Thumb knob on hover or dragging
			if s.Hovered() || s.Pressed() {
				thumbX := r.X + r.W*ratio
				thumbR := float32(5)
				p.Fill(ui.Rect{X: thumbX - thumbR, Y: r.Y + (r.H-thumbR*2)/2, W: thumbR * 2, H: thumbR * 2}, th.AccentText, thumbR)
			}
		})
		if s.Changed() {
			a.onSeek(time.Duration(posSec * float64(time.Second)))
		}
	})
}

// Helper to create responsive icon buttons
func (a *app) iconButton(c *ui.Context, th *ui.Theme, icon *ui.SVG, label, tooltip string, active bool) ui.Element {
	b := ui.Box(c).Center().Padding(6).Radius(6).Cursor(ui.CursorPointer).
		Label(label).Tooltip(tooltip)
	bg := ui.Transparent
	fg := th.Text
	if active {
		bg = th.Accent.Alpha(0.25)
		fg = th.Accent
	} else if b.Hovered() {
		bg = th.SurfaceHover
	}
	b.Background(bg)
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(fg)
	})
	return b
}

// viewControls: Single aligned compact controls row
func (a *app) viewControls(c *ui.Context, th *ui.Theme) {
	bg := ui.Hex("#141414")
	if !a.darkMode {
		bg = th.Surface
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).
		Background(bg).Padding(4, 12, 6, 12).Children(func() {

		// ── Left cluster: transport controls ───────────────────
		if len(a.playlist) > 1 {
			if a.iconButton(c, th, iconPrev, "Previous Track", "Previous (Ctrl+←)", false).Clicked() {
				a.prevTrack()
			}
		}

		if a.iconButton(c, th, iconBack5, "Step Back 5 s", "Back 5 s (←)", false).Clicked() {
			a.onSeek(a.position - 5*time.Second)
		}

		// Play / Pause circular primary button
		playIco := iconPlay
		if a.playing {
			playIco = iconPause
		}
		playBtn := ui.Box(c).Center().Radius(99).Width(32).Height(32).
			Cursor(ui.CursorPointer).Label("Play/Pause (Space)").Tooltip("Play/Pause (Space)")
		playBg := th.Accent
		if playBtn.Hovered() {
			playBg = th.AccentHover
		}
		playBtn.Background(playBg)
		playBtn.Children(func() {
			ui.Icon(c, playIco).Size(16, 16).TextColor(th.AccentText)
		})
		if playBtn.Clicked() {
			a.togglePlay()
		}

		if a.iconButton(c, th, iconFwd5, "Step Forward 5 s", "Forward 5 s (→)", false).Clicked() {
			a.onSeek(a.position + 5*time.Second)
		}

		if len(a.playlist) > 1 {
			if a.iconButton(c, th, iconNext, "Next Track", "Next (Ctrl+→)", false).Clicked() {
				a.nextTrack()
			}
		}

		// ── Clock display ──────────────────────────────────────
		clockText := formatClock(a.position) + " / " + formatClock(a.duration)
		ui.Text(c, clockText).
			FontSize(12).TextColor(ui.Hex("#999999")).SingleLine()

		// ── Middle expanding spacer ────────────────────────────
		ui.Box(c).Grow(1)

		// ── Volume Control: icon + aligned slider ──────────────
		muteIco := iconVolume
		if a.muted || a.volume == 0 {
			muteIco = iconMute
		}
		if a.iconButton(c, th, muteIco, "Mute", "Mute (M)", a.muted).Clicked() {
			a.muted = !a.muted
			a.engine.SetVolume(a.volume, a.muted)
		}

		volPercent := a.volume * 100
		volSlider := ui.SliderBase(c, &volPercent, 0, 100).Width(80).Height(20).Cursor(ui.CursorPointer)
		volSlider.Draw(func(p *ui.Painter, r ui.Rect) {
			ratio := float32(volPercent / 100)
			if ratio < 0 {
				ratio = 0
			}
			if ratio > 1 {
				ratio = 1
			}
			trackH := float32(4)
			trackY := r.Y + (r.H-trackH)/2
			p.Fill(ui.Rect{X: r.X, Y: trackY, W: r.W, H: trackH}, ui.Hex("#333333"), 2)
			p.Fill(ui.Rect{X: r.X, Y: trackY, W: r.W * ratio, H: trackH}, th.Accent, 2)
			thumbX := r.X + r.W*ratio
			thumbR := float32(5)
			p.Fill(ui.Rect{X: thumbX - thumbR, Y: r.Y + (r.H-thumbR*2)/2, W: thumbR * 2, H: thumbR * 2}, th.AccentText, thumbR)
		})
		if volSlider.Changed() {
			a.volume = volPercent / 100
			if a.muted && a.volume > 0 {
				a.muted = false
			}
			a.engine.SetVolume(a.volume, a.muted)
		}

		// ── Speed cycle button ─────────────────────────────────
		speedLabel := fmt.Sprintf("%.2fx", a.rate)
		if a.rate == 1.0 {
			speedLabel = "1.0x"
		}
		speedBtn := ui.Box(c).Center().Padding(4, 6, 4, 6).Radius(4).Cursor(ui.CursorPointer).
			Tooltip("Playback Speed (Click to cycle)")
		speedBg := ui.Transparent
		if speedBtn.Hovered() {
			speedBg = th.SurfaceHover
		}
		speedBtn.Background(speedBg)
		speedBtn.Children(func() {
			ui.Text(c, speedLabel).FontSize(11).Bold().TextColor(th.Text)
		})
		if speedBtn.Clicked() {
			a.cycleRate()
		}

		// ── Subtitles toggle button (if tracks exist) ──────────
		if len(a.subtitleTracks) > 0 {
			if a.iconButton(c, th, iconCaptions, "Subtitles", "Subtitles (C)", a.subtitlesOn).Clicked() {
				a.onSubtitles(!a.subtitlesOn)
			}
		}

		// ── Loop button (cycles off → all → one) ───────────────
		loopIco := iconRepeat
		if a.loopMode == LoopOne {
			loopIco = iconRepeatOne
		}
		if a.iconButton(c, th, loopIco, "Loop", "Loop Mode", a.loopMode != LoopOff).Clicked() {
			switch a.loopMode {
			case LoopOff:
				a.loopMode = LoopAll
			case LoopAll:
				a.loopMode = LoopOne
			default:
				a.loopMode = LoopOff
			}
			if a.menuLoopItem != nil {
				a.menuLoopItem.SetChecked(a.loopMode != LoopOff)
			}
		}

		// ── Picture in Picture (PiP) ───────────────────────────
		if a.iconButton(c, th, iconPiP, "PiP", "Picture-in-Picture", a.pip).Clicked() {
			a.onPiP(!a.pip)
		}

		// ── Side panel toggle ──────────────────────────────────
		if a.iconButton(c, th, iconPanel, "Side Panel", "Side Panel (Ctrl+P)", a.showPanel).Clicked() {
			a.showPanel = !a.showPanel
			if a.menuPanelItem != nil {
				a.menuPanelItem.SetChecked(a.showPanel)
			}
		}

		// ── Fullscreen toggle ──────────────────────────────────
		if a.iconButton(c, th, iconFullscreen, "Fullscreen", "Fullscreen (F / F11)", a.fullscreen).Clicked() {
			a.onFullscreen(!a.fullscreen)
		}
	})
}

// viewPanel: Right-side panel for Playlist, Subtitles, and Drop Zone
func (a *app) viewPanel(c *ui.Context, th *ui.Theme) {
	panelBg := ui.Hex("#111111")
	if !a.darkMode {
		panelBg = th.Surface
	}

	ui.Column(c).Width(320).FillHeight().
		Background(panelBg).
		Border(1, th.Border).Children(func() {

		// Panel tabs navigation bar
		ui.Row(c).FillWidth().Padding(6).Gap(4).Background(ui.Hex("#191919")).Children(func() {
			tabNames := []struct {
				name string
				icon *ui.SVG
			}{
				{"Playlist", iconList},
				{"Subtitles", iconCaptions},
				{"Drop Zone", iconUpload},
			}
			for i, t := range tabNames {
				idx := i
				active := a.panelTab == idx
				btn := ui.Row(c).Grow(1).Center().Padding(6, 4, 6, 4).Radius(4).
					Cursor(ui.CursorPointer).Gap(4)
				bg := ui.Transparent
				fg := th.TextMuted
				if active {
					bg = th.Surface
					fg = th.Text
				} else if btn.Hovered() {
					bg = th.SurfaceHover
				}
				btn.Background(bg)
				btn.Children(func() {
					ui.Icon(c, t.icon).Size(14, 14).TextColor(fg)
					ui.Text(c, t.name).FontSize(12).Bold().TextColor(fg)
				})
				if btn.Clicked() {
					a.panelTab = idx
				}
			}
		})

		// Tab content
		switch a.panelTab {
		case 0:
			a.viewPanelPlaylist(c, th)
		case 1:
			a.viewPanelSubtitles(c, th)
		case 2:
			a.viewPanelDrop(c, th)
		}
	})
}

// viewPanelPlaylist: Queue management
func (a *app) viewPanelPlaylist(c *ui.Context, th *ui.Theme) {
	ui.Column(c).Grow(1).Padding(10).Gap(8).Children(func() {
		// Header
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconList).Size(16, 16).TextColor(th.Accent)
			ui.Text(c, fmt.Sprintf("Playlist  ·  %d items", len(a.playlist))).
				Bold().FontSize(13).TextColor(th.Text)
		})

		if len(a.playlist) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Icon(c, iconList).Size(36, 36).TextColor(th.TextMuted)
				ui.Text(c, "Playlist is empty").
					TextColor(th.TextMuted).FontSize(13)
				ui.Text(c, "Open File > Open… or drop files here").
					TextColor(th.TextMuted).FontSize(11).TextAlign(ui.Center)
			})
			return
		}

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				for _, it := range a.playlist {
					item := it
					isCurrent := item.ID == a.currentID

					row := ui.Row(c).FillWidth().AlignItems(ui.Center).
						Padding(6, 8, 6, 8).Radius(6).Cursor(ui.CursorPointer).Gap(6)

					bg := ui.Transparent
					fg := th.Text
					if isCurrent {
						bg = th.Accent.Alpha(0.18)
						fg = th.Accent
					} else if row.Hovered() {
						bg = th.SurfaceHover
					}
					row.Background(bg)

					row.Children(func() {
						itemIcon := iconFilm
						if item.IsAudio {
							itemIcon = iconMusic
						}
						ui.Icon(c, itemIcon).Size(14, 14).TextColor(th.TextMuted)

						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, item.Title).Bold().FontSize(12).TextColor(fg).SingleLine()
							if item.Subtitle != "" {
								ui.Text(c, item.Subtitle).FontSize(10).TextColor(th.TextMuted).SingleLine()
							}
						})

						// Delete button
						del := ui.Box(c).Center().Padding(4).Radius(4).Cursor(ui.CursorPointer).
							Tooltip("Remove from playlist")
						if del.Hovered() {
							del.Background(ui.Hex("#441111"))
						}
						del.Children(func() {
							ui.Icon(c, iconTrash).Size(12, 12).TextColor(th.TextMuted)
						})
						if del.Clicked() {
							a.removeTrackByID(item.ID)
						}
					})

					if row.Clicked() {
						a.playTrackByID(item.ID)
					}
				}
			})
		})
	})
}

// viewPanelSubtitles: Responsive subtitle tracks, appearance styling, and cue list
func (a *app) viewPanelSubtitles(c *ui.Context, th *ui.Theme) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Grow(1).Padding(10).Gap(10).Children(func() {

			// ── 1. Header with Track Count & Add Button ──────────────────────
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Icon(c, iconCaptions).Size(16, 16).TextColor(th.Accent)
					nTracks := len(a.subtitleTracks)
					label := "Subtitles"
					if nTracks == 1 {
						label = "Subtitles (1 track)"
					} else if nTracks > 1 {
						label = fmt.Sprintf("Subtitles (%d tracks)", nTracks)
					}
					ui.Text(c, label).Bold().FontSize(13).TextColor(th.Text)
				})

				ui.Box(c).Grow(1)

				addBtn := ui.Button(c, "+ Add Subtitle…").Padding(3, 8, 3, 8).Radius(4)
				if addBtn.Clicked() {
					go func() {
						path, err := openNativeSubtitleDialog()
						if err != nil || path == "" {
							return
						}
						if a.win != nil {
							a.win.Update(func() { a.loadSubtitleFile(path) })
						}
					}()
				}
			})

			// ── 2. Track Selection List (Always shows ALL available tracks) ──
			ui.Column(c).FillWidth().Padding(8).Radius(8).Background(ui.Hex("#181818")).Gap(6).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Available Tracks").Bold().FontSize(11).TextColor(th.TextMuted)
					ui.Box(c).Grow(1)
					if a.subtitlesOn {
						ui.Text(c, "Active").FontSize(10).Bold().TextColor(th.Accent)
					} else {
						ui.Text(c, "Off").FontSize(10).TextColor(th.TextMuted)
					}
				})

				// "Off / Disabled" Row
				offRow := ui.Row(c).FillWidth().AlignItems(ui.Center).
					Padding(5, 8, 5, 8).Radius(5).Cursor(ui.CursorPointer).Gap(8)
				offActive := !a.subtitlesOn
				offBg := ui.Transparent
				offFg := th.TextMuted
				if offActive {
					offBg = th.Accent.Alpha(0.18)
					offFg = th.Accent
				} else if offRow.Hovered() {
					offBg = th.SurfaceHover
					offFg = th.Text
				}
				offRow.Background(offBg)
				offRow.Children(func() {
					bullet := "○"
					if offActive {
						bullet = "●"
					}
					ui.Text(c, bullet).FontSize(11).Bold().TextColor(offFg)
					ui.Text(c, "None (Subtitles Off)").FontSize(12).Bold().TextColor(offFg)
				})
				if offRow.Clicked() {
					a.onSubtitles(false)
				}

				// All Available Subtitle Tracks
				if len(a.subtitleTracks) == 0 {
					ui.Box(c).Padding(6, 4, 6, 4).Children(func() {
						ui.Text(c, "No tracks loaded. Click '+ Add Subtitle…' to load .srt or .vtt file.").
							FontSize(11).TextColor(th.TextMuted)
					})
				} else {
					for i, tr := range a.subtitleTracks {
						idx := i
						track := tr
						isActive := a.subtitlesOn && a.activeTrackIdx == idx

						trackRow := ui.Row(c).FillWidth().AlignItems(ui.Center).
							Padding(5, 8, 5, 8).Radius(5).Cursor(ui.CursorPointer).Gap(8)
						tBg := ui.Transparent
						tFg := th.Text
						if isActive {
							tBg = th.Accent.Alpha(0.20)
							tFg = th.Accent
						} else if trackRow.Hovered() {
							tBg = th.SurfaceHover
						}
						trackRow.Background(tBg)

						trackRow.Children(func() {
							bullet := "○"
							if isActive {
								bullet = "●"
							}
							ui.Text(c, bullet).FontSize(11).Bold().TextColor(tFg)

							ui.Column(c).Grow(1).Gap(1).Children(func() {
								ui.Text(c, track.Name).Bold().FontSize(12).TextColor(tFg).SingleLine()
								cueCountText := fmt.Sprintf("%d cues", len(track.Cues))
								ui.Text(c, cueCountText).FontSize(10).TextColor(th.TextMuted)
							})
						})

						if trackRow.Clicked() {
							a.activeTrackIdx = idx
							a.onSubtitles(true)
							a.statusMsg = "Subtitles: " + track.Name
						}
					}
				}
			})

			// ── 3. Responsive Subtitle Style & Position Controls ─────────────
			ui.Column(c).FillWidth().Padding(10).Radius(8).Background(ui.Hex("#181818")).Gap(10).Children(func() {
				// Section Title & Reset Action
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Subtitle Style & Position").Bold().FontSize(12).TextColor(th.Text)
					ui.Box(c).Grow(1)
					resetBtn := ui.Box(c).Padding(2, 6, 2, 6).Radius(4).Cursor(ui.CursorPointer)
					if resetBtn.Hovered() {
						resetBtn.Background(th.SurfaceHover)
					}
					resetBtn.Children(func() {
						ui.Text(c, "Reset Default").FontSize(10).TextColor(th.Accent)
					})
					if resetBtn.Clicked() {
						a.subFontSize = 18
						a.subBottomOffset = 28
					}
				})

				// Font Size Section
				ui.Column(c).FillWidth().Gap(5).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Font Size").FontSize(11).TextColor(th.TextMuted)
						ui.Box(c).Grow(1)
						ui.Box(c).Padding(1, 6, 1, 6).Radius(3).Background(th.Accent.Alpha(0.2)).Children(func() {
							ui.Text(c, fmt.Sprintf("%.0f pt", a.subFontSize)).Bold().FontSize(11).TextColor(th.Accent)
						})
					})

					// Size Step Buttons
					ui.Row(c).FillWidth().Gap(6).Children(func() {
						decBtn := ui.Button(c, "A- Smaller").Grow(1).Padding(4, 0, 4, 0).Radius(4)
						if decBtn.Clicked() && a.subFontSize > 12 {
							a.subFontSize -= 2
						}
						incBtn := ui.Button(c, "A+ Larger").Grow(1).Padding(4, 0, 4, 0).Radius(4)
						if incBtn.Clicked() && a.subFontSize < 40 {
							a.subFontSize += 2
						}
					})

					// Size Presets Row
					ui.Row(c).FillWidth().Gap(4).Children(func() {
						presets := []struct {
							label string
							val   float32
						}{
							{"14 Sm", 14},
							{"18 Med", 18},
							{"24 Lg", 24},
							{"30 XL", 30},
						}
						for _, p := range presets {
							preset := p
							chip := ui.Box(c).Grow(1).Center().Padding(3, 2, 3, 2).Radius(4).Cursor(ui.CursorPointer)
							isSel := a.subFontSize == preset.val
							bg := ui.Hex("#222222")
							fg := th.TextMuted
							if isSel {
								bg = th.Accent
								fg = th.AccentText
							} else if chip.Hovered() {
								bg = th.SurfaceHover
								fg = th.Text
							}
							chip.Background(bg)
							chip.Children(func() {
								ui.Text(c, preset.label).FontSize(10).Bold().TextColor(fg)
							})
							if chip.Clicked() {
								a.subFontSize = preset.val
							}
						}
					})
				})

				ui.Box(c).FillWidth().Height(1).Background(ui.Hex("#252525"))

				// Vertical Position Section
				ui.Column(c).FillWidth().Gap(5).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "Vertical Position").FontSize(11).TextColor(th.TextMuted)
						ui.Box(c).Grow(1)
						ui.Box(c).Padding(1, 6, 1, 6).Radius(3).Background(th.Accent.Alpha(0.2)).Children(func() {
							ui.Text(c, fmt.Sprintf("%.0f px from bottom", a.subBottomOffset)).Bold().FontSize(11).TextColor(th.Accent)
						})
					})

					// Position Step Buttons
					ui.Row(c).FillWidth().Gap(6).Children(func() {
						downBtn := ui.Button(c, "▼ Move Down").Grow(1).Padding(4, 0, 4, 0).Radius(4)
						if downBtn.Clicked() && a.subBottomOffset > 8 {
							a.subBottomOffset -= 12
						}
						upBtn := ui.Button(c, "▲ Move Up").Grow(1).Padding(4, 0, 4, 0).Radius(4)
						if upBtn.Clicked() && a.subBottomOffset < 350 {
							a.subBottomOffset += 12
						}
					})

					// Position Presets Row
					ui.Row(c).FillWidth().Gap(4).Children(func() {
						presets := []struct {
							label  string
							offset float32
						}{
							{"Bottom (28px)", 28},
							{"Lower-Mid (80px)", 80},
							{"Center (160px)", 160},
						}
						for _, p := range presets {
							preset := p
							chip := ui.Box(c).Grow(1).Center().Padding(3, 2, 3, 2).Radius(4).Cursor(ui.CursorPointer)
							isSel := math.Abs(float64(a.subBottomOffset-preset.offset)) < 8
							bg := ui.Hex("#222222")
							fg := th.TextMuted
							if isSel {
								bg = th.Accent
								fg = th.AccentText
							} else if chip.Hovered() {
								bg = th.SurfaceHover
								fg = th.Text
							}
							chip.Background(bg)
							chip.Children(func() {
								ui.Text(c, preset.label).FontSize(10).Bold().TextColor(fg)
							})
							if chip.Clicked() {
								a.subBottomOffset = preset.offset
							}
						}
					})
				})
			})

			// ── 4. Interactive Cues List (Click to seek) ──────────────────────
			cues := a.activeCues()
			if len(cues) > 0 {
				ui.Column(c).FillWidth().Padding(8).Radius(8).Background(ui.Hex("#181818")).Gap(6).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("Subtitle Cues (%d)", len(cues))).Bold().FontSize(11).TextColor(th.TextMuted)
						ui.Box(c).Grow(1)
						ui.Text(c, "Click cue to jump").FontSize(10).TextColor(th.TextMuted)
					})

					// Show list of cues
					ui.Column(c).FillWidth().Gap(3).Children(func() {
						for _, cue := range cues {
							cueCopy := cue
							isCurrent := a.position >= cue.Start && a.position <= cue.End
							row := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).
								Padding(4, 6, 4, 6).Radius(4).Cursor(ui.CursorPointer)
							if isCurrent {
								row.Background(th.Accent.Alpha(0.18))
							} else if row.Hovered() {
								row.Background(th.SurfaceHover)
							}

							row.Children(func() {
								ts := formatClock(cueCopy.Start)
								tsColor := th.Accent
								if !isCurrent {
									tsColor = th.TextMuted
								}
								ui.Text(c, ts).FontSize(11).Bold().TextColor(tsColor)
								ui.Text(c, cueCopy.Text).FontSize(12).TextColor(th.Text).SingleLine()
							})

							if row.Clicked() {
								a.onSeek(cueCopy.Start)
							}
						}
					})
				})
			}
		})
	})
}

// viewPanelDrop: Drag-and-drop landing area & Now Playing technical metadata
func (a *app) viewPanelDrop(c *ui.Context, th *ui.Theme) {
	ui.Column(c).Grow(1).Padding(10).Gap(10).Children(func() {
		// Header
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconUpload).Size(16, 16).TextColor(th.Accent)
			ui.Text(c, "Drop Zone").Bold().FontSize(13).TextColor(th.Text)
		})

		// Drop zone box
		dropBox := ui.Column(c).FillWidth().Height(120).Center().
			Radius(8).Gap(8).Border(1, th.Border)
		bg := th.Surface
		if dropBox.FileDragOver() {
			bg = th.Accent.Alpha(0.2)
		}
		dropBox.Background(bg)
		dropBox.Children(func() {
			ui.Icon(c, iconUpload).Size(28, 28).TextColor(th.Accent)
			ui.Text(c, "Drop audio or video files here").FontSize(12).Bold().TextColor(th.Text)
			browseBtn := ui.Button(c, "Browse Files…").Padding(4, 14, 4, 14).Radius(4)
			if browseBtn.Clicked() {
				go func() {
					path, err := openMediaFileDialog()
					if err != nil || path == "" {
						return
					}
					if a.win != nil {
						a.win.Update(func() { a.loadFile(path, true) })
					}
				}()
			}
			ui.Text(c, "MP4, MKV, WebM, AVI, MOV, WMV, MP3, WAV, FLAC, AAC, M4A, OGG").FontSize(10).TextColor(th.TextMuted)
		})
		if dropped := dropBox.DroppedFiles(); len(dropped) > 0 {
			for i, f := range dropped {
				a.loadFile(f, i == 0)
			}
		}

		// Now Playing technical specifications
		if a.info != nil {
			ui.Box(c).FillWidth().Height(1).Background(th.Border)

			mediaIco := iconFilm
			if a.info.IsAudio {
				mediaIco = iconMusic
			}
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Icon(c, mediaIco).Size(14, 14).TextColor(th.Accent)
				ui.Text(c, "Now Playing").Bold().FontSize(12).TextColor(th.Text)
			})

			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, a.info.Title).Bold().FontSize(13).TextColor(th.Text)
				if a.info.IsAudio {
					ui.Text(c, fmt.Sprintf("Format: %s Audio", a.info.AudioCodec)).
						FontSize(11).TextColor(th.TextMuted)
				} else {
					ui.Text(c, fmt.Sprintf("%dx%d  ·  %.0f fps", a.info.Width, a.info.Height, a.info.FPS)).
						FontSize(11).TextColor(th.TextMuted)
					ui.Text(c, fmt.Sprintf("Codecs: %s / %s", a.info.VideoCodec, a.info.AudioCodec)).
						FontSize(11).TextColor(th.TextMuted)
				}
				if a.info.FileSize > 0 {
					ui.Text(c, fmt.Sprintf("%.1f MB  ·  %s",
						float64(a.info.FileSize)/(1024*1024),
						formatClock(a.info.Duration))).
						FontSize(11).TextColor(th.TextMuted)
				}
			})
		}
	})
}

// ── Application Entry Point ───────────────────────────────────────────────────

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "Media Player",
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
		win.SetMenu(mygo.NewMenu(a.buildMenu()))

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
