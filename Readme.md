# Video Player

Video player in MyGo Framework with GpuPlayer support.

## Features

- [x] Basic video playback
- [x] Pause and resume
- [x] Seeking
- [x] Volume control
- [x] Fullscreen mode
- [ ] Audio playback
- [x] Subtitle support
- [ ] Hardware decoding support (NVDEC)

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
