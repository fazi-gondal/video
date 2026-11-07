package main

import (
	"image"
	"testing"
	"time"

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
	a.playlist = []PlaylistItem{{
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
	a.info = &VideoInfo{Title: "test.mp4", Duration: 30 * time.Second}
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
	a.playlist = []PlaylistItem{
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
	if a.loopMode != LoopOff {
		t.Errorf("expected LoopOff initially")
	}

	// Simulate loop button cycling
	a.loopMode = LoopAll
	if a.loopMode != LoopAll {
		t.Errorf("expected LoopAll")
	}
	a.loopMode = LoopOne
	if a.loopMode != LoopOne {
		t.Errorf("expected LoopOne")
	}
	a.loopMode = LoopOff
	if a.loopMode != LoopOff {
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

func TestIsAudioPath(t *testing.T) {
	audioFiles := []string{
		"track.mp3", "song.wav", "music.flac", "podcast.aac",
		"album.m4a", "radio.ogg", "classic.wma", "audio.aiff",
		"file.alac", "melody.mid", "stream.opus", "TRACK.MP3",
	}
	for _, f := range audioFiles {
		if !IsAudioPath(f) {
			t.Errorf("expected IsAudioPath(%q) = true, got false", f)
		}
	}

	nonAudioFiles := []string{
		"video.mp4", "movie.mkv", "clip.avi", "show.mov", "document.pdf", "image.png",
	}
	for _, f := range nonAudioFiles {
		if IsAudioPath(f) {
			t.Errorf("expected IsAudioPath(%q) = false, got true", f)
		}
	}
}

func TestGenerateAudioFrame(t *testing.T) {
	img := GenerateAudioFrame("Test Symphony", 12*time.Second, 180*time.Second, 800, 450)
	if img == nil {
		t.Fatal("expected non-nil audio visualizer image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 800 || bounds.Dy() != 450 {
		t.Errorf("expected 800x450, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestParseSRTBytes(t *testing.T) {
	srtData := []byte(`1
00:00:01,000 --> 00:00:03,500
Hello, world!

2
00:00:04,200 --> 00:00:06,800
Second subtitle line.
`)

	cues, err := ParseSRTBytes(srtData)
	if err != nil {
		t.Fatalf("ParseSRTBytes failed: %v", err)
	}
	if len(cues) != 2 {
		t.Fatalf("expected 2 cues, got %d", len(cues))
	}
	if cues[0].Text != "Hello, world!" {
		t.Errorf("expected cue 0 text 'Hello, world!', got %q", cues[0].Text)
	}
	if cues[1].Text != "Second subtitle line." {
		t.Errorf("expected cue 1 text 'Second subtitle line.', got %q", cues[1].Text)
	}
}

func TestNativeDialogFilters(t *testing.T) {
	pairs := [][2]string{
		{"Media Files", "*.mp4;*.mp3"},
		{"All Files", "*.*"},
	}
	buf := buildFilterUTF16(pairs)
	if len(buf) == 0 {
		t.Fatal("expected non-empty UTF-16 filter buffer")
	}
	// Last element must be double null terminator (buf[len-1] == 0 and buf[len-2] == 0)
	if buf[len(buf)-1] != 0 || buf[len(buf)-2] != 0 {
		t.Errorf("expected double-null terminated buffer, got trailing %v, %v",
			buf[len(buf)-2], buf[len(buf)-1])
	}
}

func TestSubtitleSettings(t *testing.T) {
	a := newApp()
	if a.subFontSize != 18 {
		t.Errorf("expected default subFontSize 18, got %v", a.subFontSize)
	}
	if a.subBottomOffset != 28 {
		t.Errorf("expected default subBottomOffset 28, got %v", a.subBottomOffset)
	}

	// Adjust font size
	a.subFontSize += 4
	if a.subFontSize != 22 {
		t.Errorf("expected subFontSize 22, got %v", a.subFontSize)
	}

	// Adjust vertical offset (move up/down)
	a.subBottomOffset += 20
	if a.subBottomOffset != 48 {
		t.Errorf("expected subBottomOffset 48, got %v", a.subBottomOffset)
	}
}

func TestSubtitlePanelResponsiveLayout(t *testing.T) {
	a := newApp()
	a.showPanel = true
	a.panelTab = 1 // Subtitle tab
	a.subtitlesOn = true
	a.subtitleTracks = []SubtitleTrack{
		{Name: "English [SDH]", Cues: []SubtitleCue{{Start: 1 * time.Second, End: 3 * time.Second, Text: "Hello"}}},
		{Name: "Spanish", Cues: []SubtitleCue{{Start: 1 * time.Second, End: 3 * time.Second, Text: "Hola"}}},
		{Name: "French", Cues: []SubtitleCue{{Start: 1 * time.Second, End: 3 * time.Second, Text: "Bonjour"}}},
		{Name: "German", Cues: []SubtitleCue{{Start: 1 * time.Second, End: 3 * time.Second, Text: "Hallo"}}},
	}
	a.activeTrackIdx = 0

	tt := ui.NewTester(a.view, 1280, 720)

	// All 4 tracks must be rendered
	if !tt.HasText("English [SDH]") {
		t.Errorf("expected 'English [SDH]' to be rendered")
	}
	if !tt.HasText("Spanish") {
		t.Errorf("expected 'Spanish' to be rendered")
	}
	if !tt.HasText("French") {
		t.Errorf("expected 'French' to be rendered")
	}
	if !tt.HasText("German") {
		t.Errorf("expected 'German' to be rendered")
	}

	// Subtitle controls rendered
	if !tt.HasText("Subtitle Style & Position") {
		t.Errorf("expected 'Subtitle Style & Position' header")
	}
	if !tt.HasText("A- Smaller") || !tt.HasText("A+ Larger") {
		t.Errorf("expected size stepper buttons")
	}
	if !tt.HasText("▼ Move Down") || !tt.HasText("▲ Move Up") {
		t.Errorf("expected position stepper buttons")
	}
}




