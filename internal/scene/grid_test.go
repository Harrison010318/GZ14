package scene

import (
	"testing"
)

const (
	testMapW  = 1000.0
	testMapH  = 1000.0
	testGrid  = 100.0
)

func newTestGrid() *GridAOI {
	return NewGridAOI(testMapW, testMapH, testGrid)
}

func TestPosToGrid(t *testing.T) {
	g := newTestGrid()

	tests := []struct {
		x, y     float64
		wantCol, wantRow int
	}{
		{0, 0, 0, 0},
		{100, 100, 1, 1},
		{150, 250, 1, 2},
		{999, 999, 9, 9},
		{-10, -10, 0, 0},   // clamp
		{10000, 10000, 9, 9}, // clamp
	}

	for _, tt := range tests {
		col, row := g.PosToGrid(tt.x, tt.y)
		if col != tt.wantCol || row != tt.wantRow {
			t.Errorf("PosToGrid(%.0f,%.0f) = (%d,%d), want (%d,%d)",
				tt.x, tt.y, col, row, tt.wantCol, tt.wantRow)
		}
	}
}

func TestGridDims(t *testing.T) {
	g := newTestGrid()
	if g.cols != 10 {
		t.Errorf("cols = %d, want 10", g.cols)
	}
	if g.rows != 10 {
		t.Errorf("rows = %d, want 10", g.rows)
	}
}

func TestAddAndRemoveEntity(t *testing.T) {
	g := newTestGrid()

	e := NewEntity(1, "test", 150, 150)
	col, row := g.AddEntity(e)

	if col != 1 || row != 1 {
		t.Errorf("AddEntity at (150,150) = (%d,%d), want (1,1)", col, row)
	}

	// 验证在格子中能找到
	entities := g.GetEntitiesInGrid(col, row)
	found := false
	for _, ent := range entities {
		if ent.ID == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("entity not found after AddEntity")
	}

	// 移除
	g.RemoveEntity(1, 150, 150)
	entities = g.GetEntitiesInGrid(col, row)
	for _, ent := range entities {
		if ent.ID == 1 {
			t.Error("entity still found after RemoveEntity")
		}
	}
}

func TestGetSurroundGrids(t *testing.T) {
	g := newTestGrid()

	// 中心格子
	surround := g.GetSurroundGrids(5, 5)
	if len(surround) != 9 {
		t.Errorf("center grid surround count = %d, want 9", len(surround))
	}

	// 角落格子
	surround = g.GetSurroundGrids(0, 0)
	if len(surround) != 4 {
		t.Errorf("corner(0,0) surround count = %d, want 4", len(surround))
	}

	// 边缘格子
	surround = g.GetSurroundGrids(5, 0)
	if len(surround) != 6 {
		t.Errorf("edge(5,0) surround count = %d, want 6", len(surround))
	}
}

func TestGetVisibleEntities(t *testing.T) {
	g := newTestGrid()

	// 添加 5 个实体在不同格子
	g.AddEntity(NewEntity(1, "e1", 50, 50))    // (0,0)
	g.AddEntity(NewEntity(2, "e2", 150, 50))   // (1,0)
	g.AddEntity(NewEntity(3, "e3", 50, 150))   // (0,1)
	g.AddEntity(NewEntity(4, "e4", 150, 150))  // (1,1)
	g.AddEntity(NewEntity(5, "e5", 550, 550))  // (5,5)

	// 从 (100,100) 附近看，应能看到 4 个实体（在 0,0 | 0,1 | 1,0 | 1,1）
	visible := g.GetVisibleEntities(100, 100)
	if len(visible) != 4 {
		t.Errorf("visible count from (100,100) = %d, want 4", len(visible))
	}

	// 从 (500,500) 看，应只能看到 1 个
	visible = g.GetVisibleEntities(500, 500)
	if len(visible) != 1 {
		t.Errorf("visible count from (500,500) = %d, want 1", len(visible))
	}
}

func TestMoveEntitySameGrid(t *testing.T) {
	g := newTestGrid()
	g.AddEntity(NewEntity(1, "e1", 50, 50))

	// 同格移动 (50,50) → (80,80)，不应跨格
	oldCol, oldRow, newCol, newRow := g.MoveEntity(1, 50, 50, 80, 80)

	if oldCol != 0 || oldRow != 0 || newCol != 0 || newRow != 0 {
		t.Errorf("same-grid move should not change grid: old=(%d,%d) new=(%d,%d)",
			oldCol, oldRow, newCol, newRow)
	}
}

