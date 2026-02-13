package gui

import "github.com/hajimehoshi/ebiten/v2"

// inputState tracks key/button state to detect edges (press/release).
type inputState struct {
	keys    map[ebiten.Key]bool
	buttons map[ebiten.MouseButton]bool
	lastX   int
	lastY   int
}

var gInput = &inputState{
	keys:    make(map[ebiten.Key]bool),
	buttons: make(map[ebiten.MouseButton]bool),
	lastX:   -1,
	lastY:   -1,
}

// forwardInput captures Ebiten keyboard/mouse events and forwards them
// to the active session. scaleX/scaleY/offsetX/offsetY map window coords to session coords.
func forwardInput(sess Session, scaleX, scaleY, offsetX, offsetY float64) {
	// Keyboard: detect press/release edges
	for key := ebiten.Key(0); key <= ebiten.KeyMax; key++ {
		pressed := ebiten.IsKeyPressed(key)
		was := gInput.keys[key]
		if pressed && !was {
			sess.HandleKeyPress(key)
		} else if !pressed && was {
			sess.HandleKeyRelease(key)
		}
		if pressed {
			gInput.keys[key] = true
		} else {
			delete(gInput.keys, key)
		}
	}

	if scaleX <= 0 || scaleY <= 0 {
		return
	}

	// Map window mouse position to session coordinates
	wx, wy := ebiten.CursorPosition()
	sx := int((float64(wx) - offsetX) / scaleX)
	sy := int((float64(wy) - offsetY) / scaleY)

	// Clamp to session bounds
	nw, nh := sess.NativeSize()
	if sx < 0 {
		sx = 0
	}
	if sy < 0 {
		sy = 0
	}
	if sx >= nw {
		sx = nw - 1
	}
	if sy >= nh {
		sy = nh - 1
	}

	// Only send move when position changes
	if sx != gInput.lastX || sy != gInput.lastY {
		sess.HandleMouseMove(sx, sy)
		gInput.lastX = sx
		gInput.lastY = sy
	}

	// Mouse buttons: detect press/release edges
	for _, btn := range []ebiten.MouseButton{
		ebiten.MouseButtonLeft,
		ebiten.MouseButtonRight,
		ebiten.MouseButtonMiddle,
	} {
		pressed := ebiten.IsMouseButtonPressed(btn)
		was := gInput.buttons[btn]
		if pressed && !was {
			sess.HandleMouseButton(btn, true)
		} else if !pressed && was {
			sess.HandleMouseButton(btn, false)
		}
		if pressed {
			gInput.buttons[btn] = true
		} else {
			delete(gInput.buttons, btn)
		}
	}

	// Mouse wheel
	dx, dy := ebiten.Wheel()
	if dx != 0 || dy != 0 {
		sess.HandleMouseWheel(dx, dy)
	}
}

