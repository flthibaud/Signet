package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/flthibaud/omnivore-go/internal/data"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
)

var linkSlugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

type FeedImporter struct {
	models data.Models
}

func NewFeedImporter(models data.Models) *FeedImporter {
	return &FeedImporter{
		models: models,
	}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	s = linkSlugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func renderToValidUTF8(render func(w io.Writer) error) string {
	var sb strings.Builder
	if err := render(&sb); err != nil {
		return ""
	}
	return strings.ToValidUTF8(sb.String(), "")
}

func parseDate(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	return time.Now()
}

func calculateHash(url string) string {
	hashArr := sha256.Sum256([]byte(url))
	hashHex := hex.EncodeToString(hashArr[:])

	return hashHex
}

func (s *FeedImporter) ImportRecent(feedID int64, userID uuid.UUID) error {
	// 1. Récupérer l'URL du feed depuis la DB
	feed, err := s.models.Feeds.Get(feedID)
	if err != nil {
		return fmt.Errorf("feed not found: %w", err)
	}

	// 2. Parsing sécurisé
	parser := gofeed.NewParser()
	// On ajoute un timeout pour ne pas bloquer indéfiniment
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rss, err := parser.ParseURLWithContext(feed.Url, ctx)
	if err != nil {
		return fmt.Errorf("failed to parse RSS: %w", err)
	}

	// 3. Update metadata (Titre manquant)
	if feed.Title == "" {
		s.models.Feeds.UpdateMetadata(feedID, rss.Title, rss.Image.URL)
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)

	// 4. Boucle sur les items
	for _, item := range rss.Items {
		if item.Link == "" {
			continue
		}

		wg.Add(1)

		// On passe 'item' en paramètre pour éviter les problèmes de closure
		go func(item *gofeed.Item) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// A. Check si l'article existe déjà
			articleID, err := s.models.Articles.GetIDByURL(item.Link)

			// B. S'il n'existe pas, on scrape
			if err == sql.ErrNoRows {
				// Scraping
				readabilityArticle, err := readability.FromURL(item.Link, 30*time.Second)
				if err != nil {
					log.Printf("Scraping failed for %s: %v", item.Link, err)
					return
				}

				// Préparation des données propres
				// Note: J'ai créé une petite struct intermédiaire pour passer au model, ou tu passes les champs
				newArticle := &data.Article{
					Url:         item.Link,
					Hash:        calculateHash(item.Link),
					Title:       strings.ToValidUTF8(readabilityArticle.Title(), ""),
					Content:     renderToValidUTF8(readabilityArticle.RenderHTML),
					TextContent: renderToValidUTF8(readabilityArticle.RenderText),
					PublishedAt: parseDate(item), // Helper pour gérer les dates
				}

				articleID, err = s.models.Articles.Insert(newArticle)
				if err != nil {
					log.Printf("Insert Article failed: %v", err)
					return
				}
			} else if err != nil {
				log.Printf("DB Check failed: %v", err)
				return
			}

			// C. Création du Lien Utilisateur
			slug := slugify(item.Title)
			pubDate := parseDate(item)

			err = s.models.Links.Insert(userID, articleID, feedID, slug, pubDate)
			if err != nil {
				log.Printf("Link creation failed: %v", err)
			}

		}(item)
	}

	// On attend que toutes les goroutines finissent
	wg.Wait()
	return nil
}
