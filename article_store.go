package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type articleMetadata struct {
	ID      int64  `yaml:"id"`
	Title   string `yaml:"title"`
	Image   string `yaml:"image"`
	Created string `yaml:"created_at"`
	Updated string `yaml:"updated_at"`
	Draft   bool   `yaml:"draft,omitempty"`
}

func (a *application) articlePath(id int64) string {
	return filepath.Join(a.articleDir, strconv.FormatInt(id, 10)+".md")
}

func (a *application) recentArticles(limit int) ([]article, error) {
	entries, err := os.ReadDir(a.articleDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []article
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(entry.Name(), ".md"), 10, 64)
		if err != nil || id < 1 {
			continue
		}
		item, err := a.getArticle(id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left, leftErr := time.Parse(timeFormat, items[i].Created)
		right, rightErr := time.Parse(timeFormat, items[j].Created)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		if items[i].Created == items[j].Created {
			return items[i].ID > items[j].ID
		}
		return items[i].Created > items[j].Created
	})
	if limit >= 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (a *application) publishedArticles(limit int) ([]article, error) {
	if limit == 0 {
		return nil, nil
	}
	items, err := a.recentArticles(-1)
	if err != nil {
		return nil, err
	}
	var published []article
	for _, item := range items {
		if !item.Draft {
			published = append(published, item)
			if limit >= 0 && len(published) == limit {
				break
			}
		}
	}
	return published, nil
}

func (a *application) getArticle(id int64) (article, error) {
	if id < 1 {
		return article{}, os.ErrNotExist
	}
	raw, err := os.ReadFile(a.articlePath(id))
	if err != nil {
		return article{}, err
	}
	parts := bytes.SplitN(raw, []byte("\n---\n"), 2)
	if len(parts) != 2 || !bytes.HasPrefix(parts[0], []byte("---\n")) {
		return article{}, fmt.Errorf("article %d: missing YAML front matter", id)
	}
	var meta articleMetadata
	decoder := yaml.NewDecoder(bytes.NewReader(parts[0][4:]))
	decoder.KnownFields(true)
	if err := decoder.Decode(&meta); err != nil {
		return article{}, fmt.Errorf("article %d: %w", id, err)
	}
	if meta.ID != id || meta.Title == "" || meta.Image == "" || !validArticleImage(meta.Image) {
		return article{}, fmt.Errorf("article %d: invalid metadata", id)
	}
	if _, err := time.Parse(timeFormat, meta.Created); err != nil {
		return article{}, fmt.Errorf("article %d: invalid created_at: %w", id, err)
	}
	if _, err := time.Parse(timeFormat, meta.Updated); err != nil {
		return article{}, fmt.Errorf("article %d: invalid updated_at: %w", id, err)
	}
	body := bytes.TrimPrefix(parts[1], []byte("\n"))
	return article{ID: id, Title: meta.Title, Body: strings.TrimSuffix(string(body), "\n"), Image: meta.Image, Created: meta.Created, Updated: meta.Updated, Draft: meta.Draft}, nil
}

func (a *application) writeArticle(item article) error {
	if err := os.MkdirAll(a.articleDir, 0755); err != nil {
		return err
	}
	meta, err := yaml.Marshal(articleMetadata{ID: item.ID, Title: item.Title, Image: item.Image, Created: item.Created, Updated: item.Updated, Draft: item.Draft})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(a.articleDir, ".article-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	data := append([]byte("---\n"), meta...)
	data = append(data, []byte("---\n\n"+strings.TrimSpace(item.Body)+"\n")...)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), a.articlePath(item.ID))
}

func (a *application) nextArticleID() (int64, error) {
	items, err := a.recentArticles(-1)
	if err != nil {
		return 0, err
	}
	var maxID int64
	for _, item := range items {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID + 1, nil
}

// Import articles from installations that used SQLite, then remove the old table.
// The Markdown files are committed to Git by the operator after this first run.
func (a *application) importLegacyArticles() error {
	var exists int
	if err := a.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='articles'").Scan(&exists); err != nil || exists == 0 {
		return err
	}
	rows, err := a.db.Query("SELECT id,title,body_markdown,image_name,created_at,updated_at FROM articles ORDER BY id")
	if err != nil {
		return err
	}
	var legacy []article
	for rows.Next() {
		var item article
		if err := rows.Scan(&item.ID, &item.Title, &item.Body, &item.Image, &item.Created, &item.Updated); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if !validArticleImage(item.Image) {
			return fmt.Errorf("legacy article %d has invalid image name", item.ID)
		}
		if current, err := a.getArticle(item.ID); err == nil {
			if current.Title == item.Title && current.Body == item.Body && current.Image == item.Image && current.Created == item.Created && current.Updated == item.Updated {
				if _, err := os.Stat(filepath.Join(a.imageDir(), item.Image)); err == nil {
					continue
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			} else {
				return fmt.Errorf("legacy article %d conflicts with an existing Markdown file", item.ID)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(a.imageDir(), 0755); err != nil {
			return err
		}
		oldImage := filepath.Join(filepath.Dir(a.dataPath), "article-images", item.Image)
		source, err := os.Open(oldImage)
		if err != nil {
			return err
		}
		destination := filepath.Join(a.imageDir(), item.Image)
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(output, source)
		closeErr := output.Close()
		source.Close()
		if copyErr != nil || closeErr != nil {
			os.Remove(destination)
			return errors.Join(copyErr, closeErr)
		}
		if err := a.writeArticle(item); err != nil {
			return err
		}
	}
	_, err = a.db.Exec("DROP TABLE articles")
	return err
}
