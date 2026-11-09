//go:build cgo

package qui

import native "github.com/torisetxd/qui/src"

type platform struct {
	app *native.App
}

// SetAppID sets the Wayland app id. Call it before Open. It must match the
// basename of the app's .desktop file.
func SetAppID(id string) { native.SetAppID(id) }

// Open creates the app window. Call it from the main goroutine.
func Open(title string, width, height int) *Ui {
	u := newUi()
	u.app = native.Open(title, width, height)
	return u
}

func (u *Ui) Close() { u.app.Close() }

// Wake requests a frame. It is safe to call from any goroutine.
func (u *Ui) Wake() { u.app.Wake() }

func (u *Ui) consumeBack() { u.app.ConsumeBack() }

// PickFiles opens the system file picker without blocking. The chosen paths
// arrive later through PickedFiles, and the app is woken when they do. Native
// only.
func (u *Ui) PickFiles(multiple bool) { u.app.PickFiles(multiple) }

// PickedFiles returns and clears the paths chosen in the file picker.
func (u *Ui) PickedFiles() []string {
	var out []string
	for {
		path, ok := u.app.PickedFile()
		if !ok {
			return out
		}
		out = append(out, path)
	}
}

// SetClipboard puts text on the system clipboard.
func (u *Ui) SetClipboard(text string) { u.app.SetClipboard(text) }

func (u *Ui) submit() { u.app.Submit(u.buf) }

func (u *Ui) wait() []byte { return u.app.Wait() }
