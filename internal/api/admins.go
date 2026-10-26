package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/catzoidhub/DialCatz-Den/internal/auth"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
)

// More than one admin, so a team does not share one sign-in. An admin invites another (name + email); the
// one-time link (48 h) opens the panel's accept page, where they choose their own password. Admins can remove
// each other, but not themselves and never the last one.

func (s *Server) listAdmins(c *gin.Context) {
	admins, err := s.store.ListAdmins()
	if err != nil {
		internal(c, err)
		return
	}
	invites, err := s.store.ListAdminInvites()
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"admins": admins, "invites": invites, "me": admin(c).ID})
}

func (s *Server) inviteAdmin(c *gin.Context) {
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if !bind(c, &in) {
		return
	}
	in.Name, in.Email = clean(in.Name, 120), clean(in.Email, 200)
	if in.Name == "" || !validEmail(in.Email) {
		fail(c, http.StatusBadRequest, "Enter a name and a valid email")
		return
	}
	code, inv, err := s.store.CreateAdminInvite(in.Email, in.Name, admin(c).Name)
	if errors.Is(err, store.ErrConflict) {
		fail(c, http.StatusConflict, "There is already an admin with that email")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "admin.invited", inv.Name+" <"+inv.Email+">")
	c.JSON(http.StatusOK, gin.H{"link": s.cfg.PublicURL + "/?admin-invite=" + url.QueryEscape(code), "invite": inv})
}

func (s *Server) cancelAdminInvite(c *gin.Context) {
	if err := s.store.DeleteAdminInvite(c.Param("id")); err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "admin.invite-cancelled", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) removeAdmin(c *gin.Context) {
	me := admin(c)
	if c.Param("id") == me.ID {
		fail(c, http.StatusBadRequest, "You cannot remove yourself. Ask another admin to do it.")
		return
	}
	other, err := s.store.GetAdmin(c.Param("id"))
	if err != nil {
		storeErr(c, err, "No such admin")
		return
	}
	switch err := s.store.DeleteAdmin(other.ID); {
	case errors.Is(err, store.ErrLastAdmin):
		fail(c, http.StatusBadRequest, "The last admin cannot be removed")
		return
	case err != nil:
		storeErr(c, err, "No such admin")
		return
	}
	s.audit(c, "admin.removed", other.Name+" <"+other.Email+">")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// adminInviteInfo tells the accept page who the invite is for (signed out; the code is the secret).
func (s *Server) adminInviteInfo(c *gin.Context) {
	inv, err := s.store.AdminInviteByCode(c.Query("code"))
	if err != nil {
		fail(c, http.StatusNotFound, "This invite link was used already or has expired. Ask an admin for a new one.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": inv.Name, "email": inv.Email, "invitedBy": inv.InvitedBy, "expiresAt": inv.ExpiresAt})
}

func (s *Server) acceptAdminInvite(c *gin.Context) {
	var in struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !bind(c, &in) {
		return
	}
	if err := auth.CheckStrength(in.Password); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	a, err := s.store.AcceptAdminInvite(in.Code, clean(in.Name, 120), in.Password)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(c, http.StatusNotFound, "This invite link was used already or has expired. Ask an admin for a new one.")
		return
	case errors.Is(err, store.ErrConflict):
		fail(c, http.StatusConflict, "There is already an admin with that email")
		return
	case err != nil:
		internal(c, err)
		return
	}
	s.store.Audit("admin:"+a.Email, "admin.joined", "")
	if s.startSession(c, a, "invite") {
		c.JSON(http.StatusOK, a)
	}
}
