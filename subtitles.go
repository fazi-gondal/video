package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SubtitleCue is a single timed text entry.
type SubtitleCue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// DefaultSampleCues provides subtitle cues for the demonstration video.
var DefaultSampleCues = []SubtitleCue{
	{Start: 500 * time.Millisecond, End: 2500 * time.Millisecond, Text: "Welcome to MyGo & MujicaUI Video Player"},
	{Start: 2800 * time.Millisecond, End: 5200 * time.Millisecond, Text: "Built with high-performance native Go UI"},
	{Start: 5500 * time.Millisecond, End: 7800 * time.Millisecond, Text: "Full playback controls, seek bar, and file picking"},
	{Start: 8000 * time.Millisecond, End: 9800 * time.Millisecond, Text: "Court-Gothic design tokens by Zachary Zhang"},
}

// LoadSubtitles attempts to find an adjacent .srt file or returns default cues.
func LoadSubtitles(videoPath string) []SubtitleCue {
	if videoPath == "" {
		return DefaultSampleCues
	}
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	srtCandidates := []string{
		base + ".srt",
		base + ".vtt",
		filepath.Join(filepath.Dir(videoPath), "subtitles.srt"),
	}
	for _, cand := range srtCandidates {
		if _, err := os.Stat(cand); err == nil {
			if cues, err := ParseSRTFile(cand); err == nil && len(cues) > 0 {
				return cues
			}
		}
	}
	return DefaultSampleCues
}

// ParseSRTFile parses an SubRip (.srt) subtitle file into cues.
func ParseSRTFile(path string) ([]SubtitleCue, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cues []SubtitleCue
	scanner := bufio.NewScanner(file)
	timeRegex := regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})[,.](\d{3})`)

	var current SubtitleCue
	var lines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if current.End > current.Start && len(lines) > 0 {
				current.Text = strings.Join(lines, "\n")
				cues = append(cues, current)
			}
			current = SubtitleCue{}
			lines = nil
			continue
		}

		m := timeRegex.FindStringSubmatch(line)
		if len(m) == 9 {
			start := parseSRTTime(m[1], m[2], m[3], m[4])
			end := parseSRTTime(m[5], m[6], m[7], m[8])
			current.Start = start
			current.End = end
		} else if current.Start > 0 || current.End > 0 {
			lines = append(lines, line)
		}
	}

	if current.End > current.Start && len(lines) > 0 {
		current.Text = strings.Join(lines, "\n")
		cues = append(cues, current)
	}

	return cues, scanner.Err()
}

func parseSRTTime(h, m, s, ms string) time.Duration {
	hours, _ := strconv.Atoi(h)
	mins, _ := strconv.Atoi(m)
	secs, _ := strconv.Atoi(s)
	millis, _ := strconv.Atoi(ms)
	return time.Duration(hours)*time.Hour +
		time.Duration(mins)*time.Minute +
		time.Duration(secs)*time.Second +
		time.Duration(millis)*time.Millisecond
}

// GetSubtitleAt returns the cue text matching the current playback timestamp.
func GetSubtitleAt(cues []SubtitleCue, pos time.Duration) string {
	for _, cue := range cues {
		if pos >= cue.Start && pos <= cue.End {
			return cue.Text
		}
	}
	return ""
}
