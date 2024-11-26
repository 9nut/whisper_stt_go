# stream
## Whisper Realtime STT in Go

This is a simple stream ("realtime") speech-to-text transcirber reimplementation of whisper.cpp's `stream` example. It uses `whisper.cpp`.

## Building
Checkout `whisper.cpp` and verify that all the examples are working for the environment you're building. Then build the `Go` language examples:

```shell
$ cd bindings/go
$ make examples
```

If you're building for a `CUDA` target:
```shell
$ cd bindings/go
$ GGML_CUBLAS=1 make examples
$
```

This produces the `libwishper.a` in `whisper.cpp` root directory. Now Go version of `stream` can be compiled using `go build`.  Note that the `cgo` flags in `main.go` expect `whisper.cpp` to be in an adjacent directory (i.e. `../whsiper.cpp` from this directory).

```shell
$ cd ../whisper_stt_go
$ go build
```

### Author: Skip Tavakkolian