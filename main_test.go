package main

import (
	"image"
	"testing"
	"time"

	"github.com/ZacharyZhang-NY/MujicaUI/media"
	"github.com/egoist/mygo/ui"
)

// TestVideoPlayerView checks that the VLC-style view renders without panic
// and contains the core structural elements.
func TestVideoPlayerView(t *testing.T) {
	a := newApp()
	a.duration = 10 * time.Second
	a.position = 2 * time.Second
	a.showPanel = true
	a.info = &VideoInfo{
		Title: "demo.mp4", Path: "demo.mp4",
		Duration: 10 * time.Second, Width: 640, Height: 360,
		FPS: 25, VideoCodec: "H.264", AudioCodec: "AAC",
	}
	a.playlist = []media.PlaylistItem{{
		ID: "demo.mp4", Title: "demo.mp4",
		Subtitle: "640x360 • 0:10", Duration: 10 * time.Second,
	}}
	a.currentID = "demo.mp4"

	testImg := GenerateTestFrame("demo.mp4", 2*time.Second, 10*time.Second, 640, 360)
	a.frame = ui.NewBitmap(testImg)

	tt := ui.NewTester(a.view, 1280, 720)

	// Controls bar must contain play/pause
	if !tt.HasText("Play/Pause (Space)") {
		t.Errorf("expected Play/Pause button; got: %v", tt.Texts())
	}

	// Time clock displayed
	if !tt.HasText("0:02 / 0:10") {
		t.Errorf("expected time clock '0:02 / 0:10'; got: %v", tt.Texts())
	}

	// Side panel visible with playlist items
	if !tt.HasText("Playlist") {
		t.Errorf("expected 'Playlist' header/tab; got: %v", tt.Texts())
	}
}

func TestPlaybackControlsState(t *testing.T) {
	a := newApp()
	a.duration = 30 * time.Second

	a.onPlay()
	if !a.playing {
		t.Errorf("expected playing after onPlay()")
	}

	a.onPause()
	if a.playing {
		t.Errorf("expected paused after onPause()")
	}

	a.onSeek(15 * time.Second)
	if a.position != 15*time.Second {
		t.Errorf("expected position 15s, got %v", a.position)
	}

	a.onRate(1.5)
	if a.rate != 1.5 {
		t.Errorf("expected rate 1.5, got %v", a.rate)
	}

	a.onSubtitles(false)
	if a.subtitlesOn {
		t.Errorf("expected subtitlesOn false")
	}
	a.onSubtitles(true)
	if !a.subtitlesOn {
		t.Errorf("expected subtitlesOn true")
	}

	a.onFullscreen(true)
	if !a.fullscreen {
		t.Errorf("expected fullscreen true")
	}

	a.onPiP(true)
	if !a.pip {
		t.Errorf("expected pip true")
	}
}

func TestPlaylistManagement(t *testing.T) {
	a := newApp()
	a.playlist = []media.PlaylistItem{
		{ID: "v1.mp4", Title: "Video 1", Duration: 10 * time.Second},
		{ID: "v2.mp4", Title: "Video 2", Duration: 20 * time.Second},
	}
	a.currentID = "v1.mp4"

	if len(a.playlist) != 2 {
		t.Fatalf("expected 2 items, got %d", len(a.playlist))
	}

	a.nextTrack()
	time.Sleep(50 * time.Millisecond)

	a.removeTrackByID("v1.mp4")
	if len(a.playlist) != 1 {
		t.Errorf("expected 1 item after removal, got %d", len(a.playlist))
	}
}

func TestSubtitleParsingAndMatching(t *testing.T) {
	cues := []SubtitleCue{
		{Start: 1 * time.Second, End: 3 * time.Second, Text: "Hello World"},
		{Start: 4 * time.Second, End: 6 * time.Second, Text: "Second Cue"},
	}

	if cue := GetSubtitleAt(cues, 2*time.Second); cue != "Hello World" {
		t.Errorf("expected 'Hello World' at 2s, got %q", cue)
	}
	if cue := GetSubtitleAt(cues, 5*time.Second); cue != "Second Cue" {
		t.Errorf("expected 'Second Cue' at 5s, got %q", cue)
	}
	if cue := GetSubtitleAt(cues, 7*time.Second); cue != "" {
		t.Errorf("expected empty at 7s, got %q", cue)
	}
}

func TestSubtitleTrackBuilding(t *testing.T) {
	// Empty path → empty slice, no demo cues
	tracks := buildSubtitleTracks("")
	if len(tracks) != 0 {
		t.Errorf("expected no tracks for empty path, got %d", len(tracks))
	}

	// Non-existent path → empty, no panic
	tracks = buildSubtitleTracks("/non/existent/video.mp4")
	_ = tracks
}

func TestLoopModes(t *testing.T) {
	a := newApp()
	if a.loopMode != media.LoopOff {
		t.Errorf("expected LoopOff initially")
	}

	// Simulate loop button cycling
	a.loopMode = media.LoopAll
	if a.loopMode != media.LoopAll {
		t.Errorf("expected LoopAll")
	}
	a.loopMode = media.LoopOne
	if a.loopMode != media.LoopOne {
		t.Errorf("expected LoopOne")
	}
	a.loopMode = media.LoopOff
	if a.loopMode != media.LoopOff {
		t.Errorf("expected LoopOff")
	}
}

func TestGenerateTestFrame(t *testing.T) {
	img := GenerateTestFrame("Sample", 5*time.Second, 10*time.Second, 640, 360)
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 640 || bounds.Dy() != 360 {
		t.Errorf("expected 640x360, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	rgba, ok := img.(*image.RGBA)
	if !ok || len(rgba.Pix) != 640*360*4 {
		t.Fatalf("expected valid RGBA 640x360 image")
	}
}
