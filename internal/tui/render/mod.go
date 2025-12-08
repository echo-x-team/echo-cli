package render

// Insets 用于描述内边距。
type Insets struct {
	Top    int
	Left   int
	Right  int
	Bottom int
}

// TLBR 通过 top/left/bottom/right 构造 Insets。
func TLBR(top, left, bottom, right int) Insets {
	return Insets{Top: top, Left: left, Bottom: bottom, Right: right}
}

// VH 通过垂直/水平值构造 Insets。
func VH(vertical, horizontal int) Insets {
	return Insets{Top: vertical, Left: horizontal, Bottom: vertical, Right: horizontal}
}

// Rect 表示矩形区域。
type Rect struct {
	X, Y          int
	Width, Height int
}

// Inset 按内边距收紧矩形，使用饱和计算避免下溢。
func (r Rect) Inset(in Insets) Rect {
	w := r.Width - in.Left - in.Right
	h := r.Height - in.Top - in.Bottom
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return Rect{
		X:      r.X + in.Left,
		Y:      r.Y + in.Top,
		Width:  w,
		Height: h,
	}
}
