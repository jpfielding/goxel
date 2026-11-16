# Plan: goxel UI parity with roxel + browser (wasm) build on GitHub Pages

Status: design only, nothing implemented. Branch `feat/ui-parity-wasm`.
Reference (read-only): `~/projects/dicos.rs/crates/roxel/` and `~/projects/dicos.rs/.github/workflows/pages.yml`.

Everything marked **[verified]** was run or read during planning (Go 1.27.4, darwin/arm64, Fyne v2.7.1). Everything marked **[unverified]** is a design assumption the implementer must check at the named step.

---

## 0. Findings that change the brief

1. **Fresh checkouts of origin/main do not build.** [verified] `.gitignore` has an unanchored `build` line, which ignores the vendored package `vendor/fyne.io/fyne/v2/internal/build/`. In this worktree `go build ./...` fails with `cannot find module providing package fyne.io/fyne/v2/internal/build: import lookup disabled by -mod=vendor`. It only works in `~/projects/goxel` because the directory exists there untracked. CI would fail on line one. Fixed in Step 1.
2. **`make build-ctl` is broken.** [verified] It sets `CGO_ENABLED=0`; Fyne's desktop driver and go-gl need cgo, so the build fails. Fixed in Step 1.
3. **The sample is not 61 MB on this branch.** [verified] origin/main has `pkg/testdata/dicom/sample_ct.dcm` (16,778,813 bytes, CT, 256 x 256 x 128, uncompressed Explicit VR LE, written by goxel). `example3d.dcm` exists only in the dirty main checkout. 16.8 MB is the same size class as the `ct.dcs` (16,778,878 bytes) that dicos.rs already ships on Pages, so no downsampling is needed. goxel also loads the dicos.rs CT samples [verified: `goxel.Load` on `fishtank…/ct.dcs` gives 256 x 256 x 128], but it has **no TDR parser** (`tdr.dcs` fails with "no pixel data element found"), so there are no detections to show from those.
4. **Fyne 2.7.1 runs in the browser with a plain `go build`.** [verified in a browser] No `fyne` CLI needed. After build-tagging the two go-gl files, the whole module compiles for `GOOS=js GOARCH=wasm`; a 40-line `cmd/web/main.go` plus `wasm_exec.js` and a 15-line `index.html` rendered the full current UI, fetched the sample over HTTP and displayed 2D + 3D.
5. **The CPU ray caster is not a viable web renderer.** [verified] It works as a fallback but: single-threaded wasm timings on a 256 x 256 x 128 volume are 170 ms (Fast, 256 px), 600 ms (Fast, 512 px), 3.2 s (High, 512 px) per frame, and each render blocks the browser's only thread. First paint of the sample took about 8 s. It also lacks material bands, presets, clipping, pan and multi-volume. Visible parity needs a WebGL2 port of the GPU ray caster (Step 4). The CPU path is kept as the automatic fallback.
6. **Fyne's wasm driver has gaps roxel does not have.** [verified by reading vendored source]
   - `dialog.ShowFileOpen` / `ShowFolderOpen` are no-ops in the browser (`dialog/file_wasm.go`: `fileOpenOSOverride` returns true, "TODO #2737").
   - `Window.SetOnDropped` is "not implemented yet" on wasm. (roxel has no drag-and-drop either, on any platform, and no URL/query loading: it has a file picker, a "Load sample" button, and auto-loads the sample on web start.)
   - **No touch events at all.** `glfw-js/browser_wasm.go` registers only `mousedown/mouseup/mousemove/wheel/keydown/keyup`; the touch handler is commented out. Taps work through the browser's compatibility mouse events; drags, slider drags and pinch do not.
   - The current layout is unusable at phone width (at 390 px the sidebar takes the whole canvas and GL logs `glScissor: width < 0`).
7. **Neither goxel theme is applied.** [verified] `ProSightTheme` (`pkg/goxel/theme.go`) and `customTheme` (`pkg/goxel/fyne_theme.go`) are never passed to `Settings().SetTheme`; the app runs Fyne's default theme (light in the browser test).
8. **wasm size.** [verified] `goxel.wasm` is 40.7 MB (`-s -w -trimpath`), 12.1 MB gzipped. That is Fyne itself, not the sample (testdata is only imported by tests). Pages serves `application/wasm` compressed, and the limits are 100 MB per file / 1 GB per site, so it deploys; first load is slow on a poor connection.

---

## 1. UI inventory

### 1.1 roxel (target)

**Layout selection** (`ui/layout.rs`): by logical size only. width < 640 -> PhonePortrait; else height < 520 -> PhoneLandscape; else Desktop.

**Desktop** (`ui/mod.rs::draw_desktop_panels`), outside in:

