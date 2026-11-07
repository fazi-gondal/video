//go:build windows

package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	modcomdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW = modcomdlg32.NewProc("GetOpenFileNameW")
)

type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

const (
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnExplorer      = 0x00080000
	ofnNoChangeDir   = 0x00000008
	ofnEnableSizing  = 0x00800000
)

func buildFilterUTF16(pairs [][2]string) []uint16 {
	var buf []uint16
	for _, p := range pairs {
		buf = append(buf, utf16.Encode([]rune(p[0]))...)
		buf = append(buf, 0)
		buf = append(buf, utf16.Encode([]rune(p[1]))...)
		buf = append(buf, 0)
	}
	buf = append(buf, 0)
	return buf
}

// showNativeOpenDialog invokes the Win32 GetOpenFileNameW API directly from comdlg32.dll.
// This is instantaneous, zero-overhead, pure Go (no Cgo, no PowerShell process).
func showNativeOpenDialog(title string, filterPairs [][2]string) (string, error) {
	filterBuf := buildFilterUTF16(filterPairs)
	fileBuf := make([]uint16, 4096)

	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	var ofn openFileNameW
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.lpstrFilter = &filterBuf[0]
	ofn.lpstrFile = &fileBuf[0]
	ofn.nMaxFile = uint32(len(fileBuf))
	ofn.lpstrTitle = titlePtr
	ofn.flags = ofnFileMustExist | ofnPathMustExist | ofnExplorer | ofnNoChangeDir | ofnEnableSizing

	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		// Dialog was cancelled by user
		return "", nil
	}

	return syscall.UTF16ToString(fileBuf), nil
}

var (
	mediaFilterPairs = [][2]string{
		{"All Supported Media (*.mp4, *.mkv, *.mp3, *.wav, ...)", "*.mp4;*.mkv;*.webm;*.avi;*.mov;*.wmv;*.flv;*.m4v;*.ts;*.mp3;*.wav;*.flac;*.aac;*.m4a;*.ogg;*.wma;*.aiff;*.mid;*.opus"},
		{"Audio Files (*.mp3, *.wav, *.flac, *.aac, *.m4a, ...)", "*.mp3;*.wav;*.flac;*.aac;*.m4a;*.ogg;*.wma;*.aiff;*.mid;*.opus"},
		{"Video Files (*.mp4, *.mkv, *.webm, *.avi, ...)", "*.mp4;*.mkv;*.webm;*.avi;*.mov;*.wmv;*.flv;*.m4v;*.ts"},
		{"All Files (*.*)", "*.*"},
	}

	audioFilterPairs = [][2]string{
		{"Audio Files (*.mp3, *.wav, *.flac, *.aac, *.m4a, *.ogg, ...)", "*.mp3;*.wav;*.flac;*.aac;*.m4a;*.ogg;*.wma;*.aiff;*.mid;*.opus"},
		{"All Files (*.*)", "*.*"},
	}

	videoFilterPairs = [][2]string{
		{"Video Files (*.mp4, *.mkv, *.webm, *.avi, *.mov, ...)", "*.mp4;*.mkv;*.webm;*.avi;*.mov;*.wmv;*.flv;*.m4v;*.ts"},
		{"All Files (*.*)", "*.*"},
	}

	subtitleFilterPairs = [][2]string{
		{"Subtitle Files (*.srt, *.vtt)", "*.srt;*.vtt"},
		{"All Files (*.*)", "*.*"},
	}
)

func openMediaFileDialog() (string, error) {
	return showNativeOpenDialog("Open Media File", mediaFilterPairs)
}

func openAudioFileDialog() (string, error) {
	return showNativeOpenDialog("Open Audio File", audioFilterPairs)
}

func openVideoFileDialog() (string, error) {
	return showNativeOpenDialog("Open Video File", videoFilterPairs)
}

func openNativeSubtitleDialog() (string, error) {
	return showNativeOpenDialog("Open Subtitle File", subtitleFilterPairs)
}

