package scene

import (
	"sync"
	"sync/atomic"
)

// Entity 场景中的实体（玩家）
type Entity struct {
	ID       int64
	Name     string
	Position Position
	Data     interface{} // 扩展数据
}

// Position 位置
type Position struct {
	X float64
	Y float64
}

// EntityID 全局实体 ID 生成器
var entityIDGen atomic.Int64

func NewEntity(id int64, name string, x, y float64) *Entity {
	return &Entity{
		ID:   id,
		Name: name,
		Position: Position{
			X: x,
			Y: y,
		},
	}
}

// EntityManager 实体管理器
type EntityManager struct {
	entities map[int64]*Entity
	mutex    sync.RWMutex
}

func NewEntityManager() *EntityManager {
	return &EntityManager{
		entities: make(map[int64]*Entity),
	}
}

func (em *EntityManager) Add(e *Entity) {
	em.mutex.Lock()
	em.entities[e.ID] = e
	em.mutex.Unlock()
}

// AddOrUpdate 添加或更新实体（重连时使用，覆盖已有实体）
func (em *EntityManager) AddOrUpdate(e *Entity) {
	em.mutex.Lock()
	em.entities[e.ID] = e
	em.mutex.Unlock()
}

func (em *EntityManager) Remove(id int64) {
	em.mutex.Lock()
	delete(em.entities, id)
	em.mutex.Unlock()
}

func (em *EntityManager) Get(id int64) *Entity {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	return em.entities[id]
}

func (em *EntityManager) Count() int {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	return len(em.entities)
}
