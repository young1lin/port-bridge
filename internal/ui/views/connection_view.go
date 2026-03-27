package views

import (
	"fmt"
	"image/color"
	"log"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/young1lin/port-bridge/internal/i18n"
)

var showConnectionConfirmDialog = dialog.ShowConfirm

// rowPad is the vertical padding for list rows to avoid bottom clipping.
const rowPad = float32(2)

// statusDotCache caches SVG resources by color key to avoid repeated formatting.
var statusDotCache = struct {
	sync.RWMutex
	resources map[string]*fyne.StaticResource
}{
	resources: make(map[string]*fyne.StaticResource),
}

// getOrCreateStatusDotResource returns a cached SVG resource for the given color.
func getOrCreateStatusDotResource(c color.Color) *fyne.StaticResource {
	r, g, b, _ := c.RGBA()
	key := fmt.Sprintf("dot_%d_%d_%d", r>>8, g>>8, b>>8)

	// Check cache first (read lock)
	statusDotCache.RLock()
	if res, ok := statusDotCache.resources[key]; ok {
		statusDotCache.RUnlock()
		return res
	}
	statusDotCache.RUnlock()

	// Create new resource (write lock)
	statusDotCache.Lock()
	defer statusDotCache.Unlock()

	// Double-check after acquiring write lock
	if res, ok := statusDotCache.resources[key]; ok {
		return res
	}

	fill := fmt.Sprintf("rgb(%d,%d,%d)", r>>8, g>>8, b>>8)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><circle cx="10" cy="10" r="8" fill="%s"/></svg>`, fill)
	res := fyne.NewStaticResource(key, []byte(svg))
	statusDotCache.resources[key] = res
	return res
}

// statusDot is a widget that renders a filled circle via SVG.
type statusDot struct {
	widget.BaseWidget
	fill color.Color
	img  *canvas.Image
}

func newStatusDot() *statusDot {
	d := &statusDot{fill: color.Gray{}}
	d.ExtendBaseWidget(d)
	d.img = canvas.NewImageFromResource(getOrCreateStatusDotResource(d.fill))
	d.img.FillMode = canvas.ImageFillContain
	return d
}

func (d *statusDot) SetFill(c color.Color) {
	d.fill = c
	d.img.Resource = getOrCreateStatusDotResource(c)
	d.img.Refresh()
}

func (d *statusDot) CreateRenderer() fyne.WidgetRenderer {
	return &statusDotRenderer{img: d.img}
}

type statusDotRenderer struct {
	img *canvas.Image
}

func (r *statusDotRenderer) Layout(size fyne.Size) {
	r.img.Resize(size)
	r.img.Move(fyne.NewPos(0, 0))
}

func (r *statusDotRenderer) MinSize() fyne.Size {
	return fyne.NewSize(14, 14)
}

func (r *statusDotRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.img}
}

func (r *statusDotRenderer) Refresh() {
	r.img.Refresh()
}

func (r *statusDotRenderer) Destroy() {}

// ConnectionView displays the list of SSH connections
type ConnectionView struct {
	app         fyne.App
	window      fyne.Window
	container   *fyne.Container
	list        *widget.List
	statusLabel *widget.Label
	data        []ConnectionItem
	newBtn      *widget.Button
	onEdit      func(id string)
	onDelete    func(id string)
	onTest      func(id string)
}

// ConnectionItem represents a single connection in the list
type ConnectionItem struct {
	ID          string
	Name        string
	Address     string
	IsConnected bool
	StatusIcon  string
}

// NewConnectionView creates a new connection view
func NewConnectionView(app fyne.App, window fyne.Window) *ConnectionView {
	cv := &ConnectionView{
		app:    app,
		window: window,
		data:   make([]ConnectionItem, 0),
	}

	cv.setupUI()
	cv.registerLanguageChange()
	return cv
}

// setupUI sets up the UI components
func (cv *ConnectionView) setupUI() {
	cv.list = widget.NewList(
		func() int { return len(cv.data) },
		func() fyne.CanvasObject {
			return cv.createRow()
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			cv.updateRow(id, obj)
		},
	)

	// Toolbar with icon button
	cv.newBtn = widget.NewButtonWithIcon(i18n.L("New"), theme.ContentAddIcon(), func() {
		if cv.onEdit != nil {
			cv.onEdit("")
		}
	})
	cv.newBtn.Importance = widget.HighImportance

	// Top bar: toolbar + separator
	topBar := container.NewVBox(
		container.NewHBox(cv.newBtn),
		widget.NewSeparator(),
	)

	// Bottom bar: separator + right-aligned status
	cv.statusLabel = widget.NewLabel("")
	bottomBar := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(
			layout.NewSpacer(),
			cv.statusLabel,
		),
	)

	cv.container = container.NewBorder(topBar, bottomBar, nil, nil, cv.list)
}

// registerLanguageChange registers a callback to refresh UI on language change.
func (cv *ConnectionView) registerLanguageChange() {
	i18n.OnLanguageChange(func() {
		cv.newBtn.SetText(i18n.L("New"))
		cv.list.Refresh()
		cv.refreshStatusText()
	})
}

// refreshStatusText updates the status label using current language and data.
func (cv *ConnectionView) refreshStatusText() {
	connectedCount := 0
	for _, item := range cv.data {
		if item.IsConnected {
			connectedCount++
		}
	}
	cv.statusLabel.SetText(fmt.Sprintf(i18n.L("%d connections, %d connected"), len(cv.data), connectedCount))
}

// createRow creates a row template
func (cv *ConnectionView) createRow() *fyne.Container {
	// Status dot (SVG circle)
	statusDot := newStatusDot()

	// Name (fixed width, truncate on overflow)
	nameLabel := widget.NewLabel(i18n.L("Connection Name"))
	nameLabel.Truncation = fyne.TextTruncateEllipsis
	nameWrapper := container.New(&fixedWidthLayout{width: 150}, nameLabel)

	// Address
	addrLabel := widget.NewLabel("host:port")

	// Buttons with icons
	testBtn := widget.NewButtonWithIcon(i18n.L("Test"), theme.SearchIcon(), nil)
	editBtn := widget.NewButtonWithIcon(i18n.L("Edit"), theme.DocumentCreateIcon(), nil)
	deleteBtn := widget.NewButtonWithIcon(i18n.L("Delete"), theme.DeleteIcon(), nil)
	deleteBtn.Importance = widget.DangerImportance

	// Left: status dot + name
	leftBox := container.NewHBox(statusDot, nameWrapper)

	// Right: button group
	rightBox := container.NewHBox(testBtn, editBtn, deleteBtn)

	// Top/bottom transparent padding to prevent bottom clipping in widget.List
	topSpacer := canvas.NewRectangle(color.Transparent)
	topSpacer.SetMinSize(fyne.NewSize(0, rowPad))
	botSpacer := canvas.NewRectangle(color.Transparent)
	botSpacer.SetMinSize(fyne.NewSize(0, rowPad))

	return container.NewBorder(topSpacer, botSpacer, leftBox, rightBox, addrLabel)
}

// updateRow updates a row with data
func (cv *ConnectionView) updateRow(id widget.ListItemID, obj fyne.CanvasObject) {
	if id >= len(cv.data) {
		return
	}

	item := cv.data[id]
	border, ok := obj.(*fyne.Container)
	if !ok {
		log.Printf("[ERROR] conn row obj is not *fyne.Container: %T", obj)
		return
	}

	var leftBox, rightBox *fyne.Container
	var addrLabel *widget.Label

	for _, o := range border.Objects {
		if c, ok := o.(*fyne.Container); ok && len(c.Objects) > 0 {
			switch c.Objects[0].(type) {
			case *fyne.Container, *statusDot:
				leftBox = c
			case *widget.Button:
				rightBox = c
			}
		} else if label, ok := o.(*widget.Label); ok {
			addrLabel = label
		}
	}

	// Update left: status dot + name
	if leftBox != nil && len(leftBox.Objects) >= 2 {
		// Status dot color
		if dot, ok := leftBox.Objects[0].(*statusDot); ok {
			if item.IsConnected {
				dot.SetFill(color.RGBA{R: 76, G: 175, B: 80, A: 255})
			} else {
				dot.SetFill(color.RGBA{R: 158, G: 158, B: 158, A: 255})
			}
		}
		// Name
		if wrapper, ok := leftBox.Objects[1].(*fyne.Container); ok && len(wrapper.Objects) > 0 {
			if label, ok := wrapper.Objects[0].(*widget.Label); ok {
				label.SetText(item.Name)
			}
		}
	}

	// Update address
	if addrLabel != nil {
		addrLabel.SetText(item.Address)
	}

	// Update buttons
	if rightBox != nil && len(rightBox.Objects) >= 3 {
		testBtn, ok1 := rightBox.Objects[0].(*widget.Button)
		editBtn, ok2 := rightBox.Objects[1].(*widget.Button)
		deleteBtn, ok3 := rightBox.Objects[2].(*widget.Button)
		if !ok1 || !ok2 || !ok3 {
			log.Printf("[ERROR] connection row button type assertion failed")
			return
		}

		// Update button labels for current language
		testBtn.SetText(i18n.L("Test"))
		editBtn.SetText(i18n.L("Edit"))
		deleteBtn.SetText(i18n.L("Delete"))

		testBtn.OnTapped = func() {
			if cv.onTest != nil {
				cv.onTest(item.ID)
			}
		}
		editBtn.OnTapped = func() {
			if cv.onEdit != nil {
				cv.onEdit(item.ID)
			}
		}
		deleteBtn.OnTapped = func() {
			if cv.onDelete != nil {
				cv.onDelete(item.ID)
			}
		}
	}
}

// Container returns the view container
func (cv *ConnectionView) Container() fyne.CanvasObject {
	return cv.container
}

// SetData sets the connection data
func (cv *ConnectionView) SetData(data []ConnectionItem) {
	cv.data = data
	cv.list.Refresh()

	connectedCount := 0
	for _, item := range data {
		if item.IsConnected {
			connectedCount++
		}
	}
	cv.statusLabel.SetText(fmt.Sprintf(i18n.L("%d connections, %d connected"), len(data), connectedCount))
}

// SetOnEdit sets the edit callback
func (cv *ConnectionView) SetOnEdit(callback func(id string)) {
	cv.onEdit = callback
}

// SetOnDelete sets the delete callback
func (cv *ConnectionView) SetOnDelete(callback func(id string)) {
	cv.onDelete = callback
}

// SetOnTest sets the test callback
func (cv *ConnectionView) SetOnTest(callback func(id string)) {
	cv.onTest = callback
}

// ShowConfirm shows a confirmation dialog
func (cv *ConnectionView) ShowConfirm(title, message string, callback func(bool)) {
	showConnectionConfirmDialog(title, message, callback, cv.window)
}
