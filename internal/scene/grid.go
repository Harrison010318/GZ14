package scene

import (
	"sync"
)

// GridCell 网格单元
type GridCell struct {
	entities map[int64]*Entity
}

// GridAOI 网格法 AOI 管理器
type GridAOI struct {
	mapWidth   float64
	mapHeight  float64
	gridSize   float64
	cols       int
	rows       int
	cells      [][]*GridCell
	mutex      sync.RWMutex
}

func NewGridAOI(mapWidth, mapHeight, gridSize float64) *GridAOI {
	cols := int(mapWidth / gridSize)
	if int(mapWidth)%int(gridSize) != 0 {
		cols++
	}
	rows := int(mapHeight / gridSize)
	if int(mapHeight)%int(gridSize) != 0 {
		rows++
	}

	cells := make([][]*GridCell, rows)
	for r := 0; r < rows; r++ {
		cells[r] = make([]*GridCell, cols)
		for c := 0; c < cols; c++ {
			cells[r][c] = &GridCell{
				entities: make(map[int64]*Entity),
			}
		}
	}

	return &GridAOI{
		mapWidth:  mapWidth,
		mapHeight: mapHeight,
		gridSize:  gridSize,
		cols:      cols,
		rows:      rows,
		cells:     cells,
	}
}

// PosToGrid 坐标转换为网格索引
func (g *GridAOI) PosToGrid(x, y float64) (col, row int) {
	col = int(x / g.gridSize)
	row = int(y / g.gridSize)
	if col < 0 {
		col = 0
	}
	if col >= g.cols {
		col = g.cols - 1
	}
	if row < 0 {
		row = 0
	}
	if row >= g.rows {
		row = g.rows - 1
	}
	return
}

// GetGridKeys 获取指定网格及其周围 8 格的关键字列表
func (g *GridAOI) GetSurroundGrids(col, row int) []struct{ Col, Row int } {
	result := make([]struct{ Col, Row int }, 0, 9)
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			nr := row + dr
			nc := col + dc
			if nr >= 0 && nr < g.rows && nc >= 0 && nc < g.cols {
				result = append(result, struct{ Col, Row int }{nc, nr})
			}
		}
	}
	return result
}

// AddEntity 添加实体到网格
func (g *GridAOI) AddEntity(e *Entity) (col, row int) {
	col, row = g.PosToGrid(e.Position.X, e.Position.Y)
	g.mutex.Lock()
	g.cells[row][col].entities[e.ID] = e
	g.mutex.Unlock()
	return
}

// RemoveEntity 从网格移除实体
func (g *GridAOI) RemoveEntity(id int64, x, y float64) {
	col, row := g.PosToGrid(x, y)
	g.mutex.Lock()
	delete(g.cells[row][col].entities, id)
	g.mutex.Unlock()
}

// MoveEntity 移动实体，返回旧网格和新网格（用于判断跨格）
func (g *GridAOI) MoveEntity(id int64, oldX, oldY, newX, newY float64) (oldCol, oldRow, newCol, newRow int) {
	oldCol, oldRow = g.PosToGrid(oldX, oldY)
	newCol, newRow = g.PosToGrid(newX, newY)

	if oldCol == newCol && oldRow == newRow {
		return // 同格内移动，不更新网格
	}

	g.mutex.Lock()
	delete(g.cells[oldRow][oldCol].entities, id)
	e := &Entity{ID: id, Position: Position{X: newX, Y: newY}}
	g.cells[newRow][newCol].entities[id] = e
	g.mutex.Unlock()
	return
}

// GetEntitiesInGrid 获取指定格子内的所有实体
func (g *GridAOI) GetEntitiesInGrid(col, row int) []*Entity {
	g.mutex.RLock()
	cell := g.cells[row][col]
	entities := make([]*Entity, 0, len(cell.entities))
	for _, e := range cell.entities {
		entities = append(entities, e)
	}
	g.mutex.RUnlock()
	return entities
}

// GetVisibleEntities 获取指定位置周围的可见实体
func (g *GridAOI) GetVisibleEntities(x, y float64) []*Entity {
	col, row := g.PosToGrid(x, y)
	surroundings := g.GetSurroundGrids(col, row)

	// 预分配容量
	estimated := 0
	for _, s := range surroundings {
		g.mutex.RLock()
		estimated += len(g.cells[s.Row][s.Col].entities)
		g.mutex.RUnlock()
	}

	result := make([]*Entity, 0, estimated)
	for _, s := range surroundings {
		g.mutex.RLock()
		for _, e := range g.cells[s.Row][s.Col].entities {
			result = append(result, e)
		}
		g.mutex.RUnlock()
	}
	return result
}

// GetNewNeighbors 获取新进入视野的实体（newGrid的实体中，不在oldGrid中的）
func (g *GridAOI) GetNewNeighbors(oldCol, oldRow, newCol, newRow int) []*Entity {
	oldSurround := make(map[int64]struct{})
	for _, s := range g.GetSurroundGrids(oldCol, oldRow) {
		g.mutex.RLock()
		for id := range g.cells[s.Row][s.Col].entities {
			oldSurround[id] = struct{}{}
		}
		g.mutex.RUnlock()
	}

	newEntities := make([]*Entity, 0)
	for _, s := range g.GetSurroundGrids(newCol, newRow) {
		g.mutex.RLock()
		for id, e := range g.cells[s.Row][s.Col].entities {
			if _, exists := oldSurround[id]; !exists {
				newEntities = append(newEntities, e)
			}
		}
		g.mutex.RUnlock()
	}
	return newEntities
}

// GetLostNeighbors 获取离开视野的实体（oldGrid的实体中，不在newGrid中的）
func (g *GridAOI) GetLostNeighbors(oldCol, oldRow, newCol, newRow int) []*Entity {
	newSurround := make(map[int64]struct{})
	for _, s := range g.GetSurroundGrids(newCol, newRow) {
		g.mutex.RLock()
		for id := range g.cells[s.Row][s.Col].entities {
			newSurround[id] = struct{}{}
		}
		g.mutex.RUnlock()
	}

	lostEntities := make([]*Entity, 0)
	for _, s := range g.GetSurroundGrids(oldCol, oldRow) {
		g.mutex.RLock()
		for id, e := range g.cells[s.Row][s.Col].entities {
			if _, exists := newSurround[id]; !exists {
				lostEntities = append(lostEntities, e)
			}
		}
		g.mutex.RUnlock()
	}
	return lostEntities
}
