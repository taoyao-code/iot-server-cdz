package migrate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runner 迁移执行器
type Runner struct {
	Dir string
}

// EnsureTable 保证 schema_migrations 表存在
func EnsureTable(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version BIGINT PRIMARY KEY,
        applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )`)
	return err
}

// AppliedVersions 已应用版本
func AppliedVersions(ctx context.Context, db *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := db.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		res[v] = true
	}
	return res, rows.Err()
}

type migrationFile struct {
	Version int64
	Path    string
}

// discoverUpMigrations 扫描目录中的 *_up.sql 按版本排序
func (r Runner) discoverUpMigrations(fsys fs.FS) ([]migrationFile, error) {
	var files []migrationFile
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if !strings.HasSuffix(name, "_up.sql") {
			return nil
		}
		// 前缀数字作为版本
		parts := strings.SplitN(name, "_", 2)
		if len(parts) == 0 {
			return nil
		}
		ver, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil
		}
		files = append(files, migrationFile{Version: ver, Path: path})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

// Up 执行未应用的向上迁移
func (r Runner) Up(ctx context.Context, db *pgxpool.Pool) error {
	if r.Dir == "" {
		return errors.New("migrations dir is empty")
	}
	if err := EnsureTable(ctx, db); err != nil {
		return err
	}
	applied, err := AppliedVersions(ctx, db)
	if err != nil {
		return err
	}
	// 使用 OS fs
	fsys := os.DirFS(r.Dir)
	ups, err := r.discoverUpMigrations(fsys)
	if err != nil {
		return err
	}
	for _, m := range ups {
		if applied[m.Version] {
			continue
		}
		content, err := fs.ReadFile(fsys, m.Path)
		if err != nil {
			return err
		}
		// 在事务中执行
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		_, execErr := tx.Exec(ctx, string(content))
		if execErr == nil {
			_, execErr = tx.Exec(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES($1,$2)`, m.Version, time.Now())
		}
		if execErr != nil {
			_ = tx.Rollback(ctx)
			return execErr
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
