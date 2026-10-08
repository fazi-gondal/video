package main

import (
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

const (
	stageWidth  = 640
	stageHeight = 360
	frameBytes  = stageWidth * stageHeight * 4
)

// VideoInfo holds metadata about a media file.
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

// ProbeVideo inspects a media file using ffprobe.
func ProbeVideo(path string) (*VideoInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	info := &VideoInfo{
		Path:     path,
		Title:    filepath.Base(path),
		FileSize: st.Size(),
		Width:    stageWidth,
		Height:   stageHeight,
		FPS:      25,
		Duration: 10 * time.Second,
	}

	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	if err == nil {
		var probe ffprobeJSON
		if err := json.Unmarshal(out, &probe); err == nil {
			if probe.Format.Duration != "" {
				if dSec, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && dSec > 0 {
					info.Duration = time.Duration(dSec * float64(time.Second))
				}
			}
			for _, s := range probe.Streams {
				if s.CodecType == "video" {
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
				} else if s.CodecType == "audio" {
					info.AudioCodec = strings.ToUpper(s.CodecName)
				}
			}
		}
	}

	if info.VideoCodec == "" {
		info.VideoCodec = "AVC/H.264"
	}
	if info.AudioCodec == "" {
		info.AudioCodec = "AAC"
	}

	return info, nil
}

// ExtractSingleFrame extracts one RGBA frame at the specified timestamp.
func ExtractSingleFrame(path string, at time.Duration) (image.Image, error) {
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
		stageWidth, stageHeight, stageWidth, stageHeight)

	secStr := fmt.Sprintf("%.3f", at.Seconds())
	cmd := exec.Command("ffmpeg", "-ss", secStr, "-i", path, "-vframes", "1",
		"-vf", scaleFilter, "-f", "rawvideo", "-pix_fmt", "rgba", "-")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return GenerateTestFrame(filepath.Base(path), at, 10*time.Second, stageWidth, stageHeight), nil
	}
	if err := cmd.Start(); err != nil {
		return GenerateTestFrame(filepath.Base(path), at, 10*time.Second, stageWidth, stageHeight), nil
	}

	buf := make([]byte, frameBytes)
	_, readErr := io.ReadFull(stdout, buf)
	_ = cmd.Wait()

	if readErr != nil {
		return GenerateTestFrame(filepath.Base(path), at, 10*time.Second, stageWidth, stageHeight), nil
	}

	img := &image.RGBA{
		Pix:    buf,
		Stride: stageWidth * 4,
		Rect:   image.Rect(0, 0, stageWidth, stageHeight),
	}
	return img, nil
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
	cancelFunc context.CancelFunc
	onFrame    func(img image.Image, pos time.Duration)
	onEnd      func()
}

// NewPlaybackEngine initializes a new engine.
func NewPlaybackEngine(onFrame func(img image.Image, pos time.Duration), onEnd func()) *PlaybackEngine {
	return &PlaybackEngine{
		rate:    1.0,
		volume:  0.8,
		onFrame: onFrame,
		onEnd:   onEnd,
	}
}

// Load sets the active media file and duration.
func (e *PlaybackEngine) Load(path string, duration time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
	e.filePath = path
	e.duration = duration
	e.position = 0
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

// Seek seeks to a given timestamp.
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

	// Extract single frame at target timestamp
	path := e.filePath
	go func(targetPos time.Duration, p string) {
		img, _ := ExtractSingleFrame(p, targetPos)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.filePath == p && e.position == targetPos {
			if e.onFrame != nil {
				e.onFrame(img, targetPos)
			}
			if wasPlaying {
				e.playing = true
				e.startStreamLocked()
			}
		}
	}(pos, path)
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
		// Restart stream to apply audio volume change cleanly
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

	// Audio pipeline via ffplay
	if !isMuted && vol > 0 {
		go e.runAudio(ctx, path, startPos, rate, vol)
	}

	// Video frame reader pipeline
	go e.runVideo(ctx, path, startPos, rate, duration)
}

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

func (e *PlaybackEngine) runVideo(ctx context.Context, path string, start time.Duration, rate float64, duration time.Duration) {
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
		stageWidth, stageHeight, stageWidth, stageHeight)

	// Stream 25 fps raw RGBA video
	targetFPS := 25.0
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", fmt.Sprintf("%.3f", start.Seconds()),
		"-i", path,
		"-vf", scaleFilter,
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-r", fmt.Sprintf("%.1f", targetFPS),
		"-",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.runFallback(ctx, path, start, rate, duration)
		return
	}

	if err := cmd.Start(); err != nil {
		e.runFallback(ctx, path, start, rate, duration)
		return
	}

	frameInterval := time.Duration(float64(time.Second) / (targetFPS * rate))
	frameDuration := time.Duration(float64(time.Second) / targetFPS)

	currentPos := start
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return
		case <-ticker.C:
			buf := make([]byte, frameBytes)
			_, err := io.ReadFull(stdout, buf)
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
				Pix:    buf,
				Stride: stageWidth * 4,
				Rect:   image.Rect(0, 0, stageWidth, stageHeight),
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

func (e *PlaybackEngine) runFallback(ctx context.Context, path string, start time.Duration, rate float64, duration time.Duration) {
	ticker := time.NewTicker(time.Duration(float64(40*time.Millisecond) / rate))
	defer ticker.Stop()
	currentPos := start
	step := 40 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentPos += step
			if duration > 0 && currentPos > duration {
				currentPos = duration
			}
			e.mu.Lock()
			e.position = currentPos
			e.mu.Unlock()

			img := GenerateTestFrame(filepath.Base(path), currentPos, duration, stageWidth, stageHeight)
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
				img.SetRGBA(x, y, color.RGBA{R: 220, G: 38, B: 38, A: 255}) // Accent red line
			}
		}
	}

	// Draw decorative center playback diamond/box
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
