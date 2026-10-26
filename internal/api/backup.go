package api

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/catzoidhub/DialCatz-Den/internal/config"
	"github.com/catzoidhub/DialCatz-Den/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/argon2"
)

// Backup and restore, for moving to another server (Admin → Backup). A backup is one file:
//
//	"DCDENBK1" | salt (16) | nonce (12) | AES-256-GCM( gzip( JSON ) )
//
// with the key from the admin's passphrase (Argon2id). The JSON holds the members, workspaces and their items,
// memberships, devices (token hashes) and the activity log — not the admins, who stay with each server. A restore
// replaces exactly those on this server, in one transaction; a check first ("dryRun") only opens the file and says
// what is in it.

const (
	backupMagic   = "DCDENBK1"
	backupFormat  = "dialcatz-den-backup"
	backupVersion = 1
	maxBackup     = 2 << 30 // 2 GB
	minPassphrase = 8
)

type backupFile struct {
	Format    string                 `json:"format"`
	Version   int                    `json:"version"`
	Schema    int64                  `json:"schema"`
	Server    string                 `json:"server"`
	From      string                 `json:"from"`
	CreatedAt int64                  `json:"createdAt"`
	Counts    map[string]int         `json:"counts"`
	Tables    map[string][]store.Row `json:"tables"`
}

func backupKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 2, 32)
}

func sealBackup(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(backupKey(passphrase, salt))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := append([]byte(backupMagic), salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, []byte(backupMagic)), nil
}

var errNotBackup = errors.New("This is not a DialCatz Den backup file")
var errWrongPassphrase = errors.New("The passphrase does not open this backup (or the file is damaged)")

func openBackup(data []byte, passphrase string) ([]byte, error) {
	if len(data) < len(backupMagic)+16+12+16 || string(data[:len(backupMagic)]) != backupMagic {
		return nil, errNotBackup
	}
	rest := data[len(backupMagic):]
	salt, nonce, sealed := rest[:16], rest[16:28], rest[28:]
	block, err := aes.NewCipher(backupKey(passphrase, salt))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, sealed, []byte(backupMagic))
	if err != nil {
		return nil, errWrongPassphrase
	}
	return plain, nil
}

// backup (POST /api/admin/backup {passphrase}) streams the encrypted backup file.
func (s *Server) backup(c *gin.Context) {
	var in struct {
		Passphrase string `json:"passphrase"`
	}
	if !bind(c, &in) {
		return
	}
	if len([]rune(in.Passphrase)) < minPassphrase {
		fail(c, http.StatusBadRequest, fmt.Sprintf("Choose a passphrase of at least %d characters", minPassphrase))
		return
	}
	schema, err := s.store.SchemaVersion()
	if err != nil {
		internal(c, err)
		return
	}
	f := backupFile{Format: backupFormat, Version: backupVersion, Schema: schema, Server: config.Version, From: s.cfg.PublicURL, CreatedAt: time.Now().UnixMilli(), Counts: map[string]int{}, Tables: map[string][]store.Row{}}
	for _, t := range store.BackupTables {
		rows, err := s.store.ExportTable(t)
		if err != nil {
			internal(c, err)
			return
		}
		f.Tables[t] = rows
		f.Counts[t] = len(rows)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(f); err != nil {
		internal(c, err)
		return
	}
	if err := zw.Close(); err != nil {
		internal(c, err)
		return
	}
	out, err := sealBackup(buf.Bytes(), in.Passphrase)
	if err != nil {
		internal(c, err)
		return
	}
	s.audit(c, "server.backup", fmt.Sprintf("%d members, %d workspaces, %d devices", f.Counts["members"], f.Counts["workspaces"], f.Counts["tokens"]))
	name := fmt.Sprintf("dialcatz-den-%s-%s.dcden", hostOf(s.cfg.PublicURL), time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/octet-stream", out)
}

func hostOf(public string) string {
	u, err := url.Parse(public)
	if err != nil || u.Hostname() == "" {
		return "server"
	}
	return strings.ReplaceAll(u.Hostname(), ":", "-")
}

type backupSummary struct {
	From      string         `json:"from"`
	Server    string         `json:"server"`
	CreatedAt int64          `json:"createdAt"`
	Counts    map[string]int `json:"counts"`
}

// restore (POST /api/admin/restore, multipart: file, passphrase, dryRun, confirm) checks a backup, or replaces this
// server's members, workspaces, devices and activity with it. confirm must be this server's host name.
func (s *Server) restore(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBackup)
	fh, err := c.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "Choose the backup file (.dcden)")
		return
	}
	src, err := fh.Open()
	if err != nil {
		internal(c, err)
		return
	}
	data, err := io.ReadAll(src)
	src.Close()
	if err != nil {
		fail(c, http.StatusRequestEntityTooLarge, "The backup file is too large (2 GB at most)")
		return
	}
	plain, err := openBackup(data, c.PostForm("passphrase"))
	if err != nil {
		if errors.Is(err, errNotBackup) || errors.Is(err, errWrongPassphrase) {
			fail(c, http.StatusBadRequest, err.Error())
		} else {
			internal(c, err)
		}
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		fail(c, http.StatusBadRequest, "The backup could not be read")
		return
	}
	dec := json.NewDecoder(zr)
	dec.UseNumber()
	var f backupFile
	if err := dec.Decode(&f); err != nil || f.Format != backupFormat {
		fail(c, http.StatusBadRequest, "The backup could not be read")
		return
	}
	if f.Version > backupVersion {
		fail(c, http.StatusBadRequest, "This backup comes from a newer DialCatz Den. Update this server first, then restore.")
		return
	}
	schema, err := s.store.SchemaVersion()
	if err != nil {
		internal(c, err)
		return
	}
	if f.Schema > schema {
		fail(c, http.StatusBadRequest, fmt.Sprintf("This backup comes from a newer DialCatz Den (%s). Update this server first, then restore.", f.Server))
		return
	}
	sum := backupSummary{From: f.From, Server: f.Server, CreatedAt: f.CreatedAt, Counts: f.Counts}
	if c.PostForm("dryRun") == "1" {
		c.JSON(http.StatusOK, gin.H{"summary": sum})
		return
	}
	if strings.TrimSpace(c.PostForm("confirm")) != hostOf(s.cfg.PublicURL) {
		fail(c, http.StatusBadRequest, "Type this server's name to confirm")
		return
	}
	counts, err := s.store.Restore(f.Tables)
	if err != nil {
		internal(c, err)
		fail(c, http.StatusInternalServerError, "The restore failed and nothing was changed: "+err.Error())
		return
	}
	s.audit(c, "server.restored", fmt.Sprintf("from %s (%s): %d members, %d workspaces, %d devices", f.From, time.UnixMilli(f.CreatedAt).Format("2006-01-02 15:04"), counts["members"], counts["workspaces"], counts["tokens"]))
	c.JSON(http.StatusOK, gin.H{"summary": sum, "restored": counts})
}
