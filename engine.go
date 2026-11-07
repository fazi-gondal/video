package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// defaultWidth / defaultHeight are fallback dimensions for UI canvases.
const (
	defaultWidth  = 1280
	defaultHeight = 720
)

// VideoInfo holds metadata about a media file (audio or video).
type VideoInfo struct {
	Path       string
	Title      string
	Duration   time.Duration
	Width      int
	Height     int
	FPS        float64
	VideoCodec string
	AudioCodec string
	FileSize   int64
	IsAudio    bool
}

// ffprobeJSON is the output shape of ffprobe.
type ffprobeJSON struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
	} `json:"format"`
}

// IsAudioPath checks if a file extension represents an audio format.
func IsAudioPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".wav", ".flac", ".aac", ".m4a", ".ogg", ".wma", ".alac", ".aiff", ".aif", ".mid", ".midi", ".opus":
		return true
	default:
		return false
	}
}

// ProbeVideo inspects a media file (audio or video), working seamlessly with or without ffmpeg.
func ProbeVideo(path string) (*VideoInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(path))
	isAudio := IsAudioPath(path)

	info := &VideoInfo{
		Path:     path,
		Title:    filepath.Base(path),
		FileSize: st.Size(),
		Width:    defaultWidth,
		Height:   defaultHeight,
		FPS:      25,
		Duration: 10 * time.Second,
		IsAudio:  isAudio,
	}

	// 1. Check if ffprobe is installed in PATH
	hasFFprobe := false
	if _, err := exec.LookPath("ffprobe"); err == nil {
		hasFFprobe = true
	}

	if hasFFprobe {
		cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if out, err := cmd.Output(); err == nil {
			var probe ffprobeJSON
			if err := json.Unmarshal(out, &probe); err == nil {
				if probe.Format.Duration != "" {
					if dSec, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && dSec > 0 {
						info.Duration = time.Duration(dSec * float64(time.Second))
					}
				}
				for _, s := range probe.Streams {
					switch s.CodecType {
					case "video":
						info.VideoCodec = strings.ToUpper(s.CodecName)
						if s.Width > 0 && s.Height > 0 {
							info.Width = s.Width
							info.Height = s.Height
						}
						if s.AvgFrameRate != "" {
							parts := strings.Split(s.AvgFrameRate, "/")
							if len(parts) == 2 {
								num, _ := strconv.ParseFloat(parts[0], 64)
								den, _ := strconv.ParseFloat(parts[1], 64)
								if den > 0 && num > 0 {
									info.FPS = num / den
								}
							}
						}
					case "audio":
						info.AudioCodec = strings.ToUpper(s.CodecName)
					}
				}
			}
		}
	}

	// 2. Format / Codec inference without ffprobe
	if info.VideoCodec == "" && !isAudio {
		switch ext {
		case ".mp4":
			info.VideoCodec = "H.264 / AVC"
		case ".mkv":
			info.VideoCodec = "Matroska / H.264"
		case ".webm":
			info.VideoCodec = "VP9 / WebM"
		case ".avi":
			info.VideoCodec = "AVI / MPEG-4"
		case ".mov":
			info.VideoCodec = "QuickTime MOV"
		case ".wmv":
			info.VideoCodec = "WMV"
		case ".flv":
			info.VideoCodec = "FLV"
		default:
			info.VideoCodec = "Video"
		}
	}

	if info.AudioCodec == "" {
		switch ext {
		case ".mp3":
			info.AudioCodec = "MP3"
		case ".wav":
			info.AudioCodec = "WAV / PCM"
		case ".flac":
			info.AudioCodec = "FLAC Lossless"
		case ".aac":
			info.AudioCodec = "AAC"
		case ".m4a":
			info.AudioCodec = "M4A / ALAC"
		case ".ogg":
			info.AudioCodec = "Ogg Vorbis"
		case ".wma":
			info.AudioCodec = "WMA"
		case ".aiff", ".aif":
			info.AudioCodec = "AIFF"
		case ".mid", ".midi":
			info.AudioCodec = "MIDI"
		case ".opus":
			info.AudioCodec = "Opus"
		default:
			if isAudio {
				info.AudioCodec = strings.ToUpper(strings.TrimPrefix(ext, "."))
			} else {
				info.AudioCodec = "AAC"
			}
		}
	}

	// 3. Native Windows duration probing (without ffmpeg)
	if !hasFFprobe || info.Duration == 10*time.Second {
		absPath, _ := filepath.Abs(path)
		psCmd := fmt.Sprintf(`$w = New-Object -ComObject WMPlayer.OCX; $m = $w.newMedia('%s'); Write-Output $m.duration`, strings.ReplaceAll(absPath, `'`, `''`))
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if out, err := cmd.Output(); err == nil {
			durStr := strings.TrimSpace(string(out))
			if durSec, err := strconv.ParseFloat(durStr, 64); err == nil && durSec > 0 {
				info.Duration = time.Duration(durSec * float64(time.Second))
			}
		}
	}

	return info, nil
}

