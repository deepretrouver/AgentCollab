package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var ErrClosed = errors.New("sqlite store is closed")

type writeRequest struct {
	ctx    context.Context
	write  func(*sql.Tx) error
	result chan error
}

type Store struct {
	db          *sql.DB
	writes      chan writeRequest
	done        chan struct{}
	writerDone  chan struct{}
	operationMu sync.Mutex
	stateMu     sync.RWMutex
	closed      bool
}

func Open(ctx context.Context, path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("database path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configure(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &Store{
		db:         db,
		writes:     make(chan writeRequest),
		done:       make(chan struct{}),
		writerDone: make(chan struct{}),
	}
	go store.runWriter()
	return store, nil
}

func configure(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&journal); err != nil {
		return fmt.Errorf("set journal_mode=DELETE: %w", err)
	}
	if journal != "delete" {
		return fmt.Errorf("journal_mode = %q, want delete", journal)
	}
	for _, statement := range []string{
		"PRAGMA synchronous=FULL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply %s: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) runWriter() {
	defer close(s.writerDone)
	for {
		select {
		case request := <-s.writes:
			request.result <- s.executeWrite(request.ctx, request.write)
		case <-s.done:
			return
		}
	}
}

func (s *Store) executeWrite(ctx context.Context, write func(*sql.Tx) error) (err error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("write transaction panic: %v", recovered)
		}
	}()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}
	if err := write(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit write transaction: %w", err)
	}
	return nil
}

func (s *Store) Write(ctx context.Context, write func(*sql.Tx) error) error {
	if write == nil {
		return fmt.Errorf("write function is required")
	}
	s.stateMu.RLock()
	closed := s.closed
	s.stateMu.RUnlock()
	if closed {
		return ErrClosed
	}

	request := writeRequest{ctx: ctx, write: write, result: make(chan error, 1)}
	select {
	case s.writes <- request:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrClosed
	}
	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrClosed
	}
}

// Read executes a read-only callback using the Store database connection.
func (s *Store) Read(ctx context.Context, read func(*sql.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.stateMu.RLock()
	closed := s.closed
	s.stateMu.RUnlock()
	if closed {
		return ErrClosed
	}
	return read(s.db)
}

func (s *Store) IntegrityCheck(ctx context.Context) error {
	return s.Read(ctx, func(db *sql.DB) error {
		var result string
		if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
			return fmt.Errorf("run integrity_check: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("integrity_check returned %q", result)
		}
		return nil
	})
}

func (s *Store) Close() error {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil
	}
	s.closed = true
	close(s.done)
	s.stateMu.Unlock()
	<-s.writerDone
	return s.db.Close()
}
