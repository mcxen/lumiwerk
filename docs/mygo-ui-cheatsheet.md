# MyGo UI 速查表（Lumiwerk 专用）

范围：MyGo v0.7.3 的 `github.com/egoist/mygo/ui`。本表只收**在本仓库中已经 grep 验证过**的 API，
签名抄自 `~/go/pkg/mod/github.com/egoist/mygo@v0.7.3/ui/*.go`，用例抄自 `examples/gallery/main.go`
或本仓库 `src/`。

模块路径：`github.com/egoist/mygo/ui`，包名 `ui`；源码在 `package main` 里直接 import。

---

## 1. 布局与尺寸

### 1.1 容器

```go
func Row(c *Context) Element           // 横向布局
func Column(c *Context) Element        // 纵向布局
func Box(c *Context) Element           // 空白盒子（最轻的容器）
func Spacer(c *Context) Element        // 弹性占位（Grow(1) 语义）
func Grid(c *Context) Element          // CSS Grid 容器
func Scroll(c *Context) Element        // 垂直滚动容器
func ScrollBoth(c *Context) Element    // 双向滚动容器
func Split(c *Context, size *float32, first func(), second func()) Element
func SplitVertical(c *Context, size *float32, first func(), second func()) Element
```

容器靠 `Children(fn)` 填内容；`fn` 里只声明子元素，顺序即布局顺序。

```go
ui.Column(c).Grow(1).MinWidth(0).Padding(24, 24, 28, 24).Children(func() {
	ui.Text(c, "标题")
})
```

### 1.2 尺寸与间距（Element 方法）

```go
func (e Element) Width(v float32) Element
func (e Element) Height(v float32) Element
func (e Element) Size(w, h float32) Element
func (e Element) MinWidth(v float32) Element
func (e Element) MaxWidth(v float32) Element
func (e Element) MinHeight(v float32) Element
func (e Element) MaxHeight(v float32) Element
func (e Element) Grow(f float32) Element      // 占据剩余空间的比例
func (e Element) Shrink(f float32) Element    // 收缩系数，0 表示不收缩
func (e Element) Fill() Element               // 宽高都填满父容器
func (e Element) Padding(v ...float32) Element // 1 个=四边，2 个=上下/左右，3 个=上/左右/下，4 个=上右下左
func (e Element) Margin(v ...float32) Element  // 同上
func (e Element) Gap(v float32) Element        // 子元素间距
func (e Element) GapX(v float32) Element
func (e Element) GapY(v float32) Element
func (e Element) AlignItems(a Align) Element   // ui.Start/Center/End/Stretch
func (e Element) AlignContent(a Align) Element
func (e Element) AlignSelf(a Align) Element
func (e Element) Justify(a Align) Element      // 主轴分布
func (e Element) Radius(r ...float32) Element  // 圆角
func (e Element) Clip() Element                // 裁剪子元素
func (e Element) Absolute() Element           // 绝对定位
func (e Element) Left(v float32) Element
func (e Element) Top(v float32) Element
func (e Element) Right(v float32) Element
func (e Element) Bottom(v float32) Element
func (e Element) Translate(x, y float32) Element
func (e Element) Scale(x, y float32) Element
func (e Element) Rotate(degrees float32) Element
func (e Element) Opacity(o float32) Element
func (e Element) AspectRatio(r float32) Element
func (e Element) Columns(n int) Element        // Grid 列数
func (e Element) Rows(n int) Element          // Grid 行数
```

本仓库典型写法（`src/settings_view.go`）：

```go
ui.Column(c).Width(212).Shrink(0).Padding(12, 10, 12, 10).Gap(2).Children(func() { ... })
```

### 1.3 滚动与虚拟列表

```go
func Scroll(c *Context) Element
func (e Element) TrackScroll(s *ScrollState) Element   // 绑定滚动状态
type ScrollState struct { Handle; X, Y float32; MaxX, MaxY float32; ... }
func (e Element) ScrollIntoView() Element
```

```go
scroll := ui.Scroll(c).Fill().TrackScroll(&a.browseScroll)
scroll.Children(func() { ... })
```

---

## 2. 样式与间距

```go
func (e Element) Background(c Color) Element
func (e Element) Border(width float32, c Color) Element
func (e Element) BorderWidth(v ...float32) Element
func (e Element) BorderColor(c Color) Element
func (e Element) Shadow(x, y, blur, spread float32, c Color) Element
func (e Element) InsetShadow(x, y, blur, spread float32, c Color) Element
func (e Element) Gradient(from, to Color, angle float32) Element
func (e Element) Opacity(o float32) Element
func (e Element) Draw(fn func(p *Painter, r Rect)) Element   // 自绘
```

阴影写法（`src/settings_chrome.go`）：

