# Video Player

A feature-rich video player built with the [MyGo](https://github.com/egoist/mygo)
framework for Go UI and [MujicaUI](https://github.com/ZacharyZhang-NY/MujicaUI).

## Features

- **Media Playback**: Basic playback, pause, resume, precise seeking,
  and adjustable playback speed.
- **Audio Controls**: Volume adjustment, mute/unmute support,
  and background audio streaming via `ffplay`.
- **Display Modes**: Fullscreen mode, picture-in-picture (PiP),
  and resizable native video rendering.
- **Subtitle Support**: SRT/VTT sidecar file parsing, automatic adjacent
  subtitle detection, and on-screen cue overlay.
- **Playlist & Queue**: Queue management, track switching,
  and multiple item playlist handling.
- **High-Performance UI**: Declarative native UI built using the MyGo
  framework and MujicaUI widgets.

## Dependencies

- [MujicaUI](https://github.com/ZacharyZhang-NY/MujicaUI)
- [MyGo](https://github.com/egoist/mygo)
- [GpuPlayer](https://github.com/ZacharyZhang-NY/GpuPlayer)

## Build & Cgo Requirements

- **Cgo is NOT used**: This project builds completely without Cgo (`CGO_ENABLED=0`).
  - No C compiler (such as GCC, Clang, or MinGW) is required to build the binary.
  - Native windowing and rendering rely on pure Go / `purego`.
  - Media processing communicates with external CLI tools via standard pipes
    rather than Cgo bindings.
- **Runtime Prerequisites**:
  - `ffmpeg`, `ffprobe`, and `ffplay` must be installed and available in `PATH`.

## How to run

```bash
mygo run main.go
```

## How to build

Using MyGo:

```bash
mygo build -o video main.go
```

Using standard Go (pure Go build without Cgo):

```bash
CGO_ENABLED=0 go build -o video.exe .
```