// ExtractSingleFrame extracts one frame at the specified timestamp.
func ExtractSingleFrame(path string, at time.Duration) (image.Image, error) {
	return ExtractSingleFrameAt(path, at, 0, 0)
}

// ExtractSingleFrameAt extracts a frame; if w/h are 0 the native resolution is kept.
func ExtractSingleFrameAt(path string, at time.Duration, w, h int) (image.Image, error) {
	fw, fh := w, h
	if fw <= 0 || fh <= 0 {
		if info, err := ProbeVideo(path); err == nil && info.Width > 0 && info.Height > 0 {
			fw, fh = info.Width, info.Height
		} else {
			fw, fh = defaultWidth, defaultHeight
		}
	}

	if IsAudioPath(path) {
		return GenerateAudioFrame(filepath.Base(path), at, 10*time.Second, fw, fh), nil
	}

	// If ffmpeg is available, use it for frame extraction with hardware acceleration
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		secStr := fmt.Sprintf("%.3f", at.Seconds())
		cmd := exec.Command("ffmpeg",
			"-hwaccel", "auto",
			"-threads", "0",
			"-ss", secStr,
			"-i", path,
			"-vframes", "1",
			"-s", fmt.Sprintf("%dx%d", fw, fh),
			"-f", "rawvideo",
			"-pix_fmt", "rgba",
			"-",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}

		stdout, err := cmd.StdoutPipe()
		if err == nil && cmd.Start() == nil {
			bufSize := fw * fh * 4
			buf := make([]byte, bufSize)
			if _, readErr := io.ReadFull(stdout, buf); readErr == nil {
				_ = cmd.Wait()
				return &image.RGBA{
					Pix:    buf,
					Stride: fw * 4,
					Rect:   image.Rect(0, 0, fw, fh),
				}, nil
			}
			_ = cmd.Wait()
		}
	}

	// Standalone fallback: clean synthetic frame
	return GenerateTestFrame(filepath.Base(path), at, 10*time.Second, fw, fh), nil
}

// PlaybackEngine manages video and audio playback pipelines.
type PlaybackEngine struct {
	mu         sync.Mutex
	filePath   string
	duration   time.Duration
	position   time.Duration
	playing    bool
	rate       float64
	volume     float64
	muted      bool
	vidWidth   int
	vidHeight  int
	isAudio    bool
	seekSeq    uint64
	cancelFunc context.CancelFunc
	onFrame    func(img image.Image, pos time.Duration)
	onEnd      func()
}

// NewPlaybackEngine initializes a new engine.
func NewPlaybackEngine(onFrame func(img image.Image, pos time.Duration), onEnd func()) *PlaybackEngine {
	return &PlaybackEngine{
		rate:      1.0,
		volume:    0.8,
		vidWidth:  defaultWidth,
		vidHeight: defaultHeight,
		onFrame:   onFrame,
		onEnd:     onEnd,
	}
}

// Load sets the active media file, duration, and media type.
func (e *PlaybackEngine) Load(path string, duration time.Duration, isAudio bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
	e.filePath = path
	e.duration = duration
	e.position = 0
	e.isAudio = isAudio
}