```go
e.Shadow(0, 1, 2, 0, pal.cardShadow).Shadow(0, 1, 6, 0, pal.cardShadow.Alpha(0.5))
```

主题（`ui.Theme`）字段：`Dark, Background, Surface, SurfaceHover, SurfacePressed, Border,
Text, TextMuted, Accent, AccentHover, AccentPressed, AccentText, Danger, Warning, Success,
Selection, Focus, Inverse, InverseText, Scrollbar, ScrollbarWidth, Radius, Spacing, FontSize, Font`。

```go
t := *c.Theme()          // 复制一份改
t.Accent = pal.accent
c.SetTheme(&t)           // 只影响当前视图
```

---

## 3. 文本与字体

```go
func Text(c *Context, s string) Element
func Textf(c *Context, format string, args ...any) Element
func RichText(c *Context, spans ...Span) Element
func EditableText(c *Context, value *string) Element
```

字体相关（Element 方法）：

```go
func (e Element) FontSize(v float32) Element
func (e Element) FontWeight(w int) Element
func (e Element) Font(family string) Element
func (e Element) Bold() Element
func (e Element) Italic() Element
func (e Element) Underline() Element
func (e Element) Strikethrough() Element
func (e Element) LineHeight(m float32) Element
func (e Element) LetterSpacing(v float32) Element
func (e Element) TextAlign(a Align) Element
func (e Element) TextColor(c Color) Element
func (e Element) TextBackground(c Color) Element
func (e Element) SingleLine() Element
func (e Element) MaxLines(n int) Element
func (e Element) Lines(min, max int) Element
func (e Element) Ellipsis(s string) Element   // 截断样式
func (e Element) Label(s string) Element       // 无障碍标签
```

```go
ui.Text(c, "目录为空").FontSize(16).Bold().TextColor(pal.text)
ui.Text(c, path).FontSize(13).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
```

---

## 4. 颜色

```go
func Hex(s string) Color          // "#1d1d1f"
func RGB(r, g, b uint8) Color
func RGBA(r, g, b uint8, alpha float32) Color
func (c Color) Alpha(a float32) Color
func (c Color) Mix(o Color, t float32) Color
func (c Color) Over(base Color) Color
```

```go
textMuted := ui.Hex("#6e6e78")
chip := ui.RGBA(0, 0, 0, 0.045)
hover := chip.Mix(pal.text, 0.06)
```

---

## 5. 常用控件

### 5.1 按钮

```go
func Button(c *Context, label string) Element
func PrimaryButton(c *Context, label string) Element
func ButtonBase(c *Context) Element
func (e Element) Clicked() bool
func (e Element) DoubleClicked() bool
func (e Element) RightClicked() bool
func (e Element) Pressed() bool
func (e Element) Hovered() bool
func (e Element) Disabled(d bool) Element
func (e Element) Tooltip(s string) Element
```

```go
if ui.Button(c.Key("settings-save"), "应用").Clicked() { ... }
```

### 5.2 输入

```go
func TextInput(c *Context, value *string) Element
func TextInputBase(c *Context, value *string) Element
func TextArea(c *Context, value *string) Element
func SearchField(c *Context, query *string) Element
func NumberInput(c *Context, value *float64, lo, hi, step float64) Element
func (e Element) Password() Element
func (e Element) Placeholder(s string) Element
func (e Element) ReadOnly(on bool) Element
func (e Element) Changed() bool
func (e Element) Submitted() bool
```

```go
ui.TextInput(c, &a.Settings.QuickRule).Grow(1).MinWidth(160)
search := ui.SearchField(c.Key("query"), &a.Query).Width(150)
```

### 5.3 选择与开关

```go
func Checkbox(c *Context, checked *bool, label string) Element
func Toggle(c *Context, on *bool, label string) Element
func Switch(c *Context, on *bool) Element
func SwitchBase(c *Context, on *bool) Element
func Radio[T comparable](c *Context, selected *T, value T, label string) Element
func Segmented(c *Context, selected *int, labels ...string) Element
func SegmentedBase(c *Context, selected *int, n int) SegmentedParts
func Select(c *Context, selected *string, options []string) Element
func SelectBase[T comparable](c *Context, selected *T) SelectParts[T]
func Combobox(c *Context, selected *string, options []string) Element
func Rating(c *Context, value *int, max int) Element
```

```go
ui.Radio(c, &a.Settings.EnterOpens, false, "重命名")
ui.Radio(c, &a.Settings.EnterOpens, true, "打开 / 关闭")
```

### 5.4 滑块与进度