| Area | Content |
|---|---|
| App bar (top, PANEL fill, margin 16 x 8) | hexagon mark 24 px, "roxel" 21 px HEADING, right-aligned open file name 13 px MUTED, middle-elided, hover = full path |
| Empty state (no study) | only the app bar plus a centred card: mark 64 px, "Open a DICOS study" 22 px, one-line description, primary "Open file…" (44 px tall), "Open folder…" (native) or "Load sample" (web), load status, formats note |
| Status bar (bottom, BG fill) | one 12 px MUTED line of navigation hints, changing with focus mode and touch |
| Left sidebar, fixed 232 px, scrollable | **Study**: "Open now" card (tinted 27,47,52 with 4 px ACCENT left edge) showing the file name; "Open file…"; "Open folder…"/"Load sample"; spinner "Loading name…"; error in DANGER plus retry hint. **Details**: Voxels, Extent (mm), Modality, Volumes as key/value rows. **Volumes** (only if > 1): selectable full-width rows. **Detections**: heading with "N reported"; "No threat report was loaded with this study." when empty; else "Show detection boxes" checkbox, "Show all"/"Hide all", "Show only the selected one", then rows (checkbox, colour swatch, name, confidence %) |
| Right slice panel, width = clamp(0.36 x window, 336, min(520, window - 232 - 532)) | header "2D slice" + "Fit image" + "Focus slice"; black image well with 30 px footer (scale bar "N mm" left, caption "Axial 12 / 128 · 640 x 440 mm" right); **Slice** group: Axial/Coronal/Sagittal segmented, slice slider, Previous/Next; **2D windowing** ("stored values"): Center, Width sliders; **Image**: Zoom 50-500 % step 25, Volume combo (if > 1), "Composite projection" checkbox |
| Centre top: volume header | row 1: "3D view" title, right side "View along" X Y Z buttons, "Fit view", "Focus volume"; row 2: "Slice plane" checkbox, framed "Cutaway" + dependent "Reverse cutaway"; when unavailable the note "Needs a single 2D slice of this volume"; in focus mode also the plane picker and slice slider |
| Centre: 3D viewport | volume render; overlays: slice-plane outline in ACCENT, corner readout "Slice plane · Axial 12 / 128", X/Y/Z axis gizmo bottom-left |
| Centre bottom: shelf, 3 equal columns (2 if width < 720, lighting then becomes a "Lighting" menu button) | **3D windowing**: Center, Width, Opacity 0-1, Density 0-1. **Palette**: Materials / Threat / Grayscale segmented, fixed 34 px legend (band dots+names / grey ramp with low-high values / "Dense material in red"), Quality Fast / Balanced / High, "Material bands" menu button (Materials only). **Lighting**: Depth cue, Ambient, Diffuse, Specular 0-1 |

**Material bands editor** (popup, 340 px): "Band thresholds" strip (`ui/band_slider.rs`: 84 px tall, coloured segments, round handles r=6 with the threshold value above, band name below when the segment is > 36 px; nearest-handle drag, clamped between neighbours), then "Band opacity" one 0-2x slider per band. Default bands: Air 8000 (transparent), Organic 15000, Inorganic 20000, Metal 25000, Dense 30000 — identical to goxel's `volume.DefaultColorBands()`.

**Focus modes**: "Focus volume" hides sidebar and slice panel; "Focus slice" replaces everything with slice header, the well, and the three slice groups docked below in 3 columns. Esc exits (unless a popup or text field has focus). Both re-fit the camera.

**Compact layouts** (`ui/phone.rs`): bottom tab bar (49 px + home-indicator inset 34/21) with Study, Detections (red count badge), 2D slice, 3D view; active tab has an ACCENT top bar and ACCENT text; tapping the active view tab collapses/expands its controls sheet (chevron). Sheet is fixed size (portrait: bottom, 8+44+8+(4x44+3x8)+8 px; landscape: right, 400 px) and **paged, not scrolled**: 3D pages Camera / Slice / Windowing / Display / Lighting; 2D pages Slice / Image / Volume. Camera page gives tap equivalents for every gesture (Fit view, Focus volume, View along X/Y/Z, Orbit|Pan mode toggle with four arrow buttons, zoom -/+). Touch sliders are 44 px rows with label+value stacked left and -/+ steppers. Focus on compact: no app bar or tab bar, one bottom action bar "Exit focus | Fit view/Fit image | Adjust display".

**Theme** (`ui/theme.rs`), dark only:

| Token | RGB | Use |
|---|---|---|
| BG | 16,22,29 | window, status bar, tab bar, inputs' extreme bg |
| PANEL | 23,31,40 | panels, popups |
| TEXT | 222,231,239 | body text |
| HEADING | 244,248,251 | titles |
| MUTED | 184,196,208 | labels, notes |
| ACCENT | 105,211,191 | focus ring (2 px), selection stroke, mark, slice plane |
| DANGER | 246,151,141 | errors, badge |
| SELECTED | 36,81,80 | selected/active fill |
| OUTLINE | 124,148,168 | control borders |
| RAIL | 104,126,146 | slider rail |
| THUMB | 236,242,247 | slider thumb, hover border |
| HAIRLINE | 48,62,76 | separators |
| WELL | 0,0,0 | image well |
| control fill / hover | 38,52,66 / 52,70,86 | buttons |

Type: egui default proportional font. Desktop sizes Body 13.5, Button 13, Heading 17, Small 12; compact 16 / 15 / 18 / 13. Corner radius 5 (controls), 6 (cards, well), 7 (segmented). Spacing 8 x 8, button padding 10 x 5, control height 28 (44 on touch). Label column 64 px, number column 56 px.

**Interactions**: 3D mouse: drag orbits (0.005 rad/px), Shift+drag pans, wheel zooms (factor 1 + 0.1 x lines). 3D touch (`touch.rs`): one finger orbits (or pans when "Pan" is selected), two fingers pan + pinch-zoom. 2D: Ctrl+scroll or pinch zooms 50-500 %, drag pans inside the well. Keyboard: Esc only. File open: native picker (file or folder) on desktop; on web a multi-file picker where the **largest file is the CT**, plus "Load sample" (fetches `samples/fishtank-knife/{ct,tdr}.dcs` relative to the page) and the sample auto-loads at start. No drag-and-drop, no URL parameters.

**State model** (`state.rs`): one `UiState` of plain data — camera; layout class; `focus_volume`, `slice_expanded`; phone tab/sheet/page fields; `show_slice_locator`, `cutaway`, `reverse_cutaway`; `library` (volumes, active 3D index, loaded path, show_threats, selected threat); `transfer` (preset, bands, debounce deadline 60 ms); `settings` (window center/width, density, quality, ambient 0.65, diffuse 0.8, specular 0.3, depth cue 0.35, opacity 0.25); `slice_view` (orientation, index, max, window, composite, volume index, zoom); `loading_file`, `load_error`; transient `actions`. Every control is drawn by a small function taking `&mut UiState`, reused between desktop panels and phone sheet pages. Loading is asynchronous with at most one load in flight.

