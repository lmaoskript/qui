//go:build linux && !android && cgo

package main

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/torisetxd/qui"
	"github.com/torisetxd/qui/sys"
)

const (
	captureSupported = true
	fftSize          = 1024
)

type capture struct {
	wake            func()
	screenOn, micOn atomic.Bool
	opening         atomic.Bool
	energy          atomic.Uint32
	mu              sync.Mutex
	cells           []qui.Color
	spectrum        [bandCount]float32
	message         string
	frames          uint64
	inMenu          bool
}

func newCapture(wake func()) *capture {
	c := &capture{wake: wake, message: "Nothing is captured until you press a button."}
	c.inMenu = c.visibleInMenu()
	return c
}

func dataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".local/share")
}

func desktopFile() string { return filepath.Join(dataHome(), "applications", appID+".desktop") }

func (c *capture) visibleInMenu() bool {
	data, err := os.ReadFile(desktopFile())
	return err == nil && !strings.Contains(string(data), "NoDisplay=true")
}

func (c *capture) say(format string, args ...any) {
	c.mu.Lock()
	c.message = fmt.Sprintf(format, args...)
	c.mu.Unlock()
	c.wake()
}

func (c *capture) status() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.message
}

func (c *capture) live() bool { return c.screenOn.Load() || c.micOn.Load() }

func (c *capture) level() float32 { return math.Float32frombits(c.energy.Load()) }

func (c *capture) screenLabel() string {
	switch {
	case c.opening.Load():
		return "Waiting…"
	case c.screenOn.Load():
		return "Stop sharing"
	}
	return "Share screen"
}

func (c *capture) micLabel() string {
	if c.micOn.Load() {
		return "Mute mic"
	}
	return "Use mic"
}

func (c *capture) menuLabel() string {
	if c.inMenu {
		return "Remove from app menu"
	}
	return "Add to app menu"
}

func (c *capture) toggleMenu() {
	app := sys.App{ID: appID, Name: "qui showcase", Comment: "A desktop built with qui", Categories: "Development;", InstallAs: "qui-showcase"}
	var err error
	if c.inMenu {
		err = sys.Uninstall(app)
	} else {
		err = sys.Install(app)
	}
	if err != nil {
		c.say("%v", err)
		return
	}
	c.inMenu = c.visibleInMenu()
	if c.inMenu {
		c.say("Installed to ~/.local/bin and the application menu.")
	} else {
		c.say("Removed the launcher, the binary and the saved screen permission.")
	}
}

func (c *capture) toggleScreen() {
	if c.opening.Load() {
		return
	}
	if c.screenOn.Swap(false) {
		return
	}
	c.opening.Store(true)
	c.say("Choose a screen in the permission dialog. Later runs restore it without asking.")
	go c.runScreen()
}

func (c *capture) runScreen() {
	if _, err := os.Stat(desktopFile()); err != nil {
		if err := sys.Install(sys.App{ID: appID, Name: "qui showcase", Hidden: true}); err != nil {
			c.opening.Store(false)
			c.say("%v", err)
			return
		}
	}
	screen, err := sys.OpenScreen(sys.ScreenOptions{AppID: appID, FPS: 30, ShowCursor: true})
	c.opening.Store(false)
	if err != nil {
		c.say("%v", err)
		return
	}
	defer screen.Close()
	c.screenOn.Store(true)
	if screen.HadPermission() {
		c.say("Restored the saved permission, so no dialog was needed.")
	} else {
		c.say("Sharing. The permission is saved for next time.")
	}
	var buffer []qui.Color
	for c.screenOn.Load() {
		frame, err := screen.Next(0, 300*time.Millisecond)
		if errors.Is(err, sys.ErrTimeout) {
			continue
		}
		if err != nil {
			c.say("Sharing stopped.")
			break
		}
		buffer = downsample(frame, buffer)
		c.mu.Lock()
		c.cells, buffer = buffer, c.cells
		c.frames++
		c.mu.Unlock()
		c.wake()
	}
	c.screenOn.Store(false)
	c.mu.Lock()
	c.cells = nil
	c.mu.Unlock()
	c.wake()
}

func downsample(f sys.Frame, into []qui.Color) []qui.Color {
	if len(into) != mosaicCols*mosaicRows {
		into = make([]qui.Color, mosaicCols*mosaicRows)
	}
	for row := 0; row < mosaicRows; row++ {
		for col := 0; col < mosaicCols; col++ {
			var r, g, b int
			for sample := 0; sample < 4; sample++ {
				x := (col*f.Width + f.Width*(sample%2*2+1)/4) / mosaicCols
				y := (row*f.Height + f.Height*(sample/2*2+1)/4) / mosaicRows
				at := min(y, f.Height-1)*f.Stride + min(x, f.Width-1)*4
				b += int(f.Data[at])
				g += int(f.Data[at+1])
				r += int(f.Data[at+2])
			}
			into[row*mosaicCols+col] = qui.RGB(uint8(r/4), uint8(g/4), uint8(b/4))
		}
	}
	return into
}

func (c *capture) mosaic() []qui.Color {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]qui.Color(nil), c.cells...)
}

func (c *capture) bands() [bandCount]float32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.spectrum
}

func (c *capture) toggleMic() {
	if c.micOn.Swap(false) {
		return
	}
	c.micOn.Store(true)
	go c.runMic()
}

func (c *capture) runMic() {
	mic, err := sys.OpenAudio(sys.AudioOptions{Name: "qui showcase", Channels: 1})
	if err != nil {
		c.micOn.Store(false)
		c.say("%v", err)
		return
	}
	defer mic.Close()
	c.say("Listening. The wallpaper reacts to your voice; no audio is stored.")
	window := make([]float32, fftSize)
	chunk := make([]float32, fftSize)
	for c.micOn.Load() {
		n, _, err := mic.Read(chunk, 200*time.Millisecond)
		if errors.Is(err, sys.ErrTimeout) {
			continue
		}
		if err != nil {
			c.say("The microphone stream ended.")
			break
		}
		copy(window, window[n:])
		copy(window[fftSize-n:], chunk[:n])
		c.analyze(window)
	}
	c.micOn.Store(false)
	c.energy.Store(0)
	c.mu.Lock()
	c.spectrum = [bandCount]float32{}
	c.mu.Unlock()
	c.wake()
}

func fft(values []complex128) {
	n := len(values)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				even, odd := values[start+k], values[start+k+size/2]*w
				values[start+k], values[start+k+size/2] = even+odd, even-odd
				w *= step
			}
		}
	}
}

func (c *capture) analyze(samples []float32) {
	values := make([]complex128, fftSize)
	sum := 0.0
	for i, s := range samples {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize)
		values[i] = complex(float64(s)*hann, 0)
		sum += float64(s) * float64(s)
	}
	fft(values)
	var bands [bandCount]float32
	for band := range bands {
		low := 2 * math.Pow(180, float64(band)/bandCount)
		high := 2 * math.Pow(180, float64(band+1)/bandCount)
		peak := 0.0
		for bin := int(low); bin <= int(high) && bin < fftSize/2; bin++ {
			peak = max(peak, cmplx.Abs(values[bin]))
		}
		db := 20 * math.Log10(peak/fftSize+1e-7)
		bands[band] = float32(max(0, min(1, (db+84)/60)))
	}
	rms := math.Sqrt(sum / fftSize)
	c.energy.Store(math.Float32bits(float32(min(1, rms*6))))
	c.mu.Lock()
	for i := range bands {
		c.spectrum[i] = max(bands[i], c.spectrum[i]*0.82)
	}
	c.mu.Unlock()
}