```go
func Slider(c *Context, value *float64, lo, hi float64) Element
func SliderBase(c *Context, value *float64, lo, hi float64) Element
func StepSlider(c *Context, value *float64, lo, hi, step float64) Element
func RangeSlider(c *Context, low, high *float64, lo, hi, step float64) Element
func Progress(c *Context, value float64) Element
func Spinner(c *Context) Element
func Meter(c *Context, value, lo, hi float64, levels *MeterLevels) Element
```

```go
ui.Slider(c, &a.Settings.Thumb, 64, 512).Width(180).Label("默认缩略图")
```

### 5.5 列表与网格

```go
func List(c *Context, s *ListState, n int, row ...func(int)) Element
func GridView(c *Context, s *GridState, n int, minWidth, height float32, item func(i int)) Element
func Outline[K comparable](c *Context, s *OutlineState[K], roots []K, children func(K) []K, row func(K)) Element
func Table(c *Context, s *ListState, columns []TableColumn, n int, cell func(row, col int)) Element
type ListState struct { Handle; Key func(row int) any; ... }
type GridState struct { Handle; Key func(item int) any; Selected *int; ... }
```

```go
state := a.mediaListState()
ui.List(c, state.List, len(a.Visible), func(i int) { ... })
grid := ui.GridView(c, state.Grid, len(a.Visible), float32(cell), float32(cell+30), func(i int) { ... })
```

**行 key 必须是字符串**：`ui.Selection[string]` 要求每个网格/列表行的 key 是 string，
用 `appState.mediaListState()` 取（见 `src/selection.go`）。

### 5.6 导航与结构

```go
func Tabs(c *Context, selected *int, labels ...string) Element
func Breadcrumbs(c *Context, items []string, chosen *int) Element
func Sidebar(c *Context, selected *string, fn func()) Element
func SidebarItem(c *Context, id string, icon *SVG, label string) Element
func Tree(c *Context, fn func()) Element
func TreeItem(c *Context, label string, open *bool, children func()) Element
func Accordion(c *Context, fn func()) Element
func Form(c *Context, fn func()) Element
func Field(c *Context, label string, fn func()) Element
func Divider(c *Context) Element
func Badge(c *Context, text string) Element
```

```go
ui.Tabs(c, &g.tab, "Files", "Search", "History")
if ui.Breadcrumbs(c, items, &chosen).Label("路径").Grow(1).MinWidth(160).Changed() { ... }
```

### 5.7 图标与图片

```go
func Icon(c *Context, s *SVG) Element
func Image(c *Context, src ImageSource) Element
func ImageSize(src ImageSource) (w, h float32)
func MustParseSVG(data []byte) *SVG
```

```go
ui.Icon(c, lucideChevronDown).Size(16, 16).TextColor(pal.text.Alpha(0.85))
```

---

## 6. 覆盖层

```go
func Overlay(c *Context, fn func())                      // 全屏覆盖层根
func Popover(c *Context, anchor Element, open *bool, fn func()) Element
func PopoverBase(c *Context, anchor Element, open *bool, fn func(panel Element)) Element
func Modal(c *Context, open *bool, fn func()) Element   // 点外面/Esc 关闭
func DialogBase(c *Context, open *bool, fn func(backdrop Element, panel Element)) Element
func AlertDialog(c *Context, open *bool, title, message string, buttons ...string) int
func TooltipBase(c *Context, anchor Element, fn func(tip Element)) Element
func (c *Context) Toast(message string)
func (c *Context) ToastAction(message, label string, action func())
```

```go
ui.Modal(c, &g.dialog, func() {
	ui.Text(c, "A modal dialog").FontSize(18).Bold()
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if ui.PrimaryButton(c, "Done").Clicked() { g.dialog = false }
	})
})
if ui.AlertDialog(c, &g.alert, "Delete “Notes”?", "This deletes the note…", "Cancel", "Delete") == 1 { ... }
```

---

## 7. 动画

```go
type Animation struct { Duration time.Duration; Ease Easing; Spring SpringOptions; Delay time.Duration }
type SpringOptions struct { Duration time.Duration; Bounce float64; Mass, Stiffness, Damping float64; Precision float64; Velocity float32 }
var Smooth, Snappy, Bouncy Animation
func EaseIn(t float32) float32
func EaseOut(t float32) float32
func EaseInOut(t float32) float32
func Linear(t float32) float32
func CubicBezier(x1, y1, x2, y2 float32) Easing
```

Element 动画方法：

```go
func (e Element) AnimateTo(key any, target float32, a Animation) float32
func (e Element) Animate(key any, target float32, d time.Duration) float32
func (e Element) AnimateColor(key any, target Color, a Animation) Color
func (e Element) Spring(key any, target float32, options ...SpringOptions) float32
func (e Element) Transition(t ElementTransition) Element
```

```go
item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
x := sw.AnimateTo("knob", b2f(*on)*16, ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.15}})
```

---

## 8. 事件与输入