### 1.2 goxel today

**Layout** (`pkg/goxel/ui.go::createSideBySidePanel`): `Border(left = VScroll(Accordion[Metadata | Layers | Findings]), center = HSplit(3D panel, 2D panel) at 0.55)`. Before a load: a single centred label "Open a DICOM file or folder to view 3D volume". Main menu File (Open File…, Open Folder…, Export Screenshot [stub]) and View (Full Screen). Window 1920 x 1000. No app bar, status bar, focus modes, or size-dependent layout.

- **Left accordion**: Metadata form (10 DICOM patient/study/series/equipment fields); Layers (per volume: info button, enable checkbox, alpha slider, value); Findings ("All Findings" toggle, per finding: colour square, checkbox, alpha slider). `ScanCollection.Findings` is never populated by `parseDICOMDataset`, so Findings is empty for every file goxel can load today.
- **3D panel** (`volume.go::VolumeRenderer` over `volume.GPUVolumeRenderer`, CPU fallback): under the view, `MultiRangeSlider` (band thresholds, same five bands), "Band Opacity" accordion (0-2 per band), Quality select (Fast/Medium/High — **no effect on the GPU path**), Preset select (DEFAULT / FINDING / MONOCHROME / LAPTOP_REMOVAL), Opacity slider (default 0.5, shared with 2D composite), Z Scale slider, "BG Color" dialog. First band handle doubles as the density threshold.
- **2D panel** (`DICOXImage` + `Interactive2DView`): Volume select, View select (Axial/Coronal/Sagittal, default Coronal), "Composite View" checkbox (**default on**, hides the slice slider), W/L and W/W text entries, Slice `LabeledSlider`, "Add Finding" (draws a rectangle, then an info dialog; nothing is stored). Image is `ImageFillContain`; no zoom, pan, scale bar or caption.
- **Interactions**: 3D drag rotates when the pixel under the cursor is not background and pans when it is (0.01 rad/px); wheel zooms 0.5-5. Keys: Up/Right and Down/Left step slices, S = screenshot stub. File open via Fyne dialogs; load is synchronous on the UI goroutine; errors in a modal dialog.
- **Renderer API that already exists but has no UI**: `SetWindowLevel/Width`, `SetLighting`, `SetDensityThreshold`, `SetClippingPlane`, `SetPan/AdjustPan`, `Params/SetParams` (includes `StepSize`), `ResetView`. The desktop GPU path draws its own axis gizmo (`render_gizmo.go`).
- **State model**: none as such. `Viewer` holds widgets and data together; state lives in widgets and closures; `updateLayout` rebuilds the whole panel after each load. Slider callbacks are debounced 50 ms through `debouncer`.
- **Theme**: Fyne default (see finding 7).

### 1.3 Gap table

Priority: **P1** = needed for visible parity, in this plan's main steps. **P2** = parity polish, own step, can ship after the first deploy. **Out** = not planned, with the reason.

