package main

import (
	"context"
	"sync"
	"time"
)

// memStore es el almacen en memoria de TARGET_APP_STORE=memoria. Replica la semantica de store
// (seed, stock, 404 y 409, defecto oversell) sin base de datos, para que el handler real corra en
// pruebas de integracion que no tienen Postgres. No se usa en despliegues.
type memStore struct {
	mu       sync.Mutex
	oversell bool
	products []Product
	orders   []Order
	nextOID  int64
}

func newMemStore(oversell bool) *memStore {
	m := &memStore{oversell: oversell}
	m.reset()
	return m
}

func (m *memStore) reset() {
	m.products = nil
	for i, p := range seedProducts {
		m.products = append(m.products, Product{ID: int64(i + 1), Name: p.Name, Stock: p.Stock})
	}
	m.orders = nil
	m.nextOID = 1
}

func (m *memStore) Ready() bool                { return true }
func (m *memStore) Ping(context.Context) error { return nil }

func (m *memStore) ListProducts(context.Context) ([]Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Product(nil), m.products...), nil
}

func (m *memStore) CreateProduct(_ context.Context, name string, stock int) (Product, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := Product{ID: int64(len(m.products) + 1), Name: name, Stock: stock}
	m.products = append(m.products, p)
	return p, nil
}

func (m *memStore) ListOrders(context.Context) ([]Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Order(nil), m.orders...), nil
}

func (m *memStore) PlaceOrder(_ context.Context, productID int64, qty int) (Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := int(productID) - 1
	if productID < 1 || idx >= len(m.products) {
		return Order{}, errNotFound
	}
	if !m.oversell && m.products[idx].Stock < qty {
		return Order{}, errInsufficientStock
	}
	m.products[idx].Stock -= qty
	o := Order{ID: m.nextOID, ProductID: productID, Qty: qty, CreatedAt: time.Now().UTC()}
	m.nextOID++
	m.orders = append(m.orders, o)
	return o, nil
}

func (m *memStore) Clean(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reset()
	return nil
}

func (m *memStore) Diff(context.Context) (int, error) { return 0, nil }
