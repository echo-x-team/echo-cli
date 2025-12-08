package render

// CursorPos 代表光标位置。
type CursorPos struct {
	X int
	Y int
}

// Renderable 统一的可渲染抽象。
type Renderable interface {
	Render(area Rect, buf *Buffer)
	DesiredHeight(width int) int
	CursorPos(area Rect) *CursorPos
}

// baseRenderable 提供默认空实现。
type baseRenderable struct{}

func (baseRenderable) CursorPos(Rect) *CursorPos { return nil }

// StaticLines 用于包装已准备好的行。
type StaticLines []Line

func (s StaticLines) Render(area Rect, buf *Buffer) {
	lines := []Line(s)
	if area.Height > 0 && len(lines) > area.Height {
		lines = lines[:area.Height]
	}
	buf.WriteLines(lines...)
}

func (s StaticLines) DesiredHeight(int) int {
	return len(s)
}

func (s StaticLines) CursorPos(Rect) *CursorPos { return nil }

// ColumnRenderable 垂直堆叠子元素。
type ColumnRenderable struct {
	children []Renderable
}

// NewColumn 创建空列。
func NewColumn() *ColumnRenderable {
	return &ColumnRenderable{children: []Renderable{}}
}

// WithColumnChildren 便捷构造列。
func WithColumnChildren(children ...Renderable) *ColumnRenderable {
	return &ColumnRenderable{children: append([]Renderable{}, children...)}
}

// Push 添加子元素。
func (c *ColumnRenderable) Push(child Renderable) {
	if c == nil || child == nil {
		return
	}
	c.children = append(c.children, child)
}

// Render 依次渲染子元素。
func (c *ColumnRenderable) Render(area Rect, buf *Buffer) {
	if c == nil {
		return
	}
	y := area.Y
	for _, child := range c.children {
		height := child.DesiredHeight(area.Width)
		childArea := Rect{X: area.X, Y: y, Width: area.Width, Height: height}
		child.Render(childArea, buf)
		y += height
		if area.Height > 0 && y-area.Y >= area.Height {
			break
		}
	}
}

// DesiredHeight 返回所有子元素高度之和。
func (c *ColumnRenderable) DesiredHeight(width int) int {
	if c == nil {
		return 0
	}
	total := 0
	for _, child := range c.children {
		total += child.DesiredHeight(width)
	}
	return total
}

// CursorPos 返回第一个包含光标的子元素位置。
func (c *ColumnRenderable) CursorPos(area Rect) *CursorPos {
	if c == nil {
		return nil
	}
	y := area.Y
	for _, child := range c.children {
		height := child.DesiredHeight(area.Width)
		childArea := Rect{X: area.X, Y: y, Width: area.Width, Height: height}
		if pos := child.CursorPos(childArea); pos != nil {
			return pos
		}
		y += height
	}
	return nil
}

// RowRenderable 按固定宽度水平布局子元素。
type RowRenderable struct {
	children []rowChild
}

type rowChild struct {
	width int
	item  Renderable
}

// NewRow 创建空行。
func NewRow() *RowRenderable {
	return &RowRenderable{}
}

// Push 添加一个固定宽度的子元素。
func (r *RowRenderable) Push(width int, child Renderable) {
	if r == nil || child == nil {
		return
	}
	r.children = append(r.children, rowChild{width: width, item: child})
}

// Render 渲染水平排列的子元素。
func (r *RowRenderable) Render(area Rect, buf *Buffer) {
	if r == nil {
		return
	}
	x := area.X
	for _, child := range r.children {
		if x-area.X >= area.Width {
			break
		}
		w := child.width
		if w <= 0 || w > area.Width-(x-area.X) {
			w = area.Width - (x - area.X)
		}
		childArea := Rect{X: x, Y: area.Y, Width: w, Height: area.Height}
		child.item.Render(childArea, buf)
		x += w
	}
}

// DesiredHeight 返回最大子元素高度。
func (r *RowRenderable) DesiredHeight(width int) int {
	if r == nil || len(r.children) == 0 {
		return 0
	}
	remain := width
	maxHeight := 0
	for _, child := range r.children {
		w := child.width
		if w <= 0 || w > remain {
			w = remain
		}
		h := child.item.DesiredHeight(w)
		if h > maxHeight {
			maxHeight = h
		}
		remain -= w
		if remain <= 0 {
			break
		}
	}
	return maxHeight
}

