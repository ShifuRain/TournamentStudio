package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
)

//go:embed bundles/*.json
var defaultBundles embed.FS

type Catalog struct {
	strings map[string]map[string]string
}

func Load(externalDir string) (*Catalog, error) {
	c := &Catalog{strings: make(map[string]map[string]string)}

	entries, err := defaultBundles.ReadDir("bundles")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		lang := langFromFilename(e.Name())
		data, err := defaultBundles.ReadFile("bundles/" + e.Name())
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		c.strings[lang] = m
	}

	if externalDir != "" {
		if err := c.loadExternal(externalDir); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Catalog) loadExternal(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		lang := langFromFilename(e.Name())
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		if c.strings[lang] == nil {
			c.strings[lang] = make(map[string]string)
		}
		for k, v := range m {
			c.strings[lang][k] = v
		}
	}
	return nil
}

func langFromFilename(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func (c *Catalog) Translate(lang, key string) string {
	if m, ok := c.strings[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := c.strings["en"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}