```go
func (e Element) Key(key any) Element          // 构造前调用，绑定身份
func (e Element) OnClick(fn func()) Element
func (e Element) OnChange(fn func()) Element
func (e Element) OnSubmit(fn func()) Element
func (e Element) OnShortcut(mods Modifiers, key Key, fn func()) Element
func (e Element) ContextMenu(build func(m *Menu)) Element
func (e Element) Menu(build func(m *Menu)) Element
func (e Element) HandleInput(fn func(ev InputEvent) bool) Element
func (e Element) Drag(value any) Element
func (e Element) DroppedFiles() []string
func (e Element) Focus() Element
func (e Element) Focused() bool
func (e Element) FocusVisible() bool
func (e Element) AutoFocus() Element
func (e Element) Access(fn func(n *platform.AccessNode)) Element
```

`Context` 上的输入与调度：

```go
func (c *context) Shortcut(mods Modifiers, key Key) bool
func (c *context) After(d time.Duration)
func (c *context) Now() time.Time
func (c *context) Size() (width, height float32)
func (c *context) WriteClipboard(s string)
func (c *context) ReadClipboard() string
```

菜单：

```go
func MenuButton(c *Context, label string, build func(m *Menu)) Element
func (m *Menu) Item(label string) *MenuItem
func (m *Menu) Separator()
func (m *Menu) Submenu(label string, build func(m *Menu)) *MenuItem
func (it *MenuItem) Checked(on bool) *MenuItem
func (it *MenuItem) Radio(on bool) *MenuItem
func (it *MenuItem) Shortcut(mods Modifiers, key Key) *MenuItem
func (it *MenuItem) Disabled(d bool) *MenuItem
func (it *MenuItem) Chosen() bool
```

```go
ui.MenuButton(c, a.filterLabel(), func(m *ui.Menu) {
	if m.Item("全部类型").Chosen() { ... }
})
```

---

## 9. 本项目踩过的坑

1. **`Key` 必须在构造前调用。** `ui.TextArea(c.Key("report"), &text)` 合法；
   构造后 `.Key` 会 panic。`TextArea`/`TextInput`/`SearchField`/列表这类有状态控件尤其如此。
2. **列表/网格的行 key 必须是字符串。** MyGo 的 `ui.Selection[string]` 要求每个 row key 是
   string，用 `appState.mediaListState()`，陈旧索引返回 `!missing:<index>`，不要回退成整数。
3. **`Context` 是 `*ui.Context`，`Element` 的方法都是值接收者。** `ui.Context` 由 MyGo 框架传入，
   不要自己 new；`c.Theme()` 返回的是当前主题的指针，改之前先复制一份。
4. **`c.SetTheme` 只影响当前视图树**，不影响其他窗口。
5. **窗口操作只在 MyGo 主线程。** 后台 goroutine 里用 `appState.update(func(){...})` 调度，
   且 `update` 要容忍 `a.Window == nil`。
6. **Objective-C 调用必须包在 `withAutoreleasePool` 里**，它内部 `runtime.LockOSThread`。
7. **新视图走 `safeView`，新 goroutine 走 `safeGo`**，panic 会被捕获并写进诊断日志。
8. **`ui.Menu` 的回调是“菜单项被选中”语义**，写法是
   `if m.Item("全部").Checked(cond).Chosen() { ... }`，不是把逻辑写进 Item 的参数里。
9. **`settingsPopup` 这类自定义控件依赖 `ui.SelectBase`**，切换时 `*value` 会立即被改写，
   要在同一个 frame 内读取，不要缓存旧值。
10. **`Tooltip` 是 Element 方法**（`ui.Button(...).Tooltip("适应窗口")`），
    自定义 tooltip 用 `ui.TooltipBase(c, anchor, func(tip ui.Element){...})`。
11. **`ui.Icon` 需要 `*ui.SVG`**，图标是 `MustParseSVG` 出来的常量；
    `Size(w,h)` 决定绘制尺寸，`TextColor` 决定描边颜色。
12. **`Grid` 容器和 `GridView` 控件是两回事**：前者是 CSS Grid 布局容器，
    后者是带虚拟滚动的网格控件，本项目媒体区用的是后者。

---

## 10. 验证方式

```sh
cd ~/go/pkg/mod/github.com/egoist/mygo@v0.7.3/ui
grep -rn "func (e Element) Grow" *.go       # Element 方法
grep -rn "^func Slider" *.go                # 包级函数
grep -rn "func (c \*context) Toast" *.go    # Context 方法
```

包级函数在 `ui` 包里查找，`Element` 方法在 `api_gen.go` 里（`func (_handle Element) Xxx`），
`Context` 方法在 `context.go` 里（`func (c *context) Xxx`，通过接口暴露）。
