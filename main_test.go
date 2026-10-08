package main

import (
	"image"
	"testing"
	"time"

	"github.com/ZacharyZhang-NY/MujicaUI/media"
	"github.com/egoist/mygo/ui"
)

func TestVideoPlayerView(t *testing.T) {
	a := newApp()
	a.duration = 10 * time.Second
	a.position = 2 * time.Second
	a.showDrawer = true
	a.info = &VideoInfo{
		Title:      "demo.mp4",
		Path:       "demo.mp4",
		Duration:   10 * time.Second,
		Width:      640,
		Height:     360,
		FPS:        25,
		VideoCodec: "H.264",
		AudioCodec: "AAC",
	}
	a.playlist = []media.PlaylistItem{
		{
			ID:       "demo.mp4",
			Title:    "demo.mp4",
			Subtitle: "640x360 • 0:10",
			Duration: 10 * time.Second,
		},
	}
	a.currentID = "demo.mp4"

	// Create test frame
	testImg := GenerateTestFrame("demo.mp4", 2*time.Second, 10*time.Second, 640, 360)
	a.frame = ui.NewBitmap(testImg)

	tt := ui.NewTester(a.view, 960, 740)

	// Header brand text (compact: "MyGo Player")
	if !tt.HasText("MyGo Player") {
		t.Errorf("expected header text 'MyGo Player', got texts: %v", tt.Texts())
	}

	// Open button is icon-only; its label/tooltip is "Open File (Ctrl+O)"
	if !tt.HasText("Open File (Ctrl+O)") {
		t.Errorf("expected 'Open File (Ctrl+O)' icon button, got texts: %v", tt.Texts())
	}

	// Drawer tabs
	if !tt.HasText("Video Details") {
		t.Errorf("expected 'Video Details' tab, got texts: %v", tt.Texts())
	}
	if !tt.HasText("Drop Zone") {
		t.Errorf("expected 'Drop Zone' tab, got texts: %v", tt.Texts())
	}

	// Playback control present
	if !tt.HasText("Play") {
		t.Errorf("expected 'Play' button in controls, got texts: %v", tt.Texts())
	}
}

func TestPlaybackControlsState(t *testing.T) {
	a := newApp()
	a.duration = 30 * time.Second

	// Play
	a.onPlay()
	if !a.playing {
		t.Errorf("expected playing == true after onPlay()")
	}

	// Pause
	a.onPause()
	if a.playing {
		t.Errorf("expected playing == false after onPause()")
	}

	// Seek
	a.onSeek(15 * time.Second)
	if a.position != 15*time.Second {
		t.Errorf("expected position == 15s, got %v", a.position)
	}

	// Rate
	a.onRate(1.5)
	if a.rate != 1.5 {
		t.Errorf("expected rate == 1.5, got %v", a.rate)
	}

	// Subtitles
	a.onSubtitles(false)
	if a.subtitlesOn {
		t.Errorf("expected subtitlesOn == false")
	}
	a.onSubtitles(true)
	if !a.subtitlesOn {
		t.Errorf("expected subtitlesOn == true")
	}

	// Fullscreen & PiP
	a.onFullscreen(true)
	if !a.fullscreen {
		t.Errorf("expected fullscreen == true")
	}
	a.onPiP(true)
	if !a.pip {
		t.Errorf("expected pip == true")
	}
}

func TestPlaylistManagement(t *testing.T) {
	a := newApp()
	a.playlist = []media.PlaylistItem{
		{ID: "video1.mp4", Title: "Video 1", Duration: 10 * time.Second},
		{ID: "video2.mp4", Title: "Video 2", Duration: 20 * time.Second},
	}
	a.currentID = "video1.mp4"

	if len(a.playlist) != 2 {
		t.Fatalf("expected 2 playlist items, got %d", len(a.playlist))
	}

	// Next track
	a.nextTrack()
	time.Sleep(50 * time.Millisecond)

	// Remove item
	a.removeTrackByID("video1.mp4")
	if len(a.playlist) != 1 {
		t.Errorf("expected 1 playlist item after removal, got %d", len(a.playlist))
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
		t.Errorf("expected empty cue at 7s, got %q", cue)
	}
}

func TestSubtitleTrackBuilding(t *testing.T) {
	// buildSubtitleTracks with empty path returns empty slice (no demo cues)
	tracks := buildSubtitleTracks("")
	if len(tracks) != 0 {
		t.Errorf("expected no tracks for empty path, got %d", len(tracks))
	}

	// Non-existent video returns empty (no side-car, no embedded streams)
	tracks = buildSubtitleTracks("/non/existent/video.mp4")
	// No panic, just empty
	_ = tracks
}

func TestGenerateTestFrame(t *testing.T) {
	img := GenerateTestFrame("Sample", 5*time.Second, 10*time.Second, 640, 360)
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 640 || bounds.Dy() != 360 {
		t.Errorf("expected 640x360 dimensions, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	// Assert image is valid RGBA
	rgba, ok := img.(*image.RGBA)
	if !ok || len(rgba.Pix) != 640*360*4 {
		t.Fatalf("expected valid RGBA image with 640*360*4 bytes")
	}
}
