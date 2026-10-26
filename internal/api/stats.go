package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
)

// Activity dashboard (Admin → Activity → Dashboard): the activity log of the last N days counted per day and kind, per
// weekday and hour, per person and per action, with the period before for a trend. Days are the admin's own (tz is
// their offset from UTC in minutes), so "busiest day" means their day.

type statDay struct {
	Day   string         `json:"day"`
	Total int            `json:"total"`
	Kinds map[string]int `json:"kinds"`
}

type statCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func top(m map[string]int, n int) []statCount {
	out := make([]statCount, 0, len(m))
	for k, v := range m {
		out = append(out, statCount{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// auditStats (GET /audit/stats?days=30&tz=420).
func (s *Server) auditStats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 || days > 365 {
		days = 30
	}
	tz, _ := strconv.Atoi(c.Query("tz"))
	if tz < -14*60 || tz > 14*60 {
		tz = 0
	}
	loc := time.FixedZone("admin", tz*60)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, -(days - 1))
	end := today.AddDate(0, 0, 1)

	series := make([]statDay, days)
	index := map[string]int{}
	for i := range series {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		series[i] = statDay{Day: d, Kinds: map[string]int{}}
		index[d] = i
	}
	kinds := map[string]int{}
	actors := map[string]int{}
	actions := map[string]int{}
	people := map[string]bool{}
	devices := map[string]int{}
	var heat [7][24]int
	total, connections, rejected := 0, 0, 0
	err := s.store.EachAudit(start.UnixMilli(), end.UnixMilli(), func(at int64, actor, action string) {
		t := time.UnixMilli(at).In(loc)
		i, ok := index[t.Format("2006-01-02")]
		if !ok {
			return
		}
		k := store.AuditKindOf(action)
		total++
		series[i].Total++
		series[i].Kinds[k]++
		kinds[k]++
		actors[actor]++
		actions[action]++
		people[actor] = true
		heat[int(t.Weekday())][t.Hour()]++
		switch {
		case action == "device.connected":
			connections++
			devices[strings.TrimPrefix(actor, "member:")]++
		case action == "admin.oauth.rejected" || action == "passkey.clone-warning":
			rejected++
		}
	})
	if err != nil {
		internal(c, err)
		return
	}
	prev, err := s.store.AuditCountBetween(start.AddDate(0, 0, -days).UnixMilli(), start.UnixMilli())
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"days":        series,
		"kinds":       kinds,
		"actors":      top(actors, 8),
		"actions":     top(actions, 10),
		"members":     top(devices, 8),
		"heat":        heat,
		"total":       total,
		"previous":    prev,
		"people":      len(people),
		"connections": connections,
		"warnings":    rejected,
	})
}
