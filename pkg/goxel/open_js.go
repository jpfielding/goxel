//go:build js

package goxel

import (
	"errors"
	"fmt"
	"log/slog"
	"syscall/js"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// SamplePath is the sample study fetched by "Load sample", relative to the page.
const SamplePath = "samples/sample_ct.dcm"

// canOpenFolder reports whether this platform can open a folder of slices.
// Browsers only hand us individual files.
const canOpenFolder = false

// openFile shows the browser's file picker. When several files are chosen the
// largest one is loaded, matching roxel's web picker.
func (v *Viewer) openFile() {
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("multiple", true)
	input.Set("accept", ".dcm,.dcs,application/dicom")

	var onChange js.Func
	onChange = js.FuncOf(func(this js.Value, args []js.Value) any {
		onChange.Release()
		files := input.Get("files")
		n := files.Get("length").Int()
		if n == 0 {
			return nil
		}
		largest := files.Index(0)
		for i := 1; i < n; i++ {
			f := files.Index(i)
			if f.Get("size").Float() > largest.Get("size").Float() {
				largest = f
			}
		}
		name := largest.Get("name").String()
		go v.loadBuffer(name, func() ([]byte, error) {
			return awaitBytes(largest.Call("arrayBuffer"))
		})
		return nil
	})
	input.Call("addEventListener", "change", onChange)
	input.Call("click")
}

// openFolder is not available in the browser.
func (v *Viewer) openFolder() {
	dialog.ShowInformation("Open folder", "Folders cannot be opened in the browser. Choose one or more files instead.", v.win)
}

// LoadSample fetches the bundled sample study from the page's origin.
func (v *Viewer) LoadSample() {
	go v.loadBuffer("sample_ct.dcm", func() ([]byte, error) {
		resp, err := await(js.Global().Call("fetch", SamplePath))
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", SamplePath, err)
		}
		if !resp.Get("ok").Bool() {
			return nil, fmt.Errorf("fetching %s: HTTP %d", SamplePath, resp.Get("status").Int())
		}
		return awaitBytes(resp.Call("arrayBuffer"))
	})
}

// loadBuffer reads bytes with read, parses them, and installs the study on
// the UI goroutine. It must run on its own goroutine: read blocks on promises.
func (v *Viewer) loadBuffer(name string, read func() ([]byte, error)) {
	data, err := read()
	if err != nil {
		v.reportLoadError(err)
		return
	}
	slog.Info("Loading buffer", "name", name, "bytes", len(data))
	scan, err := LoadBytes(name, data)
	if err != nil {
		v.reportLoadError(err)
		return
	}
	fyne.Do(func() {
		if err := v.SetScanData(scan); err != nil {
			dialog.ShowError(err, v.win)
		}
	})
}

func (v *Viewer) reportLoadError(err error) {
	slog.Error("Load failed", "error", err)
	fyne.Do(func() { dialog.ShowError(err, v.win) })
}

// await blocks the calling goroutine until promise settles.
func await(promise js.Value) (js.Value, error) {
	type result struct {
		val js.Value
		err error
	}
	ch := make(chan result, 1)
	var onOK, onErr js.Func
	onOK = js.FuncOf(func(this js.Value, args []js.Value) any {
		onOK.Release()
		onErr.Release()
		ch <- result{val: args[0]}
		return nil
	})
	onErr = js.FuncOf(func(this js.Value, args []js.Value) any {
		onOK.Release()
		onErr.Release()
		msg := "promise rejected"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	promise.Call("then", onOK, onErr)
	r := <-ch
	return r.val, r.err
}

// awaitBytes resolves a promise of an ArrayBuffer into a Go byte slice.
func awaitBytes(promise js.Value) ([]byte, error) {
	buf, err := await(promise)
	if err != nil {
		return nil, err
	}
	u8 := js.Global().Get("Uint8Array").New(buf)
	data := make([]byte, u8.Get("length").Int())
	js.CopyBytesToGo(data, u8)
	return data, nil
}
