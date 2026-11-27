package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Trusted keyboard and mouse input for scenarios about the browser's own
// behaviour (copy, paste, text selection, the context menu): a CDP input event
// is a real one, so Ctrl+C copies the selection, Ctrl+V and Shift+Insert fire
// the paste event with the clipboard's text, and a drag selects text. A
// synthetic DOM event does none of that.

// keyInfo: a key's DOM key, code and Windows virtual key code.
type keyInfo struct {
	key, code string
	vk        int
}

// parseCombo reads "ctrl+c", "shift+insert", "meta+v", "enter", "a" into
// CDP modifier bits (Alt 1, Ctrl 2, Meta 4, Shift 8) and the key.
func parseCombo(combo string) (mods int, k keyInfo, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(combo)), "+")
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "alt":
			mods |= 1
		case "ctrl", "control":
			mods |= 2
		case "meta", "cmd":
			mods |= 4
		case "shift":
			mods |= 8
		default:
			return 0, k, fmt.Errorf("key combo %q: unknown modifier %q", combo, m)
		}
	}
	name := parts[len(parts)-1]
	named := map[string]keyInfo{
		"enter": {"Enter", "Enter", 13}, "insert": {"Insert", "Insert", 45}, "escape": {"Escape", "Escape", 27},
		"backspace": {"Backspace", "Backspace", 8}, "tab": {"Tab", "Tab", 9}, "space": {" ", "Space", 32},
	}
	switch {
	case named[name].code != "":
		k = named[name]
	case len(name) == 1 && name[0] >= 'a' && name[0] <= 'z':
		k = keyInfo{name, "Key" + strings.ToUpper(name), int(name[0] - 'a' + 'A')}
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		k = keyInfo{name, "Digit" + name, int(name[0])}
	default:
		return 0, k, fmt.Errorf("key combo %q: unknown key %q", combo, name)
	}
	if mods&8 != 0 && len(k.key) == 1 {
		k.key = strings.ToUpper(k.key)
	}
	return mods, k, nil
}

// keys presses and releases each combo in turn, as trusted key events. A
// plain printable key types its character; one with Ctrl/Meta/Alt does not
// (the browser's shortcut for it runs: copy, paste, select all).
func (p *page) keys(combos []string) error {
	for _, combo := range combos {
		mods, k, err := parseCombo(combo)
		if err != nil {
			return err
		}
		down := map[string]any{"type": "rawKeyDown", "modifiers": mods, "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "nativeVirtualKeyCode": k.vk}
		text := ""
		if mods&7 == 0 && len(k.key) == 1 {
			text = k.key
		} else if k.code == "Enter" && mods&7 == 0 {
			text = "\r"
		}
		if text != "" {
			down["type"] = "keyDown"
			down["text"] = text
			down["unmodifiedText"] = text
		}
		if err := p.b.call(p.session, "Input.dispatchKeyEvent", down, nil); err != nil {
			return err
		}
		up := map[string]any{"type": "keyUp", "modifiers": mods, "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "nativeVirtualKeyCode": k.vk}
		if err := p.b.call(p.session, "Input.dispatchKeyEvent", up, nil); err != nil {
			return err
		}
	}
	return nil
}

// rect: the bounding box (viewport px) of the first element matching sel,
// where it comes to rest. CSS transitions run on the browser's wall clock,
// not the page's virtual one (the console slides in over 140 ms), so they are
// finished first: otherwise the box is read mid-slide and the press lands where
// the element was a moment ago (a drag that selects nothing, on a loaded runner).
func (p *page) rect(sel string) (x0, y0, x1, y1 float64, err error) {
	q, _ := json.Marshal(sel)
	var r []float64
	expr := fmt.Sprintf(`(() => { for (const a of document.getAnimations()) if (a instanceof CSSTransition) a.finish(); const e = document.querySelector(%s); if (!e) return null; const b = e.getBoundingClientRect(); return [b.left, b.top, b.right, b.bottom]; })()`, q)
	if err := p.eval(expr, &r); err != nil {
		return 0, 0, 0, 0, err
	}
	if len(r) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("no element %s", sel)
	}
	return r[0], r[1], r[2], r[3], nil
}

// drag presses the left button near the top-left corner of the element, moves
// to near its bottom-right corner in a few steps and releases there: a mouse
// text selection across it.
func (p *page) drag(sel string) error {
	x0, y0, x1, y1, err := p.rect(sel)
	if err != nil {
		return err
	}
	fx, fy, tx, ty := x0+2, y0+2, x1-2, y1-2
	if err := p.mouse("mousePressed", fx, fy, "left", 1); err != nil {
		return err
	}
	for i := 1; i <= 5; i++ {
		f := float64(i) / 5
		if err := p.mouse("mouseMoved", fx+(tx-fx)*f, fy+(ty-fy)*f, "left", 0); err != nil {
			return err
		}
	}
	return p.mouse("mouseReleased", tx, ty, "left", 1)
}

// rightClick presses and releases the right button in the middle of the
// element: the browser fires contextmenu there (headless shows no menu).
func (p *page) rightClick(sel string) error {
	x0, y0, x1, y1, err := p.rect(sel)
	if err != nil {
		return err
	}
	x, y := (x0+x1)/2, (y0+y1)/2
	if err := p.mouse("mousePressed", x, y, "right", 1); err != nil {
		return err
	}
	return p.mouse("mouseReleased", x, y, "right", 1)
}

// clickOn clicks the left button in the middle of the element.
func (p *page) clickOn(sel string) error {
	x0, y0, x1, y1, err := p.rect(sel)
	if err != nil {
		return err
	}
	return p.click((x0+x1)/2, (y0+y1)/2)
}

func (p *page) mouse(typ string, x, y float64, button string, clicks int) error {
	ev := map[string]any{"type": typ, "x": x, "y": y, "button": button, "clickCount": clicks}
	if typ == "mouseMoved" {
		ev["buttons"] = 1
	}
	return p.b.call(p.session, "Input.dispatchMouseEvent", ev, nil)
}