| # | Feature | roxel | goxel today | Change | Files | Pri |
|---|---|---|---|---|---|---|
| 1 | Theme | dark palette above | Fyne default; two dead theme types | one `fyne.Theme` with roxel's tokens, applied in both entry points; delete the two unused themes; set the 3D clear colour to BG | `pkg/goxel/theme.go` (rewrite), delete `fyne_theme.go`, `pkg/volume/theme.go` + `config.go` default background, `cmd/main.go`, `cmd/web/main.go` | P1 |
| 2 | State model | plain `UiState`, draw functions | state in widgets | `uiState` struct + `Viewer.rebuild()`; one constructor function per control group | new `pkg/goxel/state.go`, `ui.go` | P1 |
| 3 | Responsive layout | Desktop / PhonePortrait / PhoneLandscape by size | fixed HSplit | root custom `fyne.Layout` that reports the class; rebuild on change | new `pkg/goxel/responsive.go`, `ui.go` | P1 |
| 4 | App bar + status bar | mark, name, file name; nav hints | none | add | new `pkg/goxel/chrome.go` | P1 |
| 5 | Empty state | centred card with open actions | one label | add card | `chrome.go` | P1 |
| 6 | Sidebar | Study / Details / Volumes / Detections, 232 px | accordion Metadata / Layers / Findings | Study card + open buttons + load status; Details (4 rows); Volumes = existing layer rows when > 1; Findings with roxel's empty sentence ("No findings were loaded with this study."); existing DICOM metadata form kept, collapsed, at the bottom | new `pkg/goxel/sidebar.go`, `ui.go` | P1 |
| 7 | Async load, spinner, inline error | yes | synchronous, modal error | load in a goroutine, `fyne.Do` to install; `loadingFile` / `loadError` in state | `ui.go`, `sidebar.go` | P1 |
| 8 | 3D header | View along X/Y/Z, Fit view, Focus volume, Slice plane, Cutaway, Reverse | none | add; needs `VolumeRenderer.LookAlong(axis)` and cutaway wiring to `SetClippingPlane` | new `pkg/goxel/controls3d.go`, `volume.go` | P1 (slice-plane *outline* is P2) |
| 9 | 3D shelf | windowing, opacity, density, palette, legend, quality, lighting | band slider, band opacity, quality, preset, opacity, Z scale, BG colour | regroup into three columns; add Center/Width, Density, Lighting sliders (APIs exist); Materials/Threat/Grayscale segmented mapping to DEFAULT / FINDING / MONOCHROME; legend strip; band editor moves into a popup; Quality maps to `StepSize` | `controls3d.go`, `volume.go`, `multi_range_slider.go` (touch sizes only) | P1 |
| 10 | Depth cue | slider | not in shader | add uniform to the shared shader body | `pkg/volume` shader | P2 |
| 11 | 2D panel | header, well + footer, Slice / Windowing / Image groups | selects, entries, slider | segmented plane picker, slider + Previous/Next, Center/Width sliders (commit on release), Zoom slider, Volume select, Composite checkbox **default off**, caption under the image | new `pkg/goxel/controls2d.go`, `interactive_view.go` | P1 |
| 12 | 2D zoom/pan gestures, scale bar | Ctrl+scroll, pinch, drag-pan, mm scale bar | none | zoom slider drives an image inside `container.NewScroll`; Ctrl+scroll and scale bar later | `interactive_view.go` | slider P1, rest P2 |
| 13 | Focus modes + Esc | yes | none | `focusVolume` / `focusSlice` in state; Esc in `SetOnTypedKey`; arrows keep stepping slices | `ui.go` | P1 |
| 14 | Phone layouts | tab bar, paged sheets, focus bar, touch sliders | none | build from the same group constructors | new `pkg/goxel/phone.go` | P1 |
| 15 | 3D mouse | Shift+drag pans | background-drag pans | pan when Shift is held (`desktop.Driver.CurrentKeyModifiers`, desktop only) **or** on background drag (kept; it is the only option where modifiers are unavailable) | `volume.go` | P1 |
| 16 | Touch | orbit, pinch, two-finger pan | none on web (Fyne limitation) | JS shim in `index.html`: one finger -> mouse events, pinch -> wheel events; plus the Camera page's tap buttons. Two-finger pan is **not** reproduced; the Orbit/Pan toggle covers it | `cmd/web/index.html`, `phone.go` | P1 |
| 17 | File open on web | multi-file picker, largest = CT | no-op dialog | `<input type=file>` via `syscall/js`; largest file is loaded | new `pkg/goxel/open_js.go`, `open_desktop.go`, `LoadBytes` in `dicom_loader.go` | P1 |
| 18 | Sample | "Load sample" + auto-load on web | none | fetch `samples/sample_ct.dcm` relative to the page | `open_js.go`, `cmd/web/main.go` | P1 |
| 19 | Web 3D renderer | wgpu/WebGPU | go-gl only | WebGL2 implementation of `GPUVolumeRenderer` behind a build tag; CPU fallback | `pkg/volume` | P1 |
| 20 | Orientation gizmo, slice-plane outline, finding boxes on web | drawn as 2D overlay | drawn in GL (desktop) | port the two line programs to WebGL2 after the volume pass works | `pkg/volume/gpu_ray_caster_js.go` | P2 |
| 21 | Detections from a threat report | TDR parsed | no TDR parser | not planned here. The Findings section shows the empty sentence. Adding TDR means a parser in `pkg/dicom` (or a dependency on dicos.go) — separate decision | — | Out |
| 22 | Drag-and-drop, URL loading | not in roxel | not in goxel | none (parity is "absent"); Fyne wasm cannot do drop anyway | — | Out |
| 23 | Folder open on web / multi-slice series from the picker | not in roxel web | path-based `LoadDICOMDir` | not planned; web loads one multi-frame file | — | Out |
| 24 | Editable number boxes beside sliders | DragValue | none | read-only value labels | — | Out |