// ebitenKeyToScancode maps Ebiten keys to RDP scancodes.
func ebitenKeyToScancode(key ebiten.Key) uint16 {
	scancodes := map[ebiten.Key]uint16{
		ebiten.KeyEscape:       0x01,
		ebiten.Key1:            0x02,
		ebiten.Key2:            0x03,
		ebiten.Key3:            0x04,
		ebiten.Key4:            0x05,
		ebiten.Key5:            0x06,
		ebiten.Key6:            0x07,
		ebiten.Key7:            0x08,
		ebiten.Key8:            0x09,
		ebiten.Key9:            0x0A,
		ebiten.Key0:            0x0B,
		ebiten.KeyMinus:        0x0C,
		ebiten.KeyEqual:        0x0D,
		ebiten.KeyBackspace:    0x0E,
		ebiten.KeyTab:          0x0F,
		ebiten.KeyQ:            0x10,
		ebiten.KeyW:            0x11,
		ebiten.KeyE:            0x12,
		ebiten.KeyR:            0x13,
		ebiten.KeyT:            0x14,
		ebiten.KeyY:            0x15,
		ebiten.KeyU:            0x16,
		ebiten.KeyI:            0x17,
		ebiten.KeyO:            0x18,
		ebiten.KeyP:            0x19,
		ebiten.KeyBracketLeft:  0x1A,
		ebiten.KeyBracketRight: 0x1B,
		ebiten.KeyEnter:        0x1C,
		ebiten.KeyControlLeft:  0x1D,
		ebiten.KeyA:            0x1E,
		ebiten.KeyS:            0x1F,
		ebiten.KeyD:            0x20,
		ebiten.KeyF:            0x21,
		ebiten.KeyG:            0x22,
		ebiten.KeyH:            0x23,
		ebiten.KeyJ:            0x24,
		ebiten.KeyK:            0x25,
		ebiten.KeyL:            0x26,
		ebiten.KeySemicolon:    0x27,
		ebiten.KeyQuote:        0x28,
		ebiten.KeyBackquote:    0x29,
		ebiten.KeyShiftLeft:    0x2A,
		ebiten.KeyBackslash:    0x2B,
		ebiten.KeyZ:            0x2C,
		ebiten.KeyX:            0x2D,
		ebiten.KeyC:            0x2E,
		ebiten.KeyV:            0x2F,
		ebiten.KeyB:            0x30,
		ebiten.KeyN:            0x31,
		ebiten.KeyM:            0x32,
		ebiten.KeyComma:        0x33,
		ebiten.KeyPeriod:       0x34,
		ebiten.KeySlash:        0x35,
		ebiten.KeyShiftRight:   0x36,
		ebiten.KeyAltLeft:      0x38,
		ebiten.KeySpace:        0x39,
		ebiten.KeyCapsLock:     0x3A,
		ebiten.KeyF1:           0x3B,
		ebiten.KeyF2:           0x3C,
		ebiten.KeyF3:           0x3D,
		ebiten.KeyF4:           0x3E,
		ebiten.KeyF5:           0x3F,
		ebiten.KeyF6:           0x40,
		ebiten.KeyF7:           0x41,
		ebiten.KeyF8:           0x42,
		ebiten.KeyF9:           0x43,
		ebiten.KeyF10:          0x44,
		ebiten.KeyF11:          0x57,
		ebiten.KeyF12:          0x58,
		ebiten.KeyHome:         0x47,
		ebiten.KeyArrowUp:      0x48,
		ebiten.KeyPageUp:       0x49,
		ebiten.KeyArrowLeft:    0x4B,
		ebiten.KeyArrowRight:   0x4D,
		ebiten.KeyEnd:          0x4F,
		ebiten.KeyArrowDown:    0x50,
		ebiten.KeyPageDown:     0x51,
		ebiten.KeyInsert:       0x52,
		ebiten.KeyDelete:       0x53,
		ebiten.KeyControlRight: 0x1D,
		ebiten.KeyAltRight:     0x38,
		ebiten.KeyMetaLeft:     0x5B,
		ebiten.KeyMetaRight:    0x5C,
	}
	if sc, ok := scancodes[key]; ok {
		return sc
	}
	return 0
}

