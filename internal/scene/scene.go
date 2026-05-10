package scene

import (
	"log"
	"sync"
)

// Scene 单个地图场景
type Scene struct {
	ID    int
	aoi   *GridAOI
	em    *EntityManager
}

func NewScene(id int, mapWidth, mapHeight, gridSize float64) *Scene {
	return &Scene{
		ID:  id,
		aoi: NewGridAOI(mapWidth, mapHeight, gridSize),
		em:  NewEntityManager(),
	}
}

// AddPlayer 添加玩家到场景（重连时若玩家已存在则更新位置）
func (s *Scene) AddPlayer(id int64, name string, x, y float64) []*Entity {
	existing := s.em.Get(id)
	if existing != nil {
		// 重连：更新位置
		oldX, oldY := existing.Position.X, existing.Position.Y
		existing.Name = name
		existing.Position.X = x
		existing.Position.Y = y

		oldCol, oldRow := s.aoi.PosToGrid(oldX, oldY)
		newCol, newRow := s.aoi.PosToGrid(x, y)
		if oldCol != newCol || oldRow != newRow {
			s.aoi.RemoveEntity(id, oldX, oldY)
			s.aoi.AddEntity(existing)
		}
	} else {
		e := NewEntity(id, name, x, y)
		s.em.Add(e)
		s.aoi.AddEntity(e)
	}

	// 获取周围可见实体（不含自己）
	visible := s.aoi.GetVisibleEntities(x, y)
	others := make([]*Entity, 0, len(visible))
	for _, v := range visible {
		if v.ID != id {
			others = append(others, v)
		}
	}

	log.Printf("[Scene:%d] player enter: id=%d name=%s pos=(%.1f,%.1f) visible=%d",
		s.ID, id, name, x, y, len(others))
	return others
}

// RemovePlayer 从场景移除玩家
func (s *Scene) RemovePlayer(id int64) {
	e := s.em.Get(id)
	if e == nil {
		return
	}
	s.aoi.RemoveEntity(id, e.Position.X, e.Position.Y)
	s.em.Remove(id)
	log.Printf("[Scene:%d] player leave: id=%d", s.ID, id)
}

// MovePlayer 处理玩家移动，返回需要 AOI 事件和其他玩家需要的广播
func (s *Scene) MovePlayer(id int64, newX, newY float64) (aoiEnter []*Entity, aoiLeave []*Entity, err error) {
	e := s.em.Get(id)
	if e == nil {
		return nil, nil, ErrEntityNotFound
	}

	oldX, oldY := e.Position.X, e.Position.Y

	// 网格法移动检测
	oldCol, oldRow, newCol, newRow := s.aoi.MoveEntity(id, oldX, oldY, newX, newY)

	// 更新实体位置
	e.Position.X = newX
	e.Position.Y = newY

	// 检测跨格
	if oldCol != newCol || oldRow != newRow {
		// 获取新旧邻居变化
		aoiEnter = s.aoi.GetNewNeighbors(oldCol, oldRow, newCol, newRow)
		aoiLeave = s.aoi.GetLostNeighbors(oldCol, oldRow, newCol, newRow)
	}

	return aoiEnter, aoiLeave, nil
}

// GetVisiblePlayers 获取指定位置周围的玩家（不含自己）
func (s *Scene) GetVisiblePlayers(id int64, x, y float64) []*Entity {
	all := s.aoi.GetVisibleEntities(x, y)
	others := make([]*Entity, 0, len(all))
	for _, e := range all {
		if e.ID != id {
			others = append(others, e)
		}
	}
	return others
}

// PlayerCount 获取场景内玩家数量
func (s *Scene) PlayerCount() int {
	return s.em.Count()
}

// ========== SceneManager ==========

// SceneManager 管理所有地图场景
type SceneManager struct {
	scenes    map[int]*Scene
	mutex     sync.RWMutex
	mapWidth  float64
	mapHeight float64
	gridSize  float64
}

func NewSceneManager(mapWidth, mapHeight, gridSize float64) *SceneManager {
	sm := &SceneManager{
		scenes:    make(map[int]*Scene),
		mapWidth:  mapWidth,
		mapHeight: mapHeight,
		gridSize:  gridSize,
	}
	// 默认创建地图 1
	sm.GetOrCreateScene(1)
	return sm
}

func (sm *SceneManager) GetOrCreateScene(mapID int) *Scene {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if s, ok := sm.scenes[mapID]; ok {
		return s
	}
	s := NewScene(mapID, sm.mapWidth, sm.mapHeight, sm.gridSize)
	sm.scenes[mapID] = s
	return s
}

func (sm *SceneManager) GetScene(mapID int) *Scene {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.scenes[mapID]
}

// FindSceneByRoleID 根据角色 ID 查找所在场景
func (sm *SceneManager) FindSceneByRoleID(roleID int64) *Scene {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	for _, sc := range sm.scenes {
		if sc.em.Get(roleID) != nil {
			return sc
		}
	}
	return nil
}