**goxel-only controls with no roxel counterpart — proposed handling (confirm with the owner):** Z Scale slider and BG Color dialog are removed from the layout (Z scale is already derived from voxel spacing on load; background becomes the theme's BG); `LAPTOP_REMOVAL` preset leaves the picker (config entry stays); "Add Finding" stays as a button in the desktop Image group, unchanged; File/View main menu stays on desktop and is not created in the browser (`fyne.CurrentDevice().IsBrowser()`); per-layer enable/alpha rows stay inside the Volumes section. If any of the three removals is unwanted, keep that control in the Palette column — it is a one-line change in `controls3d.go`.

---

## 2. WASM design

### 2.1 What breaks, exactly [verified]

`GOOS=js GOARCH=wasm go build ./...`:

1. vendored `fyne.io/fyne/v2/internal/build` missing (finding 1) — not wasm-specific.
2. `pkg/volume` imports `github.com/go-gl/gl/v3.3-core/gl` and `glfw/v3.3/glfw` in `gpu_ray_caster.go` and `render_gizmo.go`: "build constraints exclude all Go files".
3. With those two files tagged `//go:build !js`: `cpu_ray_caster.go:244,314: undefined: decodeJPEGFrame` (two pure-Go helpers live in the GPU file).
4. With the helpers available: `pkg/goxel/volume.go:29,83: undefined: volume.GPUVolumeRenderer, volume.NewGPUVolumeRenderer`. Nothing else. `cmd/main.go` (cobra, `os/signal`, lumberjack, `net/http/httputil`) compiles for js unchanged.

A js-tagged file that declares `type GPUVolumeRenderer struct{}` with the same 36 exported methods and a `NewGPUVolumeRenderer` that returns an error makes the whole module build and run in the browser on the CPU fallback, with **zero changes to `pkg/goxel/volume.go`**.

### 2.2 Renderer decision

Same type name, one implementation per platform — the standard-library pattern (`file_unix.go` / `file_windows.go`), no interface, no registry:

- `pkg/volume/gpu_ray_caster.go`, `render_gizmo.go`: add `//go:build !js`. No other change except moving the helpers and the shader body out (below).
- `pkg/volume/jpeg_frame.go` (untagged): `extractPixelsFromImage`, `decodeJPEGFrame`, moved verbatim.
- `pkg/volume/shader_body.go` (untagged): the fragment shader **body** as one constant, with three macros supplied by a per-platform header so both back ends compile the same ray-march code:
  - desktop header: `#version 330 core`, `#define TF_SAMPLER sampler1D`, `#define TF_SAMPLE(n) texture(transferTex, n)`, `#define DECODE_GRAD(v) ((v) * 2.0 - 1.0)`
  - web header: `#version 300 es`, `precision highp float; precision highp sampler3D;`, `#define TF_SAMPLER sampler2D`, `#define TF_SAMPLE(n) texture(transferTex, vec2(n, 0.5))`, `#define DECODE_GRAD(v) (v)`
  This is the one deliberate edit to the desktop renderer. Reason: every parity feature that touches the shader (depth cue, later ones) would otherwise be written twice and drift. If the owner prefers not to touch the desktop shader at all, copy the body into the js file instead and accept the duplication.
- `pkg/volume/gpu_ray_caster_js.go` (`//go:build js`): `GPUVolumeRenderer` on a private WebGL2 context through `syscall/js`. It mirrors the desktop design, which already renders off-screen and reads back into an `*image.RGBA` that Fyne displays:
  - context: `document.createElement("canvas")` (never attached), `getContext("webgl2", {antialias:false, preserveDrawingBuffer:false})`. If null, `NewGPUVolumeRenderer` returns an error and `VolumeRenderer` falls back to CPU exactly as on a desktop without GL. Fyne's own canvas uses a separate WebGL1 context; they do not interact. `fyne-io/gl-js` is not usable for this (WebGL1 only, no `TexImage3D`, global context).
  - volume texture: `texStorage3D(TEXTURE_3D, 1, RGBA16F, x, y, z)` then one `texSubImage3D` per Z slice with `HALF_FLOAT` data, `LINEAR` filtering. `RGBA16` normalised (what desktop uses) is not in WebGL2 core; `RGBA16F` is, and is filterable without extensions. R = density / 65535; GBA = signed central-difference gradient / 65535 stored directly (hence `DECODE_GRAD(v) = v`; storing it desktop-style around 0.5 would leave only about 0.001 of gradient resolution in half floats, right at the shader's lighting threshold). Needs a ~20-line `float32 -> float16 bits` function. Per-slice upload keeps the transient buffer at one slice (512 KB for the sample) instead of 67 MB.
  - size guard: RGBA16F costs 8 bytes per voxel on the GPU (67 MB for the sample; 721 MB for a 512 x 512 x 344 study). On js only: if voxels > 32 M, halve each axis (box filter) before upload and log it.
  - transfer function: `RGBA8`, `TEXTURE_2D`, N x 1, `LINEAR` (the source table is already 8-bit `color.RGBA`; `RGBA32F` + `LINEAR` would need an extension).
  - render: full-screen triangle into an RGBA8 framebuffer at the renderer's size, `readPixels` into a `Uint8Array`, `js.CopyBytesToGo` into `rendered.Pix`, flip rows. Create at 768 x 768 on web (`NewVolumeRenderer` passes 1024 today; choose by `fyne.CurrentDevice().IsBrowser()`), 512 when Quality = Fast.
  - threading: no worker goroutine or command channel; wasm is single-threaded, so `Render()` runs the GL calls inline. Keep the mutex-guarded setter bodies the same as desktop (they only set fields and dirty flags — copy them; about 250 trivial lines. Splitting the desktop file to share them is a larger refactor of working code and is not proposed).
  - first version draws volumes only. `AddFinding`, `ClearFindings`, `SetFindingMode` store data and draw nothing until Step 9; the gizmo likewise.
- `pkg/goxel/volume.go`: unchanged for the swap. Small additions for parity features only (Step 5).

**Infeasible / reduced on web, and the closest alternative**

| Item | Why | Alternative |
|---|---|---|
| Native file/folder dialog | Fyne wasm no-op | own `<input type=file>` (Step 3) |
| Drag-and-drop | Fyne wasm stub | none; roxel has none either |
| Real touch gestures in Fyne widgets | Fyne wasm has no touch input | JS touch-to-mouse/wheel shim + tap controls (Step 7); two-finger pan dropped |
| Multi-threaded CPU rendering | Go wasm is single-threaded | WebGL2; CPU only as fallback |
| Large studies (> ~32 M voxels) at full resolution | GPU and wasm memory | 2x downsample for the 3D texture on js |
| Small download | Fyne wasm baseline is ~40 MB raw | none within Fyne; gzip brings transfer to ~12 MB |

### 2.3 Files and data in the browser

- `pkg/goxel/dicom_loader.go`: add `LoadBytes(name string, data []byte) (*ScanCollection, error)` = `dicom.ReadBuffer` + existing `parseDICOMDataset(ds, name)`. [verified working in the browser experiment]
- `pkg/goxel/open_desktop.go` (`!js`): `openFile` / `openFolder` moved from `ui.go` unchanged in behaviour (Fyne dialogs), `loadSample` absent.
- `pkg/goxel/open_js.go` (`js`):
  - `openFile`: create `<input type="file" multiple accept=".dcm,.dcs">`, `click()`, on `change` read each `File.arrayBuffer()` (promise `then` with `js.FuncOf`, `js.CopyBytesToGo`), pick the largest, `LoadBytes`. The click happens from a goroutine shortly after the mouse event; browsers accept it within the transient-activation window. **[unverified in Safari]** — check at Step 3; fallback is a real HTML button overlaid by `index.html`.
  - `loadSample`: `fetch("samples/sample_ct.dcm")` via `syscall/js` (relative URL, so it works under `https://<user>.github.io/goxel/`). Using `syscall/js` rather than `net/http` keeps `net/http` out of the binary.
- `cmd/web/main.go` (`//go:build js`): `app.New()`, set theme, `goxel.NewViewer`, start `loadSample` (roxel auto-loads its sample so first-time visitors do not get an empty screen), `Run()`. No cobra, no signal handling, no log files.
- `cmd/web/index.html`: page chrome in roxel's colours (`background:#10161d`), a "Loading goxel… (about 12 MB)" line removed when the module starts, an error box if `WebAssembly.instantiateStreaming` fails or WebGL is missing, `<script src="wasm_exec.js">`, and the touch shim (Step 7). `wasm_exec.js` is copied from `$(go env GOROOT)/lib/wasm/` at build time so it always matches the toolchain; it is not committed.
- Sample: `pkg/testdata/dicom/sample_ct.dcm`, copied to `samples/` at build time. Optional later: re-encode it as JPEG-LS once the new `jpegs` release is in (smaller download, and the demo then exercises the codec in the browser) — measure wasm decode time first.

---

## 3. Deployment

### 3.1 Makefile

Fix `build-ctl` (drop `CGO_ENABLED=0`), then add:

```make
WEB_DIR = bin/web

build-wasm: ## Builds the browser (wasm) viewer into bin/web
	mkdir -p $(WEB_DIR)/samples
	GOOS=js GOARCH=wasm go build -trimpath \
		-ldflags "-s -w -X 'main.GitSHA=$(GIT_SHA)'" \
		-o $(WEB_DIR)/goxel.wasm ./cmd/web
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(WEB_DIR)/
	cp cmd/web/index.html $(WEB_DIR)/
	cp pkg/testdata/dicom/sample_ct.dcm $(WEB_DIR)/samples/

serve-wasm: build-wasm ## Serves bin/web on http://127.0.0.1:8080
	python3 -m http.server 8080 --bind 127.0.0.1 --directory $(WEB_DIR)
```

`python3 -m http.server` serves `.wasm` as `application/wasm` [verified], which `instantiateStreaming` requires. `bin/` is already git-ignored.

### 3.2 Workflows (goxel has no `.github/` today)

`.github/workflows/ci.yml` — on push and pull_request:

```yaml
name: CI
on: { push: { branches: [main] }, pull_request: {} }
permissions: { contents: read }
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - name: GL/X11 headers for go-gl and Fyne
        run: sudo apt-get update && sudo apt-get install -y libgl1-mesa-dev xorg-dev
      - run: go vet ./pkg/...
      - run: go test -short ./pkg/...
      - name: wasm compiles
        run: GOOS=js GOARCH=wasm go build ./...
```

`.github/workflows/pages.yml` — mirrors dicos.rs: same triggers, permissions, concurrency group, build job then deploy job gated on main.

```yaml
name: Pages
# One-time owner setup: Settings -> Pages -> Source: "GitHub Actions".
on: { push: { branches: [main] }, workflow_dispatch: {} }
permissions: { contents: read, pages: write, id-token: write }
concurrency: { group: pages, cancel-in-progress: true }
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: make build-wasm
      - run: du -sh bin/web bin/web/*
      - uses: actions/configure-pages@v5
      - uses: actions/upload-pages-artifact@v3
        with: { path: bin/web }
  deploy:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    environment: { name: github-pages, url: "${{ steps.deployment.outputs.url }}" }
    steps:
      - id: deployment
        uses: actions/deploy-pages@v4
```

The wasm build needs no cgo and no apt packages. `-mod=vendor` is implicit; this only works after Step 1 restores the ignored vendored directory. Open point: Pages on a private repository needs a paid plan — the owner should confirm the repo's visibility.

---

## 4. Implementation steps

Each step leaves desktop working and ends with its checks green. Steps 1-4 produce a deployable browser build with the old layout; 5-8 are the parity work; 9 is polish.

### Step 1 — Repair the baseline (vendor, Makefile)
- `.gitignore`: change `build` to `/build` (and `bin`, `tmp` to `/bin`, `/tmp` for the same reason).
- `go mod vendor`, then `git status` must show `vendor/fyne.io/fyne/v2/internal/build/` as new files; add them.
- Makefile `build-ctl`: remove `CGO_ENABLED=0`.
- **Checks:** in a clean clone/worktree `go build ./...` and `go test -short ./pkg/...` pass; `make build-ctl` produces `bin/goxel`; `git status --ignored vendor` lists nothing.

### Step 2 — jpegs bump (coordinate with the parallel agent)
- `go get github.com/jpfielding/jpegs@<new tag>`, `go mod tidy`, `go mod vendor`.
- Do this immediately after Step 1 or rebase onto whichever lands first; both rewrite `vendor/modules.txt`.
- **Checks:** `go test ./pkg/...` (full, not `-short`) passes; `git diff --stat vendor/` touches only `github.com/jpfielding/jpegs` and `modules.txt`.

### Step 3 — wasm build on the CPU fallback
- `pkg/volume`: tag `gpu_ray_caster.go` and `render_gizmo.go` `//go:build !js`; move the two JPEG helpers to `jpeg_frame.go`; add `gpu_ray_caster_js.go` with the type, `NewGPUVolumeRenderer` returning `errors.New("webgl2 renderer not implemented")`, and no-op methods (signatures: `grep '^func (gr \*GPUVolumeRenderer) [A-Z]' pkg/volume/gpu_ray_caster.go`).
- `pkg/goxel`: `LoadBytes`; split `openFile`/`openFolder` into `open_desktop.go`; add `open_js.go` (file input, `loadSample`); in `CreateRenderer` use `canvas.ImageScaleSmooth` when `IsBrowser()` (`ImageScaleFastest` logs "Invalid canvas.ImageScale value" on every refresh in the browser [verified]).
- `cmd/web/main.go`, `cmd/web/index.html`, Makefile targets, both workflows.
- **Checks:** `GOOS=js GOARCH=wasm go build ./...` passes; native build and tests unchanged; `make serve-wasm`, open `http://127.0.0.1:8080`: the viewer appears, console shows `Loaded DICOM … width=256 height=256 depth=128`, the 2D image shows, a (slow, rough) 3D image shows; File > Open File… opens the browser picker and loading `pkg/testdata/dicom/sample_ct.dcm` from disk works in Chrome, Firefox and Safari. On a fork or after merge: the Pages workflow is green and the published URL loads the sample.

### Step 4 — WebGL2 renderer
- `shader_body.go` + platform headers; desktop compiles the shared body.
- Implement `gpu_ray_caster_js.go` as in 2.2 (volumes only).
- **Checks:** desktop render of the sample is visually unchanged before/after the shader split (take a screenshot of the same camera pose before starting). In the browser: console shows `GPU volume renderer initialized successfully`, not the CPU warning; the 3D view matches the desktop image for the sample (same bands, same lighting); dragging orbits smoothly (target: > 20 fps at 768 px on an integrated GPU; measure with the browser's performance panel); moving a band handle recolours within a debounce interval; first paint after page load under 3 s on localhost. Force the fallback (`getContext` returning null via a devtools override) and confirm the CPU path still displays.

### Step 5 — Theme, state, desktop layout
- `theme.go`: `goxelTheme` implementing `fyne.Theme` with section 1.1's tokens. Mapping: Background -> BG; OverlayBackground, MenuBackground, HeaderBackground -> PANEL; Foreground -> TEXT; Placeholder, Disabled -> MUTED; Primary, Focus -> ACCENT; Selection, Pressed -> SELECTED; Button, InputBackground -> (38,52,66); Hover -> (52,70,86); InputBorder -> OUTLINE; Separator -> HAIRLINE; ScrollBar -> RAIL; Error -> DANGER. Sizes: Text 13.5, CaptionText 12, SubHeadingText 17, HeadingText 21, Padding 4, InnerPadding 8, InputRadius/SelectionRadius 5. Fonts: Fyne's bundled font (typeface parity with egui's default is not attempted). Always return the dark values regardless of variant. Delete `ProSightTheme` and `customTheme`. Apply with `a.Settings().SetTheme` in `cmd/main.go` and `cmd/web/main.go`. Set `volume` config background to BG.
- `state.go`: `uiState` with exactly: `layout` (desktop/phonePortrait/phoneLandscape), `focusVolume`, `focusSlice`, `phoneTab`, `sheetOpen`, `focusAdjustOpen`, `volumePage`, `slicePage`, `bandsOpen`, `selectedBand`, `dragPans`, `showSlicePlane`, `cutaway`, `reverseCutaway`, `preset`, `bands`, `quality`, `windowCenter3D`, `windowWidth3D`, `opacity`, `density`, `ambient`, `diffuse`, `specular`, `orientation`, `sliceIndex`, `composite`, `zoom2D`, `windowCenter2D`, `windowWidth2D`, `loadedName`, `loadingFile`, `loadError`. Controls write to state and call one `apply…` function; `Viewer.rebuild()` builds the widget tree from state and is called only when layout class, focus, tab, page, or loaded study changes.
- Small helpers in `widgets.go`: `segmented(labels, selected, onSelect)` (grid of buttons, selected = `HighImportance`), `sliderRow(label, min, max, step, format, value, onChanged, onCommitted)` (`Border(left=label 64 px, right=value 56 px, center=widget.Slider)`, using `OnChangeEnded` for commit), `section(title, note)`, `keyValue`, `mark(size)` (hexagon as a small SVG `StaticResource`).
- `chrome.go`, `sidebar.go`, `controls3d.go`, `controls2d.go`: one function per roxel group (names follow roxel: `studySection`, `detailsSection`, `volumesSection`, `findingsSection`, `volumeHeader`, `windowingGroup3D`, `paletteGroup`, `lightingRows`, `sliceGroup`, `windowingGroup2D`, `imageGroup`). Desktop root: `Border(top=appBar, bottom=statusBar, left=sidebar, right=slicePanel, center=Border(top=volumeHeader, bottom=shelf, center=volumeRenderer))` inside the responsive layout, which sets sidebar = 232 px and slice panel = `clamp(0.36w, 336, min(520, w-232-532))`.
- `volume.go` additions: `LookAlong(axis int)` (X: rotation (pi/2, 0); Y: (0, pi/2 - 0.01); Z: (0, 0); then reset pan), `FitView()` (existing `ResetView` plus pan reset, with zoom chosen so the scaled box fits), Shift-to-pan in `Dragged` via `fyne.CurrentApp().Driver().(desktop.Driver)` when the assertion succeeds, Quality -> `SetParams` step size 0.004 / 0.002 / 0.001. Cutaway: `SetClippingPlane` with the unit axis of the 2D orientation and `d = -(sliceIndex + 0.5) / sliceCount`, sign flipped for reverse; disabled with the note when composite is on. `VolumeRenderer.Align` flips X/Y/Z, so confirm the kept side visually and invert once if needed.
- Density becomes its own slider; the first band handle stops driving `SetDensityThreshold`.
- Composite defaults to off; default orientation Axial (roxel's default).
- Remove `createSideBySidePanel`, `setupForm`'s accordion wiring and the widget fields on `Viewer` that the new constructors replace. `pkg/goxel/layout/manager.go` is unused today; delete it only if nothing imports it after this step.
- **Checks:** `go test -short ./pkg/...`; add table tests for the pure parts (`layoutFromSize` with roxel's six cases: 1440x900, 1100x720, 393x852, 852x393, 390x300, 1024x768; `slicePanelWidth`; cutaway plane for each orientation). Run desktop at 1440 x 900 next to `cargo run -p roxel` with a comparable CT: same regions in the same places, same colours (pick BG/PANEL/ACCENT pixels and compare to the table), every control in section 1.1's desktop table present or listed as Out/P2. Each control changes the image it should; Fit view and X/Y/Z work; cutaway hides one side and Reverse the other; errors from a non-DICOM file appear in the sidebar, not a dialog; the window stays responsive while a file loads.

### Step 6 — Focus modes and keyboard
- Focus volume / Focus slice buttons and layouts; Esc exits (ignore when a popup is showing or an `Entry` has focus); arrow keys still step slices; status bar text follows roxel's `navigation_text`.
- **Checks:** each focus mode fills the workspace and returns with the button and with Esc; in Focus volume the plane picker and slice slider appear in the header and move the cutaway.

### Step 7 — Compact layouts and touch
- `responsive.go`: the root layout's `Layout(objects, size)` computes the class; when it differs from `state.layout` it schedules `rebuild()` with `go fyne.Do(...)` (never mutate the tree inside `Layout`). Every sub-tree must have a small `MinSize` (wrap long labels with truncation; no fixed 300 x 300 minimum on the 3D widget in compact mode) or the canvas will not shrink — this is what breaks the current UI at 390 px.
- `phone.go`: tab bar (grid of four buttons, 49 px, active = ACCENT text + 3 px top bar, findings badge), fixed-size paged sheets built from the Step 5 group functions, pages exactly as section 1.1; Camera page with Fit view, Focus volume, X/Y/Z, Orbit|Pan toggle + four arrow buttons (orbit 15 degrees, pan 80 px) and zoom -/+ (x1.25); compact focus bar. In compact layouts `sliderRow` renders the touch variant (44 px, -/+ steppers). `MultiRangeSlider`: handle radius 11 and height 96 when compact.
- `index.html` touch shim: on `touchstart/move/end` with one touch, `preventDefault()` and dispatch `mousedown/mousemove/mouseup` `MouseEvent`s on `document` with the touch's `clientX/clientY` and `button: 0`; with two touches, dispatch `wheel` events whose `deltaY` follows the change in finger distance. **[unverified]** that Fyne accepts synthetic events — its handlers read plain event properties and do not check `isTrusted`, so it should; verify first, before building the rest of this step. If it does not work, the tap controls on the Camera page and the steppers remain the touch story, and that limitation goes in the README.
- **Checks:** unit tests for the tab/sheet state transitions (tap other tab -> opens with sheet; tap active view tab -> toggles sheet). In the browser's device mode at 393 x 852 and 852 x 393: no clipped content, no `glScissor` warnings, all four tabs work, each sheet page fits without scrolling, the 3D viewport is at least 300 x 240. With touch emulation on: one-finger drag orbits, slider thumbs drag, pinch zooms. On a real phone if available.

### Step 8 — Deploy and document
- README: browser demo link, `make build-wasm` / `make serve-wasm`, known web limits (single-file load, no folder open, large studies downsampled, download size). Update CLAUDE.md's file list for `pkg/goxel` and `pkg/volume`.
- **Checks:** Pages workflow green on main; the public URL loads the sample and passes the Step 4 and Step 7 browser checks; CI workflow green.

### Step 9 — Parity polish (each item independent)
- Depth cue uniform + slider (shared shader body, both back ends).
- WebGL2 line pass: finding boxes and axis gizmo.
- Slice-plane outline and corner readout in the 3D view.
- 2D: Ctrl+scroll zoom, scale bar in the footer.
- Palette legend refinements (two-line band legend, grey ramp with window values).

---

## 5. Risks

| Risk | Likelihood | Effect | Mitigation |
|---|---|---|---|
| Synthetic touch-to-mouse events not honoured by Fyne, or awkward on iOS Safari (scroll/zoom of the page itself) | medium | no drag gestures on phones | test first in Step 7; `touch-action:none` and `preventDefault`; tap controls already cover every action |
| WebGL2 half-float volume looks different from desktop RGBA16 (banding in soft gradients) | low-medium | visible mismatch | signed gradient storage (2.2); compare screenshots in Step 4 |
| Shared shader body changes desktop output | low | desktop regression | before/after screenshot check in Step 4; fall back to a copied body |
| `input.click()` from a goroutine rejected by Safari's activation rules | low-medium | "Open file…" does nothing in Safari | check in Step 3; HTML overlay button fallback |
| Fyne retained-mode rebuilds lose widget state or flicker on resize across a breakpoint | medium | janky resize | all state in `uiState`; rebuild only on class change, not on every resize |
| Fyne MinSize floors keep the canvas wider than a phone | medium | clipped compact layout | explicit acceptance check at 393 px; audit `MinSize` of custom widgets (`VolumeRenderer` returns 300 x 300 today) |
| 40 MB wasm, 17 MB sample | certain | slow first load | loading line with size in `index.html`; optional JPEG-LS sample later |
| Go wasm heap grows and never shrinks; large uploads may exhaust memory | medium for large studies | tab crash | per-slice upload; 32 M voxel cap; web loads one file |
| `readPixels` each frame stalls on some GPUs | low | low frame rate | 768 px target, 512 px on Fast |
| Vendor conflicts with the parallel jpegs bump | medium | merge churn | do Steps 1-2 first and together |
| Pages unavailable (private repo on a free plan) | unknown | no public demo | owner confirms visibility before Step 8 |
| Removing Z Scale / BG Color / LAPTOP_REMOVAL from the UI is unwanted | unknown | lost controls | listed for confirmation under the gap table; trivial to keep |

## 6. Questions for the owner (answers change scope, not the approach)

1. Share the fragment shader body between desktop and web (recommended), or leave the desktop shader untouched and duplicate it?
2. Drop Z Scale, BG Color and the LAPTOP_REMOVAL preset from the UI, or keep them?
3. Is TDR/detections support wanted in goxel at all? It is the one roxel sidebar section with no data behind it here.
4. Is the repo public (Pages), and should the demo auto-load the sample as roxel does (17 MB on every first visit)?
5. Keep "DICOM study" wording and "Findings" (proposed, since goxel is a DICOM viewer), or adopt roxel's "DICOS" / "Detections"?
