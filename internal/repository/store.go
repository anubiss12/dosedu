package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store bundles the pgx pool and gives every *Repo a shared connection.
type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	s.Pool.Close()
}
