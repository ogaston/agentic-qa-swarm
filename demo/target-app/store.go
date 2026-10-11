package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" para database/sql
)

// baselineVersion es lo que imprime `GET /__aqs/baseline/version` (contrato de go-reset: version).
const baselineVersion = "ref-v1"

var (
	errNotFound          = errors.New("producto no encontrado")
	errInsufficientStock = errors.New("stock insuficiente")
)

// Datos fijos del baseline. Tras `clean` la base queda exactamente con estas filas (ids 1 y 2).
var seedProducts = []struct {
	Name  string
	Stock int
}{
	{"cafe", 10},
	{"te", 5},
}

type store struct {
	db       *sql.DB
	oversell bool // TARGET_APP_BUG=oversell: el defecto sembrado (la invariante no se aplica)
}

type Product struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Stock int    `json:"stock"`
}

type Order struct {
	ID        int64     `json:"id"`
	ProductID int64     `json:"product_id"`
	Qty       int       `json:"qty"`
	CreatedAt time.Time `json:"created_at"`
}

func openStore(ctx context.Context, dsn string, oversell bool) (*store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		if err := db.PingContext(pctx); err == nil {
			break
		} else if pctx.Err() != nil {
			_ = db.Close()
			return nil, err
		}
		time.Sleep(time.Second)
	}
	return &store{db: db, oversell: oversell}, nil
}

func (s *store) Close() error { return s.db.Close() }

func (s *store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Migrate crea el esquema y siembra los datos fijos si la base esta vacia.
// La invariante de stock vive en la app, no como CHECK de la base: el defecto sembrado la salta.
func (s *store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS products (
			id    BIGSERIAL PRIMARY KEY,
			name  TEXT NOT NULL,
			stock INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id         BIGSERIAL PRIMARY KEY,
			product_id BIGINT NOT NULL REFERENCES products(id),
			qty        INTEGER NOT NULL CHECK (qty > 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM products`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return s.seed(ctx)
	}
	return nil
}

func (s *store) seed(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, p := range seedProducts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO products (name, stock) VALUES ($1, $2)`, p.Name, p.Stock); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Clean deja la base en el baseline: vacia pedidos y productos, reinicia ids y vuelve a sembrar.
func (s *store) Clean(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `TRUNCATE orders, products RESTART IDENTITY`); err != nil {
		return err
	}
	for _, p := range seedProducts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO products (name, stock) VALUES ($1, $2)`, p.Name, p.Stock); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Diff cuenta las filas que difieren del baseline: pedidos extra, productos de seed alterados o
// ausentes, y productos extra. 0 = limpia.
func (s *store) Diff(ctx context.Context) (int, error) {
	const q = `
SELECT
  (SELECT count(*) FROM orders)
  + (SELECT count(*)
       FROM (VALUES (1::bigint, 'cafe', 10), (2::bigint, 'te', 5)) AS seed(id, name, stock)
       LEFT JOIN products p ON p.id = seed.id
      WHERE p.id IS NULL OR p.name <> seed.name OR p.stock <> seed.stock)
  + (SELECT count(*) FROM products WHERE id NOT IN (1, 2))`
	var n int
	if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, stock FROM products ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Stock); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *store) CreateProduct(ctx context.Context, name string, stock int) (Product, error) {
	var p Product
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO products (name, stock) VALUES ($1, $2) RETURNING id, name, stock`, name, stock).
		Scan(&p.ID, &p.Name, &p.Stock)
	return p, err
}

func (s *store) ListOrders(ctx context.Context) ([]Order, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, product_id, qty, created_at FROM orders ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.ProductID, &o.Qty, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PlaceOrder descuenta stock y registra el pedido en una transaccion. Con el defecto sembrado no
// comprueba el stock: el resultado puede ser negativo.
func (s *store) PlaceOrder(ctx context.Context, productID int64, qty int) (Order, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var stock int
	err = tx.QueryRowContext(ctx, `SELECT stock FROM products WHERE id = $1 FOR UPDATE`, productID).Scan(&stock)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if !s.oversell && stock < qty {
		return Order{}, errInsufficientStock
	}
	if _, err := tx.ExecContext(ctx, `UPDATE products SET stock = stock - $2 WHERE id = $1`, productID, qty); err != nil {
		return Order{}, err
	}
	o := Order{ProductID: productID, Qty: qty}
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO orders (product_id, qty) VALUES ($1, $2) RETURNING id, created_at`, productID, qty).
		Scan(&o.ID, &o.CreatedAt); err != nil {
		return Order{}, err
	}
	return o, tx.Commit()
}
