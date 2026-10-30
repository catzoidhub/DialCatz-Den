package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
)

// The app's side: no login, the token is the member. One token per device; revoking it locks that device out.

const callerKey = "caller"

func (s *Server) requireToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if token == "" || token == h {
			fail(c, http.StatusUnauthorized, "Missing token (Authorization: Bearer dct_…)")
			return
		}
		caller, err := s.store.TokenCaller(token)
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusUnauthorized, "This token is not valid any more (revoked or expired). Ask your admin for a new invite link.")
			return
		}
		if err != nil {
			internal(c, err)
			return
		}
		c.Set(callerKey, caller)
		c.Next()
	}
}

func caller(c *gin.Context) *store.Caller { return c.MustGet(callerKey).(*store.Caller) }

type identity struct {
	Member     memberOut          `json:"member"`
	Device     string             `json:"device"`
	Workspaces []store.Membership `json:"workspaces"`
}
type memberOut struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// AvatarURL: the admin's picture, else Gravatar's (?d=404: a 404 there means "show initials").
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// identity is who the token belongs to, with only the workspaces the token's scope covers.
func (s *Server) identity(cl *store.Caller) (*identity, error) {
	m, err := s.store.GetMember(cl.Member.ID)
	if err != nil {
		return nil, err
	}
	ws := []store.Membership{}
	for _, w := range m.Workspaces {
		if cl.May(w.WorkspaceID) {
			ws = append(ws, w)
		}
	}
	return &identity{Member: memberOut{ID: m.ID, Name: m.Name, Email: m.Email, AvatarURL: m.AvatarURL}, Device: cl.Device, Workspaces: ws}, nil
}

func (s *Server) me(c *gin.Context) {
	id, err := s.identity(caller(c))
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, id)
}

// connect exchanges a one-time invite code for a device token (from a dialcatz://connect link).
func (s *Server) connect(c *gin.Context) {
	var in struct {
		Code   string `json:"code"`
		Device string `json:"device"`
	}
	if !bind(c, &in) {
		return
	}
	token, cl, err := s.store.RedeemInvite(strings.TrimSpace(in.Code), clean(in.Device, 80))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusUnauthorized, "This invite link was already used or has expired. Ask your admin for a new one.")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	s.store.Audit("member:"+cl.Member.Name, "device.connected", cl.Device)
	id, err := s.identity(cl)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "identity": id})
}

// access is the caller's role on a workspace: "" when they are not a member, or their token's scope leaves the
// workspace out.
func (s *Server) access(cl *store.Caller, workspaceID string) (role string, inScope bool, err error) {
	if !cl.May(workspaceID) {
		return "", false, nil
	}
	role, err = s.store.Role(workspaceID, cl.Member.ID)
	return role, true, err
}

// role checks the caller belongs to the workspace (and the token may use it) and returns their role.
func (s *Server) role(c *gin.Context) (string, bool) {
	role, inScope, err := s.access(caller(c), c.Param("id"))
	if err != nil {
		internal(c, err)
		return "", false
	}
	if !inScope {
		fail(c, http.StatusForbidden, "This device's token does not cover this workspace")
		return "", false
	}
	if role == "" {
		fail(c, http.StatusForbidden, "You are not a member of this workspace")
		return "", false
	}
	return role, true
}

// renameByOwner lets a workspace's owner rename it from the app (deleting stays in the admin panel). The open
// event streams hear "renamed", so every member's app shows the new name at once.
func (s *Server) renameByOwner(c *gin.Context) {
	role, ok := s.role(c)
	if !ok {
		return
	}
	if role != "owner" {
		fail(c, http.StatusForbidden, "Only the workspace's owner can rename it")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !bind(c, &in) {
		return
	}
	if in.Name = clean(in.Name, 120); in.Name == "" {
		fail(c, http.StatusBadRequest, "Give the workspace a name")
		return
	}
	if err := s.store.RenameWorkspace(c.Param("id"), in.Name); err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	cl := caller(c)
	s.store.Audit("member:"+cl.Member.Name, "workspace.renamed", in.Name)
	s.hub.Publish(c.Param("id"), "renamed", struct {
		Name string       `json:"name"`
		By   store.Person `json:"by"`
	}{in.Name, store.Person{ID: cl.Member.ID, Name: cl.Member.Name}})
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": in.Name})
}

func (s *Server) changes(c *gin.Context) {
	if _, ok := s.role(c); !ok {
		return
	}
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	rev, items, err := s.store.Changes(c.Param("id"), since)
	if err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	c.JSON(http.StatusOK, gin.H{"revision": rev, "items": items})
}

const maxChanges = 2000

func (s *Server) push(c *gin.Context) {
	role, ok := s.role(c)
	if !ok {
		return
	}
	if role == "viewer" {
		fail(c, http.StatusForbidden, "Viewers cannot change this workspace")
		return
	}
	var in struct {
		Changes []store.Change `json:"changes"`
	}
	if !bind(c, &in) {
		return
	}
	if len(in.Changes) > maxChanges {
		fail(c, http.StatusRequestEntityTooLarge, "Too many changes in one push")
		return
	}
	for i := range in.Changes {
		ch := &in.Changes[i]
		ch.Name = clean(ch.Name, 200)
		if ch.ID == "" || len(ch.ID) > 64 || (!ch.Deleted && !store.Kinds[ch.Kind]) {
			fail(c, http.StatusBadRequest, "A change has a bad id or kind")
			return
		}
	}
	cl := caller(c)
	rev, applied, conflicts, err := s.store.Push(c.Param("id"), cl.Member.ID, in.Changes)
	if err != nil {
		storeErr(c, err, "No such workspace")
		return
	}
	// The transaction has committed: tell the open event streams (the pusher's own too; the app ignores revisions
	// it already has).
	if len(applied) > 0 {
		s.hub.Publish(c.Param("id"), "change", struct {
			Revision int64        `json:"revision"`
			By       store.Person `json:"by"`
		}{rev, store.Person{ID: cl.Member.ID, Name: cl.Member.Name}})
	}
	c.JSON(http.StatusOK, gin.H{"revision": rev, "applied": applied, "conflicts": conflicts})
}