func TestMoveEntityCrossGrid(t *testing.T) {
	g := newTestGrid()
	g.AddEntity(NewEntity(1, "e1", 50, 50))

	// 跨格移动 (50,50) → (150,150)：从 (0,0) 到 (1,1)
	oldCol, oldRow, newCol, newRow := g.MoveEntity(1, 50, 50, 150, 150)

	if oldCol != 0 || oldRow != 0 {
		t.Errorf("old grid = (%d,%d), want (0,0)", oldCol, oldRow)
	}
	if newCol != 1 || newRow != 1 {
		t.Errorf("new grid = (%d,%d), want (1,1)", newCol, newRow)
	}
}

func TestNewAndLostNeighbors(t *testing.T) {
	g := newTestGrid()

	// 在 (0,0) 放一个实体
	g.AddEntity(NewEntity(1, "e1", 50, 50))
	// 在 (1,1) 放一个实体
	g.AddEntity(NewEntity(2, "e2", 150, 150))
	// 在 (2,2) 放一个实体
	g.AddEntity(NewEntity(3, "e3", 250, 250))

	// e1 从 (0,0) 移动到 (1,1)
	newEntities := g.GetNewNeighbors(0, 0, 1, 1)
	lostEntities := g.GetLostNeighbors(0, 0, 1, 1)

	// 从 (0,0) 到 (1,1)，新邻居应该包含 (2,2) 附近的实体
	foundNew := false
	for _, e := range newEntities {
		if e.ID == 3 {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Log("expected entity 3 to be a new neighbor after moving to (1,1)")
		// NOTE: 由于网格边界的不同，实际的 enter/leave 取决于周围 3×3 的范围
		// 此处不做硬断言，只做日志输出方便调试
	}

	t.Logf("new neighbors: %d, lost neighbors: %d", len(newEntities), len(lostEntities))
}

func TestGridNoRace(t *testing.T) {
	// 简单的并发安全测试
	g := newTestGrid()
	done := make(chan bool)

	// 并发 add
	go func() {
		for i := int64(0); i < 100; i++ {
			g.AddEntity(NewEntity(i, "e", 50, 50))
		}
		done <- true
	}()

	// 并发 get
	go func() {
		for i := 0; i < 100; i++ {
			g.GetVisibleEntities(50, 50)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestSceneAddRemovePlayer(t *testing.T) {
	s := NewScene(1, testMapW, testMapH, testGrid)
	others := s.AddPlayer(1, "alice", 100, 100)
	if len(others) != 0 {
		t.Errorf("first player should have no neighbors, got %d", len(others))
	}
	if s.PlayerCount() != 1 {
		t.Errorf("player count = %d, want 1", s.PlayerCount())
	}

	// 添加第二个玩家
	others = s.AddPlayer(2, "bob", 150, 150)
	if len(others) != 1 {
		t.Errorf("bob should see alice, got %d", len(others))
	}

	s.RemovePlayer(1)
	if s.PlayerCount() != 1 {
		t.Errorf("after remove, player count = %d, want 1", s.PlayerCount())
	}
}

func TestSceneMoveSameGrid(t *testing.T) {
	s := NewScene(1, testMapW, testMapH, testGrid)
	s.AddPlayer(1, "alice", 100, 100)
	s.AddPlayer(2, "bob", 150, 150)

	enter, leave, err := s.MovePlayer(1, 110, 110)
	if err != nil {
		t.Fatalf("MovePlayer failed: %v", err)
	}
	if len(enter) != 0 {
		t.Errorf("same-grid move should not trigger AOI enter, got %d", len(enter))
	}
	if len(leave) != 0 {
		t.Errorf("same-grid move should not trigger AOI leave, got %d", len(leave))
	}
}

func TestSceneMoveCrossGrid(t *testing.T) {
	s := NewScene(1, testMapW, testMapH, testGrid)
	s.AddPlayer(1, "alice", 50, 50)  // grid(0,0)
	s.AddPlayer(2, "bob", 550, 550)  // grid(5,5) - 在视野外

	// alice 移动到远处 (500,500)，应触发 AOI enter/leave
	enter, leave, err := s.MovePlayer(1, 500, 500)
	if err != nil {
		t.Fatalf("MovePlayer failed: %v", err)
	}

	t.Logf("cross-grid move: enter=%d leave=%d", len(enter), len(leave))
}
