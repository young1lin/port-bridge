package ui

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

//go:embed spinner/spinner_frame_*.svg
var spinnerFS embed.FS

// spinnerFrames holds pre-loaded SVG resources for all 30 animation frames (0..348 degrees, step 12).
// Initialized once at startup; animate() only indexes into this array — no runtime allocations.
var spinnerFrames [30]*fyne.StaticResource

func init() {
	for i := 0; i < 30; i++ {
		deg := i * 12
		name := fmt.Sprintf("spinner/spinner_frame_%03d.svg", deg)
		data, err := spinnerFS.ReadFile(name)
		if err != nil {
			log.Printf("[ERROR] Failed to load spinner frame %s: %v", name, err)
			continue
		}
		spinnerFrames[i] = fyne.NewStaticResource(fmt.Sprintf("dots_%d", deg), data)
	}
}

// Result is the outcome returned by a LoadingOverlay's task.
type Result int

const (
	ResultSuccess Result = iota
	ResultError
	ResultTimeout
)

// LoadingOverlay shows a modal dialog with a dot spinner and text.
// It blocks all interaction until Dismiss is called or the timeout expires.
type LoadingOverlay struct {
	dialog   *dialog.CustomDialog
	spinner  *dotSpinner
	label    *widget.Label
	done     chan Result
	stop     chan struct{}
	mu       sync.Mutex
	finished bool
}

// NewLoadingOverlay creates and shows a loading dialog on the given window.
func NewLoadingOverlay(w fyne.Window, title string, text string, timeout time.Duration) *LoadingOverlay {
	log.Printf("[DEBUG] LoadingOverlay: creating, title=%s, text=%s, timeout=%v", title, text, timeout)
	lo := &LoadingOverlay{
		done: make(chan Result, 1),
		stop: make(chan struct{}),
	}

	lo.spinner = newDotSpinner()
	log.Printf("[DEBUG] LoadingOverlay: spinner created")
	lo.label = widget.NewLabel(text)
	lo.label.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(
		container.NewCenter(lo.spinner),
		container.NewCenter(lo.label),
	)

	lo.dialog = dialog.NewCustomWithoutButtons(title, content, w)
	log.Printf("[DEBUG] LoadingOverlay: dialog created, calling Show()")
	lo.dialog.Show()
	log.Printf("[DEBUG] LoadingOverlay: dialog shown")

	if timeout > 0 {
		go func() {
			select {
			case <-time.After(timeout):
				log.Printf("[DEBUG] LoadingOverlay: timeout fired")
				lo.Dismiss(ResultTimeout)
			case <-lo.stop:
				log.Printf("[DEBUG] LoadingOverlay: stop received")
			}
		}()
	}

	return lo
}

// SetText updates the label text.
func (lo *LoadingOverlay) SetText(text string) {
	if lo.label != nil {
		lo.label.SetText(text)
	}
}

// Wait blocks until the overlay is dismissed.
func (lo *LoadingOverlay) Wait() Result {
	return <-lo.done
}

// Dismiss hides the dialog. Safe to call multiple times.
func (lo *LoadingOverlay) Dismiss(result Result) {
	log.Printf("[DEBUG] LoadingOverlay: Dismiss called, result=%v", result)
	lo.mu.Lock()
	defer lo.mu.Unlock()
	if lo.finished {
		log.Printf("[DEBUG] LoadingOverlay: already finished, skip")
		return
	}
	lo.finished = true
	close(lo.stop)
	lo.spinner.Stop()
	log.Printf("[DEBUG] LoadingOverlay: hiding dialog")
	fyne.Do(func() {
		lo.dialog.Hide()
	})
	log.Printf("[DEBUG] LoadingOverlay: sending to done channel")
	lo.done <- result
	log.Printf("[DEBUG] LoadingOverlay: Dismiss complete")
}

// RunWithLoading shows a loading dialog, runs task, calls done on finish/timeout.
func RunWithLoading(w fyne.Window, title string, text string, timeout time.Duration, task func(ctx context.Context) error, done func(Result, error)) {
	log.Printf("[DEBUG] RunWithLoading: starting")
	lo := NewLoadingOverlay(w, title, text, timeout)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] RunWithLoading: panic recovered: %v", r)
			}
		}()
		log.Printf("[DEBUG] RunWithLoading: task starting")
		err := task(ctx)
		cancel()
		log.Printf("[DEBUG] RunWithLoading: task returned err=%v", err)

		if err != nil {
			lo.Dismiss(ResultError)
		} else {
			lo.Dismiss(ResultSuccess)
		}

		result := lo.Wait()
		log.Printf("[DEBUG] RunWithLoading: Wait returned result=%v", result)
		if done != nil {
			log.Printf("[DEBUG] RunWithLoading: calling done callback")
			done(result, err)
		}
	}()
}

// dotSpinner renders 8 dots in a circle with fading opacity, animated by rotating in Go.
type dotSpinner struct {
	widget.BaseWidget
	img  *canvas.Image
	stop chan struct{}
	deg  int
}

func newDotSpinner() *dotSpinner {
	s := &dotSpinner{stop: make(chan struct{})}
	s.ExtendBaseWidget(s)
	initialFrame := spinnerFrames[0]
	if initialFrame == nil {
		initialFrame = fyne.NewStaticResource("dots_0_fallback", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"/>`))
	}
	s.img = canvas.NewImageFromResource(initialFrame)
	s.img.FillMode = canvas.ImageFillContain
	go s.animate()
	return s
}

func (s *dotSpinner) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *dotSpinner) animate() {
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.deg = (s.deg + 12) % 360
			idx := s.deg / 12
			if idx >= 0 && idx < 30 && spinnerFrames[idx] != nil {
				fyne.Do(func() {
					s.img.Resource = spinnerFrames[idx]
					s.img.Refresh()
				})
			}
		}
	}
}

func (s *dotSpinner) CreateRenderer() fyne.WidgetRenderer {
	return &dotSpinnerRenderer{img: s.img}
}

type dotSpinnerRenderer struct {
	img *canvas.Image
}

func (r *dotSpinnerRenderer) Layout(size fyne.Size) {
	side := fyne.Min(size.Width, size.Height)
	r.img.Resize(fyne.NewSize(side, side))
	r.img.Move(fyne.NewPos((size.Width-side)/2, (size.Height-side)/2))
}

func (r *dotSpinnerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(40, 40)
}

func (r *dotSpinnerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.img}
}

func (r *dotSpinnerRenderer) Refresh() {
	r.img.Refresh()
}

func (r *dotSpinnerRenderer) Destroy() {}
