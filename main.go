package main

// #cgo CFLAGS: -I../whisper.cpp/include -I../whisper.cpp/ggml/include
// #cgo LDFLAGS: -L../whisper.cpp
import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/veandco/go-sdl2/sdl"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)

	vthreshold := flag.Float64("v", 0.25, "Voice Activity Detection Energy Threshold (%% of energy)")
	fthreshold := flag.Float64("f", 100.0, "Cutoff Frequency for High Pass Filter")
	devid := flag.Int("d", -1, "Microphone Device ID")
	rate := flag.Int("r", 16000, "Sample Rate")
	modelpath := flag.String("m", "../whisper.cpp/models/ggml-base.en.bin", "Path to Model File")

	flag.Parse()

	if err := sdl.Init(sdl.INIT_AUDIO); err != nil {
		log.Fatal(err)
	}
	defer sdl.Quit()
	aad := NewAsyncAudio(*devid, *rate, 2000)
	aad.Resume()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		model, err := whisper.New(*modelpath)
		if err != nil {
			log.Fatal(err)
		}
		defer model.Close()
		context, err := model.NewContext()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Model Language: ", context.Language())
		context.SetThreads(5)
		context.SetAudioCtx(1500)

		samples := []float32{}
		limit := int(aad.Rate) * 10 // max samples before processing

		for {
			samples = append(samples, aad.Get(2000)...)

			if len(samples) < limit && isSpeech(samples, int(aad.Rate), 1000, float32(*vthreshold), float32(*fthreshold)) {
				// log.Println("Waiting for limit or silence...")
				time.Sleep(1000 * time.Millisecond)
				continue
			}

			// log.Println("processing sample")
			if err := context.Process(samples, nil, nil); err != nil {
				log.Fatal(err)
			}

			// Print out the results
			for {
				segment, err := context.NextSegment()
				if err != nil {
					// log.Println(err) // only EOF is valid
					break
				}
				log.Printf("[%6s->%6s] %s\n", segment.Start, segment.End, segment.Text)
			}
			samples = samples[aad.Rate:]
			context.ResetTimings()
		}
	}()

	<-sigchan

	fmt.Println("Exiting..")
}
