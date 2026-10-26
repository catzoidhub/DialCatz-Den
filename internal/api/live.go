package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/live"
	"github.com/gin-gonic/gin"
)

// Live updates for the app: one server-sent event stream per open workspace, and presence heartbeats.
//
//	event: hello     data: {"revision":N,"presence":[…]}   once, first
//	event: change    data: {"revision":N,"by":{id,name}}   after a push applied something (pull /changes then)
//	event: presence  data: {"presence":[…]}                 when who-looks-at-what changes
//	: ping                                                  every 25 s (keeps proxies from closing the stream)

const (
	pingEvery  = 25 * time.Second
	writeGrace = 15 * time.Second
)

func (s *Server) events(c *gin.Context) {
	if _, ok := s.role(c); !ok {
		return
	}
	ws := c.Param("id")
	rev, err := s.store.Revision(ws)
	if err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	cl := caller(c)
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))

	// The server's ReadTimeout / WriteTimeout (60 s) would end the stream: lift the read deadline, and push the
	// write deadline forward before every write instead.
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetReadDeadline(time.Time{})
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	write := func(chunk string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(writeGrace))
		if _, err := io.WriteString(c.Writer, chunk); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	frame := func(name string, data []byte) string { return fmt.Sprintf("event: %s\ndata: %s\n\n", name, data) }

	sub, presence := s.hub.Subscribe(ws, cl.TokenID)
	defer s.hub.Unsubscribe(sub)
	hello, _ := json.Marshal(struct {
		Revision int64           `json:"revision"`
		Presence []live.Presence `json:"presence"`
	}{rev, presence})
	if !write(frame("hello", hello)) {
		return
	}
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			if !write(frame(ev.Name, ev.Data)) {
				return
			}
		case <-ping.C:
			// A revoked token, a removed membership or a narrowed scope ends the stream within a ping.
			now, err := s.store.TokenCaller(token)
			if err != nil {
				return
			}
			if role, _, err := s.access(now, ws); err != nil || role == "" {
				return
			}
			if !write(": ping\n\n") {
				return
			}
		}
	}
}

func (s *Server) presence(c *gin.Context) {
	if _, ok := s.role(c); !ok {
		return
	}
	var in struct {
		ItemID *string `json:"itemId"`
	}
	if !bind(c, &in) {
		return
	}
	if in.ItemID != nil {
		if *in.ItemID = strings.TrimSpace(*in.ItemID); *in.ItemID == "" {
			in.ItemID = nil
		} else if len(*in.ItemID) > 64 {
			fail(c, http.StatusBadRequest, "itemId is too long")
			return
		}
	}
	cl := caller(c)
	s.hub.SetPresence(c.Param("id"), cl.TokenID, live.Presence{MemberID: cl.Member.ID, Name: cl.Member.Name, Device: cl.Device, ItemID: in.ItemID})
	c.Status(http.StatusNoContent)
}