// SetResolution updates the decode resolution to the video's native size.
func (e *PlaybackEngine) SetResolution(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vidWidth = w
	e.vidHeight = h
}

// Play begins or resumes playback.
func (e *PlaybackEngine) Play() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.filePath == "" {
		return
	}
	if e.playing {
		return
	}
	if e.duration > 0 && e.position >= e.duration {
		e.position = 0
	}
	e.playing = true
	e.startStreamLocked()
}

// Pause stops active playback pipelines.
func (e *PlaybackEngine) Pause() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.playing {
		return
	}
	e.playing = false
	e.stopLocked()
}

// Seek seeks to a given timestamp with instant response.
func (e *PlaybackEngine) Seek(pos time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pos < 0 {
		pos = 0
	}
	if e.duration > 0 && pos > e.duration {
		pos = e.duration
	}
	e.position = pos
	wasPlaying := e.playing
	e.stopLocked()

	// If playing, immediately start streaming from the new position with zero delay
	if wasPlaying {
		e.playing = true
		e.startStreamLocked()
		return
	}

	// When paused, extract a single frame for preview at the seeked position
	path := e.filePath
	vw := e.vidWidth
	vh := e.vidHeight
	e.seekSeq++
	seq := e.seekSeq
	go func(targetPos time.Duration, p string, w, h int, s uint64) {
		img, _ := ExtractSingleFrameAt(p, targetPos, w, h)
		e.mu.Lock()
		defer e.mu.Unlock()
		if s == e.seekSeq && e.filePath == p && !e.playing {
			if e.onFrame != nil {
				e.onFrame(img, targetPos)
			}
		}
	}(pos, path, vw, vh, seq)
}

// SetRate changes the playback speed.
func (e *PlaybackEngine) SetRate(r float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r <= 0 {
		r = 1.0
	}
	e.rate = r
	if e.playing {
		e.stopLocked()
		e.startStreamLocked()
	}
}

// SetVolume updates volume and mute status.
func (e *PlaybackEngine) SetVolume(v float64, muted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.volume = max(0, min(1, v))
	e.muted = muted
	if e.playing {
		e.stopLocked()
		e.startStreamLocked()
	}
}

// Stop halts all processes.
func (e *PlaybackEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.playing = false
	e.stopLocked()
}

func (e *PlaybackEngine) stopLocked() {
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
}

func (e *PlaybackEngine) startStreamLocked() {
	e.stopLocked()
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFunc = cancel

	path := e.filePath
	startPos := e.position
	rate := e.rate
	duration := e.duration
	vol := e.volume
	isMuted := e.muted
	vw := e.vidWidth
	vh := e.vidHeight
	isAudio := e.isAudio

	hasFFplay := false
	if _, err := exec.LookPath("ffplay"); err == nil {
		hasFFplay = true
	}
	hasFFmpeg := false
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		hasFFmpeg = true
	}

	// Audio pipeline: ffplay if available, otherwise native Windows audio engine
	if !isMuted && vol > 0 {
		if hasFFplay {
			go e.runAudio(ctx, path, startPos, rate, vol)
		} else {
			go e.runAudioNative(ctx, path, startPos, rate, vol)
		}
	}

	// Video / Frame reader pipeline
	if !isAudio && hasFFmpeg {
		go e.runVideo(ctx, path, startPos, rate, duration, vw, vh)
	} else {
		go e.runNativeTimer(ctx, path, startPos, rate, duration, isAudio)
	}
}

