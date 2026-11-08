package qui

const wireVersion = 1

const (
	opEnd = 0
	opOpen = 1
	opFrame = 2
	opDir = 3
	opWidth = 4
	opHeight = 5
	opMinW = 6
	opMaxW = 7
	opMinH = 8
	opMaxH = 9
	opPad = 10
	opGap = 11
	opAlign = 12
	opJustify = 13
	opPos = 14
	opBg = 15
	opFg = 16
	opBorder = 17
	opRadius = 18
	opOpacity = 19
	opClip = 20
	opScroll = 21
	opHit = 22
	opRole = 23
	opLabel = 24
	opText = 25
	opFontSize = 26
	opWeight = 27
	opMono = 28
	opEllipsis = 29
	opVirtual = 30
	opItem = 31
	opSrc = 32
	opUniform = 33
	opFps = 34
)

const (
	kindBox = 0
	kindText = 1
	kindButton = 2
	kindList = 3
	kindShader = 4
)

const (
	dirColumn = 0
	dirRow = 1
	dirStack = 2
)

const (
	sizeFit = 0
	sizeFill = 1
	sizePx = 2
	sizePct = 3
)

const (
	flagHovered = 1
	flagPressed = 2
	flagClicked = 4
	flagRightClicked = 8
	flagFocused = 16
	flagDragStart = 32
	flagDragging = 64
	flagDragEnd = 128
)

const (
	Start Align = 0
	Center Align = 1
	End Align = 2
	Stretch Align = 3
	Between Align = 4
)

const (
	RoleNone Role = 0
	RoleHeading Role = 1
	RoleLink Role = 2
	RoleImage Role = 3
	RoleMain Role = 4
	RoleNav Role = 5
)

const (
	Slate Theme = 0
	Midnight Theme = 1
	Warm Theme = 2
	Neutral Theme = 3
)

const (
	Background Color = 1 << 24
	Surface Color = 2 << 24
	Raised Color = 3 << 24
	Inset Color = 4 << 24
	Ink Color = 5 << 24
	Muted Color = 6 << 24
	Strong Color = 7 << 24
	Accent Color = 8 << 24
	Accent2 Color = 9 << 24
	Hover Color = 10 << 24
	Active Color = 11 << 24
	Item Color = 12 << 24
	Line Color = 13 << 24
	Danger Color = 14 << 24
	Selection Color = 15 << 24
	Scrollbar Color = 16 << 24
	ScrollbarHot Color = 17 << 24
	Scrim Color = 18 << 24
	Shadow Color = 19 << 24
)
