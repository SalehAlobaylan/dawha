package sourceprocessing

import (
	"context"
	"os"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
)

func TestEntityReferencesFindSeededAlias(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	references, err := (&Service{Pool: pool}).entityReferences(context.Background(), "أبو بكر")
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 1 || references[0].Type != "person" || references[0].ID != "10000000-0000-0000-0000-000000000001" {
		t.Fatalf("unexpected references: %+v", references)
	}
	if len(references[0].Aliases) != 1 || references[0].Aliases[0] != "أبو بكر" {
		t.Fatalf("unexpected aliases: %+v", references[0].Aliases)
	}
}

func TestResolveEntityUsesSeededAlias(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	aiURL := os.Getenv("AI_RESEARCH_URL")
	if databaseURL == "" || aiURL == "" {
		t.Skip("DATABASE_URL and AI_RESEARCH_URL are required")
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{URL: databaseURL})
	if err != nil || pool == nil {
		t.Fatal("database is unavailable")
	}
	defer pool.Close()
	service := &Service{Pool: pool, AI: ai.NewHTTPClient(aiURL)}
	link, err := service.resolveEntity(context.Background(), "أبو بكر")
	if err != nil {
		t.Fatal(err)
	}
	if link.Type != "person" || link.ID != "10000000-0000-0000-0000-000000000001" || link.Score != 0.95 {
		t.Fatalf("unexpected link: %+v", link)
	}
}