// runAudio uses ffplay when installed.
func (e *PlaybackEngine) runAudio(ctx context.Context, path string, start time.Duration, rate, vol float64) {
	intVol := int(math.Round(vol * 100))
	args := []string{
		"-nodisp",
		"-autoexit",
		"-ss", fmt.Sprintf("%.3f", start.Seconds()),
		"-volume", strconv.Itoa(intVol),
	}
	if rate != 1.0 {
		args = append(args, "-af", fmt.Sprintf("atempo=%.2f", rate))
	}
	args = append(args, path)

	cmd := exec.CommandContext(ctx, "ffplay", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Run()
}

// runAudioNative uses Windows Media Player engine natively without ffmpeg.
func (e *PlaybackEngine) runAudioNative(ctx context.Context, path string, start time.Duration, rate, vol float64) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	intVol := int(math.Round(vol * 100))
	startSec := start.Seconds()

	script := fmt.Sprintf(`
$w = New-Object -ComObject WMPlayer.OCX
$w.settings.autoStart = $false
$w.URL = '%s'
$w.settings.volume = %d
$w.settings.rate = %.2f
$w.controls.currentPosition = %.3f
$w.controls.play()

while ($w.playState -ne 1 -and $w.playState -ne 8) {
    Start-Sleep -Milliseconds 100
}
`, strings.ReplaceAll(abs, `'`, `''`), intVol, rate, startSec)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Run()
}

// runVideo streams raw RGBA frames from ffmpeg with hardware acceleration and double-buffered frame reuse.
func (e *PlaybackEngine) runVideo(ctx context.Context, path string, start time.Duration, rate float64, duration time.Duration, vw, vh int) {
	targetFPS := 25.0

	// Scale to max 1080p viewport while preserving exact aspect ratio for smooth HD/4K playback
	decW, decH := vw, vh
	if decW > 1920 {
		decH = int(float64(decH) * (1920.0 / float64(decW)))
		decW = 1920
	}
	if decH > 1080 {
		decW = int(float64(decW) * (1080.0 / float64(decH)))
		decH = 1080
	}
	decW = max(2, (decW/2)*2)
	decH = max(2, (decH/2)*2)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hwaccel", "auto",
		"-threads", "0",
		"-ss", fmt.Sprintf("%.3f", start.Seconds()),
		"-i", path,
		"-s", fmt.Sprintf("%dx%d", decW, decH),
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-r", fmt.Sprintf("%.1f", targetFPS),
		"-",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.runNativeTimer(ctx, path, start, rate, duration, false)
		return
	}

	if err := cmd.Start(); err != nil {
		e.runNativeTimer(ctx, path, start, rate, duration, false)
		return
	}

	frameBytes := decW * decH * 4
	reader := bufio.NewReaderSize(stdout, frameBytes*2)

	// Pre-allocate 2 frame buffers for double-buffering (eliminates continuous garbage collection spikes)
	bufA := make([]byte, frameBytes)
	bufB := make([]byte, frameBytes)
	useBufA := true

	frameInterval := time.Duration(float64(time.Second) / (targetFPS * rate))
	frameDuration := time.Duration(float64(time.Second) / targetFPS)

	currentPos := start
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return
		case <-ticker.C:
			currentBuf := bufA
			if !useBufA {
				currentBuf = bufB
			}
			useBufA = !useBufA

			_, err := io.ReadFull(reader, currentBuf)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					e.handlePlaybackEnded()
				}
				return
			}

			currentPos += frameDuration
			if duration > 0 && currentPos > duration {
				currentPos = duration
			}

			e.mu.Lock()
			e.position = currentPos
			e.mu.Unlock()

			img := &image.RGBA{
				Pix:    currentBuf,
				Stride: decW * 4,
				Rect:   image.Rect(0, 0, decW, decH),
			}

			if e.onFrame != nil {
				e.onFrame(img, currentPos)
			}

			if duration > 0 && currentPos >= duration {
				e.handlePlaybackEnded()
				return
			}
		}
	}
}

// runNativeTimer advances playback in real-time without ffmpeg, rendering audio visualizers or video canvases.
func (e *PlaybackEngine) runNativeTimer(ctx context.Context, path string, start time.Duration, rate float64, duration time.Duration, isAudio bool) {
	step := 33 * time.Millisecond
	interval := time.Duration(float64(step) / rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	currentPos := start

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentPos += time.Duration(float64(step) * rate)
			if duration > 0 && currentPos > duration {
				currentPos = duration
			}

			e.mu.Lock()
			e.position = currentPos
			e.mu.Unlock()

			if e.onFrame != nil {
				var img image.Image
				if isAudio {
					img = GenerateAudioFrame(filepath.Base(path), currentPos, duration, defaultWidth, defaultHeight)
				} else {
					img = GenerateTestFrame(filepath.Base(path), currentPos, duration, defaultWidth, defaultHeight)
				}
				e.onFrame(img, currentPos)
			}

			if duration > 0 && currentPos >= duration {
				e.handlePlaybackEnded()
				return
			}
		}
	}
}

