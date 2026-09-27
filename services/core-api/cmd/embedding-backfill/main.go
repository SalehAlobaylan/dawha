// Command embedding-backfill gives the vector leg in internal/research something
// to score.
//
// `retrieveVectorPassages` requires `sp.embedding IS NOT NULL`, the seed is pure
// SQL, and the only writer of that column is the source-processing worker -
// which cannot be pointed at seeded passages, because it looks work up through
// `source_files` and INSERTs a passage per extracted page rather than updating
// an existing one. So every test, demo and benchmark in this repository had been
// running the lexical leg alone, silently.
//
// This command is the explicit, idempotent, loud way to fill that column through
// the same `/embed` contract the worker calls. The reasoning, the refusals and
// the scope rules live in internal/embeddingbackfill, which is where they are
// tested; this file is the argument parsing and the reporting.
//
// Usage:
//
//	embedding-backfill             # the seeded corpus only
//	embedding-backfill -all        # every passage in the database
//	embedding-backfill -check      # report coverage, write nothing
//
// Environment:
//
//	DATABASE_URL     required
//	AI_RESEARCH_URL  the deterministic ai-research service. Default
//	                 http://localhost:8000. No credential is involved and none
//	                 is accepted: this is the only embedding source permitted
//	                 here, and adding a second one is out of scope by decision.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/embeddingbackfill"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	all := flag.Bool("all", false, "embed every passage in the database, not only the seeded corpus")
	check := flag.Bool("check", false, "report coverage and exit without writing anything")
	flag.Parse()

	scope := embeddingbackfill.SeedScope()
	if *all {
		scope = embeddingbackfill.AllScope()
	}

	pool, err := db.NewPool(ctx, db.PoolConfig{URL: os.Getenv("DATABASE_URL")})
	if err != nil || pool == nil {
		log.Fatal("embedding-backfill requires DATABASE_URL")
	}
	defer pool.Close()

	provider := ai.NewHTTPClient(environmentValue("AI_RESEARCH_URL", "http://localhost:8000"))
	service := embeddingbackfill.New(pool, provider.Provider)

	before, err := service.Coverage(ctx, scope)
	if err != nil {
		log.Fatalf("read the passage coverage: %v", err)
	}
	log.Printf("scope: %s (%d dimensions)", scope, embeddingbackfill.Dimensions)
	log.Printf("before: %d passages, %d carry an embedding, %d do not", before.Total, before.Embedded, before.Missing)

	if *check {
		log.Print("-check: wrote nothing")
		if before.Missing != 0 {
			log.Printf("%d passage(s) in scope have no embedding; run embedding-backfill to fill them", before.Missing)
			return
		}
		log.Print("every passage in scope carries an embedding")
		return
	}

	result, err := service.Run(ctx, scope)
	if err != nil {
		if errors.Is(err, embeddingbackfill.ErrIncomplete) {
			// The loudest line in the command. A run that embedded 12 of 15 rows
			// and exited 0 is a measurement that comes out wrong and says it
			// passed.
			log.Printf("after: %d passages, %d carry an embedding, %d do not (%d written by this run)", result.After.Total, result.After.Embedded, result.After.Missing, result.Written)
		}
		log.Fatalf("embedding backfill: %v", err)
	}
	log.Printf("after: %d passages, %d carry an embedding, %d do not (%d written by this run, %d verified as %d-dimension vectors)", result.After.Total, result.After.Embedded, result.After.Missing, result.Written, result.Verified, embeddingbackfill.Dimensions)
	if result.Model != "" {
		log.Printf("model: %s", result.Model)
	}
	if result.Written == 0 {
		log.Print("nothing to do: every passage in scope already had an embedding (idempotent no-op)")
	}
}

func environmentValue(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
