package main

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"log"
	"net/http"
	"os"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/i18n"
	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/server"
	"tournamentstudio/internal/store"
	"tournamentstudio/internal/webui"
)

func main() {
	dbPath := os.Getenv("TOURNAMENTSTUDIO_DB")
	if dbPath == "" {
		dbPath = "tournamentstudio.db"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.DB.Close()

	if err := bootstrapAdmin(st); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	pluginsDir := os.Getenv("TOURNAMENTSTUDIO_PLUGINS")
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}
	engine, err := plugin.Load(pluginsDir)
	if err != nil {
		log.Fatalf("load plugins: %v", err)
	}
	defer engine.Close()

	languagesDir := os.Getenv("TOURNAMENTSTUDIO_LANGUAGES")
	if languagesDir == "" {
		languagesDir = "languages"
	}
	catalog, err := i18n.Load(languagesDir)
	if err != nil {
		log.Fatalf("load i18n: %v", err)
	}

	frontendFS, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		log.Fatalf("prepare embedded frontend: %v", err)
	}

	s := server.New(st, engine, pluginsDir, catalog, frontendFS)
	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}

// bootstrapAdmin ensures at least one user account exists so the deployment
// is never left in a state where nobody can log in. If the users table is
// empty, it creates an initial Organizer account, either from the
// TOURNAMENTSTUDIO_ADMIN_USER / TOURNAMENTSTUDIO_ADMIN_PASSWORD environment
// variables if both are set, or otherwise as "admin" with a randomly
// generated password that is logged once.
func bootstrapAdmin(st *store.Store) error {
	users := auth.NewRepo(st)

	count, err := users.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	username := os.Getenv("TOURNAMENTSTUDIO_ADMIN_USER")
	password := os.Getenv("TOURNAMENTSTUDIO_ADMIN_PASSWORD")
	if username != "" && password != "" {
		if _, err := users.Create(username, password, auth.RoleOrganizer); err != nil {
			return err
		}
		log.Printf("Created initial admin account %q from TOURNAMENTSTUDIO_ADMIN_USER/TOURNAMENTSTUDIO_ADMIN_PASSWORD", username)
		return nil
	}

	generated, err := randomPassword()
	if err != nil {
		return err
	}
	if _, err := users.Create("admin", generated, auth.RoleOrganizer); err != nil {
		return err
	}
	log.Printf("Created initial admin account 'admin' with password: %s — save this, it will not be shown again", generated)
	return nil
}

func randomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