// CursorPos 返回行中第一个包含光标的子元素位置。
func (r *RowRenderable) CursorPos(area Rect) *CursorPos {
	if r == nil {
		return nil
	}
	x := area.X
	for _, child := range r.children {
		if x-area.X >= area.Width {
			break
		}
		w := child.width
		if w <= 0 || w > area.Width-(x-area.X) {
			w = area.Width - (x - area.X)
		}
		childArea := Rect{X: x, Y: area.Y, Width: w, Height: area.Height}
		if pos := child.item.CursorPos(childArea); pos != nil {
			return pos
		}
		x += w
	}
	return nil
}

// FlexRenderable 按 flex 权重分配高度。
type FlexRenderable struct {
	children []flexChild
}

type flexChild struct {
	flex int
	item Renderable
}

// NewFlex 创建空的 Flex 容器。
func NewFlex() *FlexRenderable {
	return &FlexRenderable{}
}

// Push 添加子元素及其 flex 值。
func (f *FlexRenderable) Push(flex int, child Renderable) {
	if f == nil || child == nil {
		return
	}
	f.children = append(f.children, flexChild{flex: flex, item: child})
}

// Render 根据 flex 分配剩余高度后渲染。
func (f *FlexRenderable) Render(area Rect, buf *Buffer) {
	if f == nil {
		return
	}
	rects := f.allocate(area)
	for i, child := range f.children {
		child.item.Render(rects[i], buf)
	}
}

// DesiredHeight 计算总高度。
func (f *FlexRenderable) DesiredHeight(width int) int {
	if f == nil {
		return 0
	}
	rects := f.allocate(Rect{Width: width, Height: int(^uint(0) >> 1)})
	if len(rects) == 0 {
		return 0
	}
	last := rects[len(rects)-1]
	return last.Y + last.Height
}

func (f *FlexRenderable) CursorPos(area Rect) *CursorPos {
	if f == nil {
		return nil
	}
	rects := f.allocate(area)
	for i, child := range f.children {
		if pos := child.item.CursorPos(rects[i]); pos != nil {
			return pos
		}
	}
	return nil
}

func (f *FlexRenderable) allocate(area Rect) []Rect {
	if f == nil || area.Height <= 0 || len(f.children) == 0 {
		return nil
	}
	rects := make([]Rect, len(f.children))
	childHeights := make([]int, len(f.children))
	totalFixed := 0
	totalFlex := 0
	for i, child := range f.children {
		if child.flex <= 0 {
			h := child.item.DesiredHeight(area.Width)
			childHeights[i] = h
			totalFixed += h
		} else {
			totalFlex += child.flex
		}
	}
	free := area.Height - totalFixed
	if free < 0 {
		free = 0
	}
	allocatedFlex := 0
	for i, child := range f.children {
		if child.flex > 0 {
			h := free * child.flex / maxInt(1, totalFlex)
			if i == len(f.children)-1 {
				h = free - allocatedFlex
			}
			allocatedFlex += h
			desired := child.item.DesiredHeight(area.Width)
			if desired < h {
				h = desired
			}
			childHeights[i] = h
		}
	}
	y := area.Y
	for i, h := range childHeights {
		rects[i] = Rect{X: area.X, Y: y, Width: area.Width, Height: h}
		y += h
	}
	return rects
}

// InsetRenderable 为子元素应用内边距。
type InsetRenderable struct {
	child  Renderable
	insets Insets
}

// NewInset 创建带内边距的 Renderable。
func NewInset(child Renderable, insets Insets) *InsetRenderable {
	return &InsetRenderable{child: child, insets: insets}
}

func (i *InsetRenderable) Render(area Rect, buf *Buffer) {
	if i == nil || i.child == nil {
		return
	}
	i.child.Render(area.Inset(i.insets), buf)
}

func (i *InsetRenderable) DesiredHeight(width int) int {
	if i == nil || i.child == nil {
		return 0
	}
	childHeight := i.child.DesiredHeight(width - i.insets.Left - i.insets.Right)
	return childHeight + i.insets.Top + i.insets.Bottom
}

func (i *InsetRenderable) CursorPos(area Rect) *CursorPos {
	if i == nil || i.child == nil {
		return nil
	}
	return i.child.CursorPos(area.Inset(i.insets))
}

// PlainTextRenderable 渲染纯文本。
type PlainTextRenderable struct {
	baseRenderable
	Text string
}

func (p PlainTextRenderable) Render(area Rect, buf *Buffer) {
	width := area.Width
	if width <= 0 {
		width = len(p.Text)
	}
	lines := wrapText(p.Text, width)
	for _, line := range lines {
		buf.WriteLine(Line{Spans: []Span{{Text: line}}})
	}
}

func (p PlainTextRenderable) DesiredHeight(width int) int {
	if width <= 0 {
		width = len(p.Text)
	}
	return len(wrapText(p.Text, width))
}
