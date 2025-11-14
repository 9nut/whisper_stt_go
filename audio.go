package main

/*
// NOTE: Only works on SDL2 2.0.5 and above!
#include <stdint.h>
extern void cOnAudio(uintptr_t userdata, unsigned char *stream, int len);
*/
import "C"
import (
	"log"
	"math"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

var (
	want, have sdl.AudioSpec
)

type MutexBuf struct {
	sync.Mutex
	Buf []float32
}

type AsyncAudio struct {
	Dev      sdl.AudioDeviceID
	Running  atomic.Bool
	Rate     int32
	LenInMS  int32
	AudioBuf MutexBuf
	WorkBuf  []float32
}

func NewAsyncAudio(capid int, rate int, mslen int32) *AsyncAudio {
	this := AsyncAudio{}

	sdl.SetHintWithPriority(sdl.HINT_AUDIO_RESAMPLING_MODE, "medium", sdl.HINT_OVERRIDE)
	ndev := sdl.GetNumAudioDevices(true)
	for i := 0; i < ndev; i++ {
		log.Printf("capture device #%d: '%s'\n", i, sdl.GetAudioDeviceName(i, true))
	}

	var want, have sdl.AudioSpec

	want.Callback = sdl.AudioCallback(C.cOnAudio)
	want.Channels = 1
	want.Format = sdl.AUDIO_F32
	want.Freq = 16000
	want.Samples = 1024
	want.UserData = unsafe.Pointer(cgo.NewHandle(&this))

	devname := ""
	if capid >= 0 {
		devname = sdl.GetAudioDeviceName(capid, true)
	}
	dev, err := sdl.OpenAudioDevice(devname, true, &want, &have, 0)
	if err != nil {
		log.Fatal(err)
	}
	this.Dev = dev

	log.Printf("Using device %s frequency %d\n", sdl.GetAudioDeviceName(capid, true), have.Freq)
	this.Rate = have.Freq
	this.LenInMS = mslen
	this.AudioBuf.Buf = make([]float32, have.Freq*mslen/1000)
	this.WorkBuf = []float32{}

	return &this
}

func (aad *AsyncAudio) Resume() bool {
	if aad.Running.CompareAndSwap(false, true) {
		sdl.PauseAudioDevice(aad.Dev, false)
		return true
	}
	return false
}

func (aad *AsyncAudio) Pause() bool {
	if aad.Running.CompareAndSwap(true, false) {
		sdl.PauseAudioDevice(aad.Dev, true)
		return true
	}
	return false
}

func Close(aad *AsyncAudio) {
	sdl.CloseAudioDevice(aad.Dev)
	aad = nil
}

func (aad *AsyncAudio) OnAudio(raw *C.uchar, sz int) {
	// log.Println("Received audio:", sz, "bytes")
	data := cpFloat32s(raw, sz)
	aad.AudioBuf.Lock()
	defer aad.AudioBuf.Unlock()
	aad.AudioBuf.Buf = append(aad.AudioBuf.Buf, data...)
}

func cpFloat32s(raw *C.uchar, sz int) []float32 {
	// turn 'raw' into Go []float32
	in := unsafe.Slice((*byte)(unsafe.Pointer(raw)), sz)
	out := make([]float32, sz/4)

	for i := 0; i < sz/4; i++ {
		out[i] = *(*float32)(unsafe.Pointer(&in[i*4]))
	}

	return out
}

func cpBytes(raw *C.uchar, len int) []byte {
	// turn 'raw' into Go []byte
	in := unsafe.Slice((*byte)(unsafe.Pointer(raw)), len)
	out := make([]byte, len)

	for i := 0; i < len; i++ {
		out[i] = in[i]
	}

	return out
}

//export OnAudio
func OnAudio(userdata C.uintptr_t, raw *C.uchar, sz int) {
	// runtime.LockOSThread()
	h := cgo.Handle(userdata)
	aad := h.Value().(*AsyncAudio)
	aad.OnAudio(raw, sz)
}

func (aad *AsyncAudio) Get(ms int32) []float32 {
	if !aad.Running.Load() {
		return []float32{}
	}

	if ms <= 0 {
		ms = aad.LenInMS
	}
	n := aad.Rate * ms / 1000
	// log.Printf("Need %d samples, %d ms\n", n, ms)

	for {
		aad.AudioBuf.Lock()
		aad.WorkBuf = append(aad.WorkBuf, aad.AudioBuf.Buf...)
		aad.AudioBuf.Buf = []float32{}
		aad.AudioBuf.Unlock()

		if len(aad.WorkBuf) >= int(n) {
			break
		}
		// log.Println("Need more, sleeping...")
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	// log.Printf("Got %d samples, %d ms\n", n, ms)
	result := aad.WorkBuf
	aad.WorkBuf = []float32{}
	return result
}

// translated into Go from functions of similar
// names in whisper.cpp/examples/common.cpp
// algorithm is here: https://en.wikipedia.org/wiki/High-pass_filter
func hpf(data []float32, cutoff, rate float32) {
	rc := 1.0 / (2.0 * math.Pi * cutoff)
	dt := 1.0 / rate
	alpha := rc / (rc + dt)

	y := data[0]

	for i := range data {
		if i == 0 {
			continue
		}
		y = alpha * (y + data[i] - data[i-1])
		data[i] = y
	}
}

func isSpeech(data []float32, rate, ms int, vthold, fthold float32) bool {
	nsamples := len(data)
	nlastsamp := (rate * ms) / 1000

	if nlastsamp >= nsamples {
		return false
	}

	if fthold > 0.0 {
		hpf(data, fthold, float32(rate))
	}

	energyAll := 0.0
	energyLast := 0.0

	for i := 0; i < nsamples; i++ {
		dd := float64(data[i] * data[i])
		energyAll += math.Abs(dd)

		if i >= nsamples-nlastsamp {
			energyLast += math.Abs(dd)
		}
	}

	energyAll /= float64(nsamples)
	energyLast /= float64(nlastsamp)

	return energyLast > float64(vthold)*energyAll
}
