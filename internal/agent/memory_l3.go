package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// PGKnowledgeBase implements KnowledgeBase using PostgreSQL + pgvector.
// Each completed diagnosis is stored with a dense embedding and can be
// retrieved by cosine similarity before a new agent run begins.
type PGKnowledgeBase struct {
	pool *pgxpool.Pool
	dim  int // embedding dimension, must match the embedding model output
}

// NewPGKnowledgeBase wraps an existing pgxpool.Pool.
// Use NewPGKnowledgeBaseFromDSN when you need the pool created here.
func NewPGKnowledgeBase(pool *pgxpool.Pool, dim int) *PGKnowledgeBase {
	return &PGKnowledgeBase{pool: pool, dim: dim}
}

// NewPGKnowledgeBaseFromDSN creates a pgxpool with pgvector type registration
// and returns a PGKnowledgeBase backed by it.
func NewPGKnowledgeBaseFromDSN(ctx context.Context, dsn string, dim int) (*PGKnowledgeBase, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("l3: failed to parse dsn: %w", err)
	}

	// Register the pgvector type codec for every new connection in the pool.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if err := pgxvector.RegisterTypes(ctx, conn); err != nil {
			return fmt.Errorf("l3: pgvector type registration: %w", err)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("l3: failed to create pgx pool: %w", err)
	}

	return &PGKnowledgeBase{pool: pool, dim: dim}, nil
}

// InitSchema creates the required PostgreSQL extension and table if they do not exist.
// Safe to call on every startup (idempotent).
func (kb *PGKnowledgeBase) InitSchema(ctx context.Context) error {
	// dim is an integer from internal config, not user input — safe to interpolate.
	ddl := fmt.Sprintf(`
		CREATE EXTENSION IF NOT EXISTS vector;

		CREATE TABLE IF NOT EXISTS diagnosis_findings (
			id          UUID DEFAULT gen_random_uuid() PRIMARY KEY,
			alert_name  TEXT NOT NULL DEFAULT '',
			namespace   TEXT NOT NULL DEFAULT '',
			root_cause  TEXT NOT NULL,
			suggestion  TEXT NOT NULL DEFAULT '',
			embedding   vector(%d),
			created_at  TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS diagnosis_findings_embedding_idx
			ON diagnosis_findings USING ivfflat (embedding vector_cosine_ops)
			WITH (lists = 100);
	`, kb.dim)

	if _, err := kb.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("l3: failed to init schema: %w", err)
	}
	return nil
}

// SaveDiagnosis inserts a completed diagnosis and its embedding into the knowledge base.
func (kb *PGKnowledgeBase) SaveDiagnosis(ctx context.Context, finding KnowledgeFinding, embedding []float32) error {
	var vec *pgvector.Vector
	if len(embedding) > 0 {
		v := pgvector.NewVector(embedding)
		vec = &v
	}

	_, err := kb.pool.Exec(ctx, `
		INSERT INTO diagnosis_findings (alert_name, namespace, root_cause, suggestion, embedding)
		VALUES ($1, $2, $3, $4, $5)
	`, finding.AlertName, finding.Namespace, finding.RootCause, finding.Suggestion, vec)
	if err != nil {
		return fmt.Errorf("l3: failed to save diagnosis: %w", err)
	}
	return nil
}

// SearchSimilar returns the top-limit diagnoses closest to queryEmbedding by cosine distance.
// Returns an empty slice (no error) when queryEmbedding is nil or the table is empty.
func (kb *PGKnowledgeBase) SearchSimilar(ctx context.Context, queryEmbedding []float32, limit int) ([]KnowledgeFinding, error) {
	if len(queryEmbedding) == 0 {
		return nil, nil
	}

	vec := pgvector.NewVector(queryEmbedding)

	rows, err := kb.pool.Query(ctx, `
		SELECT id, alert_name, namespace, root_cause, suggestion, created_at
		FROM diagnosis_findings
		ORDER BY embedding <=> $1
		LIMIT $2
	`, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("l3: failed to search similar diagnoses: %w", err)
	}
	defer rows.Close()

	var findings []KnowledgeFinding
	for rows.Next() {
		var f KnowledgeFinding
		if err := rows.Scan(&f.ID, &f.AlertName, &f.Namespace, &f.RootCause, &f.Suggestion, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("l3: failed to scan row: %w", err)
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("l3: row iteration error: %w", err)
	}

	return findings, nil
}

// FormatHistoricalFindings formats a list of historical diagnoses as a human-readable
// string suitable for injection into the agent's LLM context before the ReAct loop.
func FormatHistoricalFindings(findings []KnowledgeFinding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Historically similar diagnoses (from L3 knowledge base):\n")
	for i, f := range findings {
		b.WriteString(fmt.Sprintf(
			"  [%d] alert=%s namespace=%s root_cause=%s suggestion=%s (recorded %s)\n",
			i+1, f.AlertName, f.Namespace, f.RootCause, f.Suggestion,
			f.CreatedAt.Format(time.RFC3339),
		))
	}
	return b.String()
}