func (e *PlaybackEngine) handlePlaybackEnded() {
	e.mu.Lock()
	e.playing = false
	e.stopLocked()
	endCb := e.onEnd
	e.mu.Unlock()

	if endCb != nil {
		endCb()
	}
}

// GenerateAudioFrame renders an animated audio waveform/spectrum visualization in real-time.
func GenerateAudioFrame(title string, at, total time.Duration, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Dark gradient backdrop
	for y := 0; y < height; y++ {
		grad := uint8(10 + (y * 15 / height))
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: grad, G: grad + 2, B: grad + 6, A: 255})
		}
	}

	// Draw animated frequency spectrum bars in the middle
	numBars := 48
	barWidth := (width - 120) / numBars
	if barWidth < 4 {
		barWidth = 4
	}
	baseX := (width - (numBars * barWidth)) / 2
	centerY := height / 2

	sec := at.Seconds()
	for i := 0; i < numBars; i++ {
		phase := float64(i)*0.25 + sec*3.5
		wave := math.Sin(phase) + 0.5*math.Cos(phase*1.7) + 0.3*math.Sin(phase*2.9)
		h := int(math.Abs(wave) * float64(height/4))
		if h < 4 {
			h = 4
		}

		bx := baseX + i*barWidth
		by1 := centerY - h
		by2 := centerY + h

		rCol := uint8(40 + (i*4)%120)
		gCol := uint8(140 + (i*2)%115)
		bCol := uint8(220)

		for y := by1; y <= by2; y++ {
			if y >= 0 && y < height {
				for x := bx; x < bx+barWidth-2 && x < width; x++ {
					img.SetRGBA(x, y, color.RGBA{R: rCol, G: gCol, B: bCol, A: 255})
				}
			}
		}
	}

	// Bottom progress accent line
	if total > 0 {
		pct := float64(at) / float64(total)
		if pct > 1 {
			pct = 1
		}
		progW := int(float64(width) * pct)
		for y := height - 4; y < height; y++ {
			for x := 0; x < progW; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 56, G: 189, B: 248, A: 255})
			}
		}
	}

	return img
}

// GenerateTestFrame creates a synthetic test frame for demo/fallback.
func GenerateTestFrame(title string, at, total time.Duration, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Dark gradient backdrop
	for y := 0; y < height; y++ {
		grad := uint8(14 + (y * 22 / height))
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: grad, G: grad, B: grad + 4, A: 255})
		}
	}

	// Accent frame border
	for x := 0; x < width; x++ {
		img.SetRGBA(x, 0, color.RGBA{R: 50, G: 50, B: 60, A: 255})
		img.SetRGBA(x, height-1, color.RGBA{R: 50, G: 50, B: 60, A: 255})
	}
	for y := 0; y < height; y++ {
		img.SetRGBA(0, y, color.RGBA{R: 50, G: 50, B: 60, A: 255})
		img.SetRGBA(width-1, y, color.RGBA{R: 50, G: 50, B: 60, A: 255})
	}

	// Progress indicator line at bottom
	if total > 0 {
		pct := float64(at) / float64(total)
		if pct > 1 {
			pct = 1
		}
		progW := int(float64(width) * pct)
		for y := height - 4; y < height; y++ {
			for x := 0; x < progW; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 220, G: 38, B: 38, A: 255})
			}
		}
	}

	// Draw decorative center playback box
	cx, cy := width/2, height/2
	boxSize := 40
	for y := cy - boxSize; y <= cy+boxSize; y++ {
		for x := cx - boxSize; x <= cx+boxSize; x++ {
			if y == cy-boxSize || y == cy+boxSize || x == cx-boxSize || x == cx+boxSize {
				img.SetRGBA(x, y, color.RGBA{R: 242, G: 235, B: 221, A: 200})
			}
		}
	}

	return img
}
