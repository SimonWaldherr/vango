package vango

import (
	"image"
)

// History provides undo/redo capability for a Canvas.
type History struct {
	states  []canvasState
	current int // points to current state
	maxSize int // maximum number of states to keep
}

type canvasState struct {
	Description string
	Layers      []layerSnapshot
}

type layerSnapshot struct {
	Name      string
	ImageData []byte // raw Pix data
	Bounds    image.Rectangle
	Blend     BlendMode
	Opacity   float64
	Visible   bool
	OffsetX   int
	OffsetY   int
	MaskData  []byte
	MaskRect  image.Rectangle
	ZOrder    int
	Locked    bool
	ClipGroup bool
	Group     string
}

// NewHistory creates a history tracker with the given max number of undo steps.
func NewHistory(maxUndo int) *History {
	if maxUndo < 1 {
		maxUndo = 50
	}
	return &History{
		maxSize: maxUndo,
		current: -1,
	}
}

// SaveState captures the current canvas state.
func (h *History) SaveState(c *Canvas, description string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := canvasState{Description: description}
	for _, l := range c.Layers {
		snap := layerSnapshot{
			Name:      l.Name,
			Bounds:    l.Image.Rect,
			Blend:     l.Blend,
			Opacity:   l.Opacity,
			Visible:   l.Visible,
			OffsetX:   l.OffsetX,
			OffsetY:   l.OffsetY,
			ZOrder:    l.ZOrder,
			Locked:    l.Locked,
			ClipGroup: l.ClipGroup,
			Group:     l.Group,
		}
		// Copy pixel data
		snap.ImageData = make([]byte, len(l.Image.Pix))
		copy(snap.ImageData, l.Image.Pix)

		if l.Mask != nil {
			snap.MaskData = make([]byte, len(l.Mask.Pix))
			copy(snap.MaskData, l.Mask.Pix)
			snap.MaskRect = l.Mask.Rect
		}
		state.Layers = append(state.Layers, snap)
	}

	// Truncate future states if we're not at the end
	if h.current < len(h.states)-1 {
		h.states = h.states[:h.current+1]
	}

	h.states = append(h.states, state)
	h.current = len(h.states) - 1

	// Trim oldest if over limit
	if len(h.states) > h.maxSize {
		excess := len(h.states) - h.maxSize
		h.states = h.states[excess:]
		h.current -= excess
	}
}

// Undo restores the previous state. Returns false if nothing to undo.
func (h *History) Undo(c *Canvas) bool {
	if h.current <= 0 {
		return false
	}
	h.current--
	h.restoreState(c, h.states[h.current])
	return true
}

// Redo moves forward in history. Returns false if nothing to redo.
func (h *History) Redo(c *Canvas) bool {
	if h.current >= len(h.states)-1 {
		return false
	}
	h.current++
	h.restoreState(c, h.states[h.current])
	return true
}

// CanUndo returns true if undo is available.
func (h *History) CanUndo() bool {
	return h.current > 0
}

// CanRedo returns true if redo is available.
func (h *History) CanRedo() bool {
	return h.current < len(h.states)-1
}

// UndoCount returns the number of available undo steps.
func (h *History) UndoCount() int {
	return h.current
}

// RedoCount returns the number of available redo steps.
func (h *History) RedoCount() int {
	return len(h.states) - 1 - h.current
}

func (h *History) restoreState(c *Canvas, state canvasState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Layers = make([]*Layer, len(state.Layers))
	for i, snap := range state.Layers {
		img := image.NewNRGBA(snap.Bounds)
		copy(img.Pix, snap.ImageData)

		l := &Layer{
			Name:      snap.Name,
			Image:     img,
			Blend:     snap.Blend,
			Opacity:   snap.Opacity,
			Visible:   snap.Visible,
			OffsetX:   snap.OffsetX,
			OffsetY:   snap.OffsetY,
			ZOrder:    snap.ZOrder,
			Locked:    snap.Locked,
			ClipGroup: snap.ClipGroup,
			Group:     snap.Group,
		}
		if snap.MaskData != nil {
			mask := image.NewGray(snap.MaskRect)
			copy(mask.Pix, snap.MaskData)
			l.Mask = mask
		}
		c.Layers[i] = l
	}
}