// ebitenKeyToVNCKeysym maps Ebiten keys to X11 keysyms (used by VNC).
func ebitenKeyToVNCKeysym(key ebiten.Key) uint32 {
	keysyms := map[ebiten.Key]uint32{
		ebiten.KeyEscape:       0xFF1B,
		ebiten.KeyF1:           0xFFBE,
		ebiten.KeyF2:           0xFFBF,
		ebiten.KeyF3:           0xFFC0,
		ebiten.KeyF4:           0xFFC1,
		ebiten.KeyF5:           0xFFC2,
		ebiten.KeyF6:           0xFFC3,
		ebiten.KeyF7:           0xFFC4,
		ebiten.KeyF8:           0xFFC5,
		ebiten.KeyF9:           0xFFC6,
		ebiten.KeyF10:          0xFFC7,
		ebiten.KeyF11:          0xFFC8,
		ebiten.KeyF12:          0xFFC9,
		ebiten.KeyBackspace:    0xFF08,
		ebiten.KeyTab:          0xFF09,
		ebiten.KeyEnter:        0xFF0D,
		ebiten.KeyShiftLeft:    0xFFE1,
		ebiten.KeyShiftRight:   0xFFE2,
		ebiten.KeyControlLeft:  0xFFE3,
		ebiten.KeyAltLeft:      0xFFE9,
		ebiten.KeyCapsLock:     0xFFE5,
		ebiten.KeySpace:        0x0020,
		ebiten.KeyInsert:       0xFF63,
		ebiten.KeyDelete:       0xFFFF,
		ebiten.KeyHome:         0xFF50,
		ebiten.KeyEnd:          0xFF57,
		ebiten.KeyPageUp:       0xFF55,
		ebiten.KeyPageDown:     0xFF56,
		ebiten.KeyArrowUp:      0xFF52,
		ebiten.KeyArrowDown:    0xFF54,
		ebiten.KeyArrowLeft:    0xFF51,
		ebiten.KeyArrowRight:   0xFF53,
		ebiten.KeyA:            0x0061,
		ebiten.KeyB:            0x0062,
		ebiten.KeyC:            0x0063,
		ebiten.KeyD:            0x0064,
		ebiten.KeyE:            0x0065,
		ebiten.KeyF:            0x0066,
		ebiten.KeyG:            0x0067,
		ebiten.KeyH:            0x0068,
		ebiten.KeyI:            0x0069,
		ebiten.KeyJ:            0x006A,
		ebiten.KeyK:            0x006B,
		ebiten.KeyL:            0x006C,
		ebiten.KeyM:            0x006D,
		ebiten.KeyN:            0x006E,
		ebiten.KeyO:            0x006F,
		ebiten.KeyP:            0x0070,
		ebiten.KeyQ:            0x0071,
		ebiten.KeyR:            0x0072,
		ebiten.KeyS:            0x0073,
		ebiten.KeyT:            0x0074,
		ebiten.KeyU:            0x0075,
		ebiten.KeyV:            0x0076,
		ebiten.KeyW:            0x0077,
		ebiten.KeyX:            0x0078,
		ebiten.KeyY:            0x0079,
		ebiten.KeyZ:            0x007A,
		ebiten.Key0:            0x0030,
		ebiten.Key1:            0x0031,
		ebiten.Key2:            0x0032,
		ebiten.Key3:            0x0033,
		ebiten.Key4:            0x0034,
		ebiten.Key5:            0x0035,
		ebiten.Key6:            0x0036,
		ebiten.Key7:            0x0037,
		ebiten.Key8:            0x0038,
		ebiten.Key9:            0x0039,
		ebiten.KeyMinus:        0x002D,
		ebiten.KeyEqual:        0x003D,
		ebiten.KeyBracketLeft:  0x005B,
		ebiten.KeyBracketRight: 0x005D,
		ebiten.KeyBackslash:    0x005C,
		ebiten.KeySemicolon:    0x003B,
		ebiten.KeyQuote:        0x0027,
		ebiten.KeyBackquote:    0x0060,
		ebiten.KeyComma:        0x002C,
		ebiten.KeyPeriod:       0x002E,
		ebiten.KeySlash:        0x002F,
		ebiten.KeyControlRight: 0xFFE4,
		ebiten.KeyAltRight:     0xFFEA,
		ebiten.KeyMetaLeft:     0xFFEB,
		ebiten.KeyMetaRight:    0xFFEC,
	}
	if ks, ok := keysyms[key]; ok {
		return ks
	}
	return 0
}

// isExtendedScancode returns true for keys that require the RDP extended
// scancode flag (KBDFLAGS_EXTENDED). These are keys on the enhanced keyboard
// that share scancodes with the numeric keypad.
func isExtendedScancode(key ebiten.Key) bool {
	switch key {
	case ebiten.KeyInsert, ebiten.KeyDelete,
		ebiten.KeyHome, ebiten.KeyEnd,
		ebiten.KeyPageUp, ebiten.KeyPageDown,
		ebiten.KeyArrowUp, ebiten.KeyArrowDown,
		ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
		ebiten.KeyControlRight, ebiten.KeyAltRight,
		ebiten.KeyMetaLeft, ebiten.KeyMetaRight:
		return true
	}
	return false
}
