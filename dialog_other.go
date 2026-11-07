//go:build !windows

package main

func openMediaFileDialog() (string, error) {
	return "", nil
}

func openAudioFileDialog() (string, error) {
	return "", nil
}

func openVideoFileDialog() (string, error) {
	return "", nil
}

func openNativeSubtitleDialog() (string, error) {
	return "", nil
}

