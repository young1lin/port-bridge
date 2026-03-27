package windowguard

import (
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

type lockState struct {
	count         int
	originalFixed bool
	targetCanvas  fyne.Size
	stopCh        chan struct{}
}

var (
	lockMu sync.Mutex
	locks  = map[fyne.Window]*lockState{}
)

// ProtectParentWindow keeps a parent window at its current size while a child
// window is shown. This works around a Windows/Fyne issue where opening a
// secondary window can trigger gradual parent growth.
func ProtectParentWindow(parent fyne.Window) func() {
	if parent == nil || runtime.GOOS != "windows" {
		return func() {}
	}

	lockMu.Lock()
	if existing := locks[parent]; existing != nil {
		existing.count++
		lockMu.Unlock()
		return func() { releaseParentWindow(parent) }
	}

	state := &lockState{
		count:         1,
		originalFixed: parent.FixedSize(),
		targetCanvas:  parent.Canvas().Size(),
		stopCh:        make(chan struct{}),
	}
	locks[parent] = state
	lockMu.Unlock()

	fyne.Do(func() {
		parent.SetFixedSize(true)
	})

	go holdParentWindow(parent, state)

	return func() { releaseParentWindow(parent) }
}

func holdParentWindow(parent fyne.Window, state *lockState) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-state.stopCh:
			return
		case <-ticker.C:
			fyne.Do(func() {
				size := parent.Canvas().Size()
				if size.Width > state.targetCanvas.Width || size.Height > state.targetCanvas.Height {
					parent.Resize(state.targetCanvas)
				}
			})
		}
	}
}

func releaseParentWindow(parent fyne.Window) {
	lockMu.Lock()
	state := locks[parent]
	if state == nil {
		lockMu.Unlock()
		return
	}
	state.count--
	if state.count > 0 {
		lockMu.Unlock()
		return
	}
	delete(locks, parent)
	close(state.stopCh)
	lockMu.Unlock()

	fyne.Do(func() {
		parent.SetFixedSize(state.originalFixed)
	})
}
