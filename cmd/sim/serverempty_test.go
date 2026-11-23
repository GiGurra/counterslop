package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gigurra/counterslop/internal/app"
	"github.com/gorilla/websocket"
)

// TestWaitServerEmpty: the next scenario (or case) starts only once the last
// one's clients are gone, a connection that never said hello (the menus'
// hidden observer) included. Browser pages are reused now, so a closed page's
// socket closes on its own time: name-tags' alice met team-cap's in CI.
func TestWaitServerEmpty(t *testing.T) {
	a, err := app.New(app.Options{Version: "test", Dev: "../../web", Test: true})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	go a.Server.Run(stop)
	hs := httptest.NewServer(a.Handler)
	defer hs.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(hs.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); a.Server.Connected() == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the connection never registered")
		}
	}
	if h, _ := a.Server.PlayerCount(); h != 0 {
		t.Fatalf("no hello yet, but PlayerCount says %d humans", h)
	}
	if err := waitServerEmpty(a.Server, 100*time.Millisecond); err == nil {
		t.Fatal("waitServerEmpty returned with a client still connected")
	}
	time.AfterFunc(50*time.Millisecond, func() { ws.Close() })
	t0 := time.Now()
	if err := waitServerEmpty(a.Server, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d < 40*time.Millisecond {
		t.Fatalf("returned after %v, before the client left", d)
	}
}
