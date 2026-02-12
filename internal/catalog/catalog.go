package catalog

import (
	"fmt"
	"minidb/pkg/types"
	"sync"
	"sync/atomic"
)

type TableInfo struct {
	Name    string
	Schema  types.Schema
	ID      types.TableID
	Indexes map[string]string // ColName -> IndexName
}

type Catalog struct {
	mu          sync.RWMutex
	tables      map[string]types.TableID // Name -> ID
	tableInfos  map[types.TableID]*TableInfo
	nextTableID uint64
}

func NewCatalog() *Catalog {
	return &Catalog{
		tables:     make(map[string]types.TableID),
		tableInfos: make(map[types.TableID]*TableInfo),
		// nextTableID starts at 0, atomic add will make first ID 1
	}
}

func (c *Catalog) CreateTable(name string, schema types.Schema) (types.TableID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.tables[name]; exists {
		return 0, fmt.Errorf("table %s already exists", name)
	}

	id := types.TableID(atomic.AddUint64(&c.nextTableID, 1))

	info := &TableInfo{
		Name:    name,
		Schema:  schema,
		ID:      id,
		Indexes: make(map[string]string),
	}
	c.tableInfos[id] = info
	c.tables[name] = id

	return id, nil
}

func (c *Catalog) GetTableByName(name string) (*TableInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	id, ok := c.tables[name]
	if !ok {
		return nil, fmt.Errorf("table %s not found", name)
	}
	return c.tableInfos[id], nil
}

func (c *Catalog) GetTableByID(id types.TableID) (*TableInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	info, ok := c.tableInfos[id]
	if !ok {
		return nil, fmt.Errorf("table %d not found", id)
	}
	return info, nil
}

// CreateIndex adds an index entry to the catalog.
func (c *Catalog) CreateIndex(tableName string, colName string, idxName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tid, ok := c.tables[tableName]
	if !ok {
		return fmt.Errorf("table %s not found", tableName)
	}

	info := c.tableInfos[tid]
	info.Indexes[colName] = idxName
	return nil
}
