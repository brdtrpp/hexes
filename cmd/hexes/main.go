// Command hexes is the first playable slice: one generated floor at a
// time, the player's rig moving freely in any direction over the hex floor, enemies that
// chase, four hexes firing from triggers, Power and Heat bars, pushes and
// pits. Simple shapes, no art.
//
//	go run ./cmd/hexes             # desktop
//	GOOS=js GOARCH=wasm go build -o web/hexes.wasm ./cmd/hexes   # browser
//
// Desktop: WASD or arrows (or hold the mouse) to move, 1 to 4 to press
// cast buttons (Space is button 1), R or Shift to vent heat, Tab or E
// at a workbench to open the hex editor, Enter to redeploy after dying,
// Esc or M for the menu.
// Touch: hold anywhere to walk that way, or switch MOVE to STICK (top
// right) for a thumb joystick on the left half; tap the numbered cast
// buttons, VENT, and HEX at a workbench.
// The choice is remembered in the browser.
// Strikes on adjacent foes are automatic; walking over loot picks it up.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"

	"hexes/dungeon"
	"hexes/game"
	"hexes/loot"
	"hexes/skill"
)

var (
	colBG      = color.RGBA{0x0b, 0x0d, 0x14, 0xff}
	colWall    = color.RGBA{0x15, 0x18, 0x22, 0xff}
	colWallTop = color.RGBA{0x2a, 0x30, 0x42, 0xff}
	colFloor   = color.RGBA{0x45, 0x4f, 0x66, 0xff}
	colPit     = color.RGBA{0x03, 0x03, 0x06, 0xff}
	colPitRim  = color.RGBA{0x8a, 0x2b, 0x3f, 0xff}
	colHazard  = color.RGBA{0xc2, 0x4e, 0x12, 0xff}
	colStairs  = color.RGBA{0x1f, 0xc8, 0xa8, 0xff}
	colPlayer  = color.RGBA{0x4d, 0xe3, 0xff, 0xff}
	colPower   = color.RGBA{0x3d, 0x9b, 0xff, 0xff}
	colHeat    = color.RGBA{0xff, 0x8a, 0x2b, 0xff}
	colHot     = color.RGBA{0xff, 0x3b, 0x3b, 0xff}
	colHP      = color.RGBA{0x5c, 0xe0, 0x7a, 0xff}
	colText    = color.RGBA{0xe8, 0xec, 0xf4, 0xff}
	colDim     = color.RGBA{0x8c, 0x95, 0xab, 0xff}
	colPanel   = color.RGBA{0x14, 0x18, 0x24, 0xe0}
	colSalvage = color.RGBA{0xff, 0xc8, 0x3d, 0xff}
	colMend    = color.RGBA{0x5a, 0xff, 0x9e, 0xff}
	colShield  = color.RGBA{0x6b, 0xc7, 0xff, 0xff}
	colPurge   = color.RGBA{0xc6, 0xf7, 0xff, 0xff}

	enemyCol = map[game.Kind]color.RGBA{
		game.KindGrunt:   {0xe5, 0x48, 0x4d, 0xff},
		game.KindSkitter: {0xf2, 0xc1, 0x3d, 0xff},
		game.KindBrute:   {0xa8, 0x5c, 0xf0, 0xff},
		game.KindGunner:  {0xff, 0x6f, 0xb0, 0xff},
	}
	// Damage type colors, in default_types.json order.
	typeCol = []color.RGBA{
		{0xff, 0x7a, 0x2f, 0xff}, // plasma
		{0x8f, 0xdc, 0xff, 0xff}, // cryo
		{0xf0, 0xf0, 0xf0, 0xff}, // kinetic
		{0x8c, 0xff, 0x5a, 0xff}, // nanite
		{0xc0, 0x8c, 0xff, 0xff}, // drone
		{0x9a, 0xa0, 0xb0, 0xff}, // cloak
	}
	slotCol = [game.MaxSlots]color.RGBA{
		{0xff, 0x7a, 0x2f, 0xff}, {0xf0, 0xf0, 0xf0, 0xff}, {0x8f, 0xdc, 0xff, 0xff}, {0x8c, 0xff, 0x5a, 0xff},
	}
)

type button struct {
	x, y, r float64
	label   string
}

func (b button) hit(x, y float64) bool { return math.Hypot(x-b.x, y-b.y) <= b.r*1.15 }

// pill is a small rectangular toggle.
type pill struct{ x, y, w, h float64 }

func (p pill) hit(x, y float64) bool {
	m := p.h * 0.3 // a little slop for thumbs
	return x >= p.x-m && x <= p.x+p.w+m && y >= p.y-m && y <= p.y+p.h+m
}

// stickPref is the localStorage key for the movement mode.
const stickPref = "hexes.move"

// mouseID is the Stick ID the mouse uses; touch IDs are never negative.
const mouseID = -1

type Game struct {
	// sheet is the item card on show while sheetOn; peek is the slot
	// whose card a held cast button shows (-1 for none), as peekSheet.
	sheet      loot.Sheet
	sheetOn    bool
	peekBtn    int
	peekT      float64
	peek       int
	peekSheet  loot.Sheet
	sim        *game.Sim
	w, h       int
	k          float64 // device pixels per CSS pixel; UI sizes scale by it
	size       float64 // hex center-to-corner, pixels
	tiles      map[dungeon.Tile]*ebiten.Image
	tileSize   float64
	disc       *ebiten.Image
	face       *text.GoTextFace
	small      *text.GoTextFace
	big        *text.GoTextFace
	camX, camY float64
	casts      [game.Buttons]button // cast buttons 1 to 4 (bench.go)
	vent       button
	hint       string  // a line of help over the play field
	hintT      float64 // seconds the hint has left
	modeBtn    pill
	menuBtn    pill
	menu       menuState
	savedAt    int    // frame of the last save
	hexBtn     button // opens the hex editor
	ed         editor
	cd         condEd   // the condition editor, over the hex editor
	tm         testMenu // the test menu of example builds, over the hex editor
	touched    bool     // a touch has happened: show touch hints, not keys
	// stickMode moves with a thumb joystick on the left half of the
	// screen instead of walking toward the finger.
	stickMode bool
	stick     game.Stick
	frame     int
	fxs       fxState    // procedural effects (fxdraw.go)
	snd       soundState // procedural sound (sound.go)
	dev       devState   // headless screenshot autopilot (devshots_other.go)
	look      lookState  // world look: tiles, actors, light (look.go)
	sndBtn    pill       // mute toggle
	mm        minimap    // explored-floor chart (minimap.go)
	fog       fogState   // what the lamp lights and what's remembered (fog.go)
	teach     teachState // new unlocks to pulse (teach.go)
	ringMode  uint8      // how ringPath lays rings: flat UI, on the floor, or local (fxdraw.go)
	decals    decalState // marks skills leave on the floor (skin.go)
	// padTaught: the repair pad's how-to has shown this session (pad.go).
	padTaught bool
	zm        zoomState // the dungeon view's zoom (zoom.go)
}

func newGame() *Game {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	if err != nil {
		log.Fatal(err)
	}
	g := &Game{
		sim:   game.New(uint64(time.Now().UnixNano())),
		face:  &text.GoTextFace{Source: src, Size: 15},
		small: &text.GoTextFace{Source: src, Size: 12},
		big:   &text.GoTextFace{Source: src, Size: 30},
		peek:  -1,
	}
	g.disc = ebiten.NewImage(64, 64)
	vector.DrawFilledCircle(g.disc, 32, 32, 31, color.White, true)
	p := g.sim.P()
	g.camX, g.camY = p.Pos()
	g.stickMode = loadPref(stickPref) == "stick"
	g.stick.Deadzone = 0.15
	g.zm.init(loadPref(zoomPref))
	g.loadSave()
	g.sim.OfferKits()
	g.initFX()
	g.initSound()
	return g
}

// Layout uses the window's own size, so the view fills any phone or
// desktop window.
func (g *Game) Layout(ow, oh int) (int, int) {
	k := 1.0
	if m := ebiten.Monitor(); m != nil {
		k = math.Max(1, math.Min(3, m.DeviceScaleFactor()))
	}
	w, h := int(float64(ow)*k), int(float64(oh)*k)
	if w != g.w || h != g.h || k != g.k {
		g.w, g.h, g.k = w, h, k
		short := math.Min(float64(ow), float64(oh))
		g.zm.base = k * math.Max(13, math.Min(28, short/20))
		g.size = g.zm.size()
		r := k * math.Max(34, math.Min(56, short/9))
		g.layoutButtons(w, h, r)
		g.modeBtn = pill{float64(w) - 128*k, 10 * k, 116 * k, 26 * k}
		g.menuBtn = pill{g.modeBtn.x - 80*k, 10 * k, 70 * k, 26 * k}
		g.sndBtn = pill{g.menuBtn.x - 52*k, 10 * k, 42 * k, 26 * k}
		g.stick.Radius = k * math.Max(44, math.Min(64, short/7))
		g.face.Size, g.small.Size, g.big.Size = 15*k, 12*k, 30*k
	}
	return w, h
}

// u scales a CSS-pixel size to screen pixels.
func (g *Game) u(x float64) float32 { return float32(x * g.k) }

func (g *Game) Update() error {
	g.frame++
	g.watchTeach()
	g.watchPads()
	g.autosave()
	if g.updateSheet() {
		return nil // paused while a card is open
	}
	if g.updateMenu() {
		return nil // paused while the menu is open
	}
	if g.updateDraft() {
		return nil // the game waits on the pick
	}
	edTap := g.ed.on && (len(inpututil.AppendJustPressedTouchIDs(nil)) > 0 || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft))
	edMsg := g.ed.msg
	if g.updateEditor() {
		if edTap { // a tick per tap, a low one when an edit is refused
			g.click(!(g.ed.bad && g.ed.msg != edMsg))
		}
		return nil // paused while the editor is open
	}
	var in game.Input
	// Keyboard.
	kx, ky := 0.0, 0.0
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		kx--
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		kx++
	}
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		ky--
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		ky++
	}
	in.MoveX, in.MoveY = kx, ky
	if g.hintT > 0 {
		g.hintT -= 1.0 / float64(ebiten.TPS())
	}
	for i, keys := range keyButtons {
		for _, key := range keys {
			if inpututil.IsKeyJustPressed(key) {
				g.sim.Load.Press(i+1, &in)
				in.Restart = true
			}
			if ebiten.IsKeyPressed(key) && g.sim.Load.Holds(i+1) {
				in.Hold = true
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) || inpututil.IsKeyJustPressed(ebiten.KeyShiftLeft) || inpututil.IsKeyJustPressed(ebiten.KeyK) {
		in.Vent = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		in.Restart = true
	}

	// Pointer: touches and the mouse share one path. A press on a button
	// fires it. Otherwise, in follow mode, holding anywhere walks toward
	// the finger; in stick mode a press on the left half of the screen
	// plants a thumb joystick there.
	px, py := g.playerScreen()
	steer := func(x, y float64) {
		if math.Hypot(x-px, y-py) > g.size*0.7 {
			// Walk toward the floor point under the finger, through the
			// tilted camera.
			wx, wy := g.unproject(x, y)
			ox, oy := g.sim.P().Pos()
			in.MoveX, in.MoveY = wx-ox, wy-oy
		}
	}
	onButton := func(x, y float64) bool {
		return g.castButton(x, y) > 0 || g.vent.hit(x, y) || g.modeBtn.hit(x, y) || g.menuBtn.hit(x, y) || (g.hexOn() && g.hexBtn.hit(x, y)) || g.sndBtn.hit(x, y)
	}
	press := func(id int, x, y float64) {
		switch b := g.castButton(x, y); {
		case b > 0:
			in.Restart = true
			g.sim.Load.Press(b, &in)
		case g.vent.hit(x, y):
			in.Vent = true
		case g.modeBtn.hit(x, y):
			g.stickMode = !g.stickMode
			g.stick.End()
			mode := "follow"
			if g.stickMode {
				mode = "stick"
			}
			savePref(stickPref, mode)
			g.click(g.stickMode)
		case g.sndBtn.hit(x, y):
			g.toggleMute()
		case g.menuBtn.hit(x, y):
			g.click(true)
			g.openMenu()
		case g.hexOn() && g.hexBtn.hit(x, y):
			g.click(true)
			g.openEditor()
		default:
			in.Restart = true
			if g.stickMode && !g.stick.On && x < float64(g.w)/2 {
				g.stick.Begin(id, x, y)
				g.zm.stickAt = g.frame
			}
		}
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		g.touched = true
		x, y := ebiten.TouchPosition(id)
		press(int(id), float64(x), float64(y))
	}
	// Zoom: the wheel, + and -, and two-finger pinches (zoom.go).
	var free []freeTouch
	down := map[int]bool{}
	for _, id := range ebiten.AppendTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		down[int(id)] = true
		if !onButton(float64(x), float64(y)) {
			free = append(free, freeTouch{int(id), float64(x), float64(y)})
		}
	}
	g.updateZoom(free, down)
	held := false
	for _, id := range ebiten.AppendTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		fx, fy := float64(x), float64(y)
		if b := g.castButton(fx, fy); b > 0 && g.sim.Load.Holds(b) {
			in.Hold = true
		}
		switch {
		case g.zm.locked[int(id)]:
			// Pinching, or was: it neither steers nor holds the stick.
		case g.stick.On && int(id) == g.stick.ID:
			g.stick.Move(fx, fy)
			held = true
		case !g.stickMode && !onButton(fx, fy):
			steer(fx, fy)
		}
	}
	mx, my := ebiten.CursorPosition()
	fmx, fmy := float64(mx), float64(my)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		press(mouseID, fmx, fmy)
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if b := g.castButton(fmx, fmy); b > 0 && g.sim.Load.Holds(b) {
			in.Hold = true
		}
		switch {
		case g.stick.On && g.stick.ID == mouseID:
			g.stick.Move(fmx, fmy)
			held = true
		case !g.stickMode && !onButton(fmx, fmy):
			steer(fmx, fmy)
		}
	}
	if g.stick.On && !held {
		g.stick.End()
	}
	if i := g.peekCast(); i != g.peek {
		g.peek = i
		if i >= 0 {
			if sh, err := g.sim.SlotSheet(i); err == nil {
				g.peekSheet = sh
			} else {
				g.peek = -1
			}
		}
	}
	if sx, sy := g.stick.Vector(); sx != 0 || sy != 0 {
		// The stick points on screen; the floor runs squashed under it.
		wx, wy := sx, sy/viewCos
		m := math.Hypot(sx, sy) / math.Hypot(wx, wy)
		in.MoveX, in.MoveY = wx*m, wy*m
	}

	g.devInput(&in)
	g.sim.Input = in
	if g.ed.on || g.menu != menuClosed { // a button opened one this frame
		return nil
	}
	dt := 1.0 / float32(ebiten.TPS())
	g.sim.Step(dt)

	// Camera eases toward the player.
	x, y := g.sim.P().Pos()
	g.camX += (x - g.camX) * 0.18
	g.camY += (y - g.camY) * 0.18
	if math.Abs(x-g.camX) > 8 || math.Abs(y-g.camY) > 8 { // new floor: snap
		g.camX, g.camY = x, y
		g.fxs.parts.Clear()
		g.decals.clear()
	}
	g.updateSound(dt)
	g.updateFX(dt)
	g.updateLook(dt)
	return nil
}

func (g *Game) playerScreen() (float64, float64) {
	x, y := g.sim.P().Pos()
	return g.toScreen(x, y)
}

// toScreen maps a point on the floor, in hex units, to pixels (through
// the tilted camera, view.go).
func (g *Game) toScreen(x, y float64) (float64, float64) {
	sx, sy, _ := g.project(x, y)
	return sx, sy
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(colBG)
	if g.needTiles() {
		g.buildTiles()
	}
	g.shakeCamera()
	g.ringMode = ringGround
	g.drawFloor(screen)
	g.drawDrops(screen)
	g.drawLesson(screen)
	g.drawBenches(screen)
	g.drawPads(screen)
	g.drawLenses(screen)
	g.drawSalvage(screen)
	g.drawDecals(screen)
	g.drawGroundExtras(screen)
	g.drawStanceRing(screen)
	g.drawStanding(screen)
	g.drawActorOverlays(screen)
	g.drawShots(screen)
	g.drawLight(screen)
	g.drawForms(screen)
	g.drawParticles(screen)
	g.drawLabels(screen)
	g.drawPadLabels(screen)
	g.ringMode = ringFlat
	g.drawHUD(screen)
	g.drawMinimap(screen)
	if g.ed.on {
		g.drawEditor(screen)
	}
	g.drawDraft(screen)
	if g.menu != menuClosed {
		g.drawMenu(screen)
	}
	switch {
	case g.sheetOn:
		g.drawSheet(screen, g.sheet, false)
	case g.peek >= 0 && !g.ed.on && g.menu == menuClosed:
		g.drawSheet(screen, g.peekSheet, true)
	}
	g.devShot(screen)
}

var white1 = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}()

var image1 = image.Rect(1, 1, 2, 2)

func whiteSub() *ebiten.Image { return white1.SubImage(image1).(*ebiten.Image) }

func paint(vs []ebiten.Vertex, c color.RGBA) {
	for i := range vs {
		vs[i].SrcX, vs[i].SrcY = 1, 1
		vs[i].ColorR = float32(c.R) / 255
		vs[i].ColorG = float32(c.G) / 255
		vs[i].ColorB = float32(c.B) / 255
		vs[i].ColorA = float32(c.A) / 255
	}
}

func (g *Game) nearOpen(c skill.Coord) bool {
	for _, d := range skill.HexDirs {
		if g.sim.Floor.At(c.Add(d)) != dungeon.Wall {
			return true
		}
	}
	return false
}

func (g *Game) drawDisc(dst *ebiten.Image, x, y, r float64, cr, cg, cb, ca float32) {
	op := &ebiten.DrawImageOptions{}
	k := r / 31
	op.GeoM.Translate(-32, -32)
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x, y)
	op.ColorScale.Scale(cr*ca, cg*ca, cb*ca, ca)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(g.disc, op)
}

func fade(c color.RGBA, a float32) color.RGBA {
	return color.RGBA{uint8(float32(c.R) * a), uint8(float32(c.G) * a), uint8(float32(c.B) * a), uint8(255 * a)}
}

func typeColor(t skill.TypeID) color.RGBA {
	if int(t) < len(typeCol) {
		return typeCol[t]
	}
	return colText
}

// drawLabels draws the text effects: damage numbers and floating words.
// Shapes and particles are in fxdraw.go.
func (g *Game) drawLabels(screen *ebiten.Image) {
	s := g.size
	g.sim.FX.Each(func(f *game.FX) {
		k := f.Age / f.Life // 0 to 1
		a := 1 - k
		x, y, _ := g.lifted(f.X, f.Y, chestLift)
		switch f.Kind {
		case game.FXSlam:
			wx, wy := g.toScreen(f.FromX, f.FromY)
			mx, my := (x+wx)/2, (y+wy)/2
			g.label(screen, g.small, "SLAM", mx, my-s*0.6-float64(k)*s, fade(color.RGBA{0xff, 0xe0, 0x60, 0xff}, a), true)
		case game.FXFall:
			g.label(screen, g.face, "PIT!", x, y-s-float64(k)*s, fade(color.RGBA{0xff, 0x6b, 0x8a, 0xff}, a), true)
		case game.FXLoot:
			c := rarityCol[f.Slot%skill.NumRarities]
			g.label(screen, g.small, rarityName[f.Slot%skill.NumRarities], x, y-s*0.9-float64(k)*s*1.2, fade(c, a), true)
		case game.FXSalvage:
			g.label(screen, g.face, fmt.Sprintf("SALVAGED %.0f", f.Value), x, y-s*1.1-float64(k)*s*1.4, fade(colSalvage, a), true)
		case game.FXShape:
			name := g.sim.LastShape.Name()
			g.label(screen, g.face, "LEARNED "+strings.ToUpper(name), x, y-s*1.1-float64(k)*s*1.2, fade(colShape, a), true)
			g.label(screen, g.small, "HEX > SHAPE to use it", x, y-s*0.4-float64(k)*s*1.2, fade(colText, a), true)
		case game.FXTile:
			name := strings.ToUpper(skill.TileKind(f.Slot).String())
			g.label(screen, g.face, "LOGIC TILE: "+name, x, y-s*1.1-float64(k)*s*1.2, fade(colGate, a), true)
			g.label(screen, g.small, "tap an IF gate twice to build with it", x, y-s*0.4-float64(k)*s*1.2, fade(colText, a), true)
		case game.FXFrame:
			f := g.sim.LastFrame
			g.label(screen, g.face, "FRAME: "+strings.ToUpper(f.Name()), x, y-s*1.1-float64(k)*s*1.2, fade(frameCol(f), a), true)
			g.label(screen, g.small, "HEX > FRAMES to fit it", x, y-s*0.4-float64(k)*s*1.2, fade(colText, a), true)
		case game.FXLens:
			g.label(screen, g.face, fmt.Sprintf("LENS: OPTICS +%d", f.Slot), x, y-s*1.1-float64(k)*s*1.2, fade(colLens, a), true)
			g.label(screen, g.small, fmt.Sprintf("light reaches %.1f hexes", f.Value), x, y-s*0.4-float64(k)*s*1.2, fade(colText, a), true)
		case game.FXLesson:
			name := game.Ladder[min(f.Slot, len(game.Ladder)-1)].Name
			g.label(screen, g.face, "LEARNED "+name, x, y-s*1.1-float64(k)*s*1.2, fade(colLesson, a), true)
		case game.FXData:
			g.label(screen, g.small, fmt.Sprintf("+%.0f DATA", f.Value), x+s*0.3, y-s*0.7-float64(k)*s*1.1, fade(colData, a), true)
		case game.FXSignal:
			g.label(screen, g.face, "SIGNAL FRAGMENT", x, y-s*1.1-float64(k)*s*1.2, fade(colSignal, a), true)
			g.label(screen, g.small, "HEX > GRID to light up the fog", x, y-s*0.4-float64(k)*s*1.2, fade(colText, a), true)
		case game.FXFull:
			g.label(screen, g.small, "PACK FULL", x, y-s*0.9-float64(k)*s, fade(colHot, a), true)
		case game.FXNumber:
			if f.Value < 0.5 && !f.Terrain {
				return
			}
			c := typeColor(f.Type)
			if f.Terrain {
				c = color.RGBA{0xff, 0xe0, 0x60, 0xff}
			}
			if f.Hurt {
				c = colHot
			}
			face := g.face
			if f.Small {
				face = g.small
			}
			jx := float64(int(f.At.Q*31+f.At.R*17)%9-4) * s * 0.08
			// Big hits pop up fast and shake for a beat.
			rise := float64(k) * s * 1.4
			if f.Mag >= 1.5 && !f.Small {
				face = g.big
				if k < 0.25 {
					jx += math.Sin(float64(f.Age)*90) * s * 0.06
				}
				rise = easeOut(float64(k)) * s * 1.8
			}
			g.outlined(screen, face, fmt.Sprintf("%.0f", f.Value), x+jx, y-s*0.6-rise, fade(c, a), a)
		case game.FXMend:
			if f.Value >= 0.5 {
				g.outlined(screen, g.face, fmt.Sprintf("+%.0f", f.Value), x, y-s*0.7-float64(k)*s*1.2, fade(colMend, a), a)
			}
		case game.FXShield:
			if f.Value >= 0.5 {
				g.outlined(screen, g.small, fmt.Sprintf("SHIELD +%.0f", f.Value), x, y-s*0.9-float64(k)*s, fade(colShield, a), a)
			}
		case game.FXSoak:
			if f.Value >= 0.5 {
				g.outlined(screen, g.small, fmt.Sprintf("(%.0f)", f.Value), x+s*0.35, y-s*0.5-float64(k)*s, fade(colShield, a), a)
			}
		case game.FXPurge:
			if f.Value >= 0.5 {
				g.outlined(screen, g.small, fmt.Sprintf("HEAT -%.0f", f.Value), x, y-s*0.9-float64(k)*s, fade(colPurge, a), a)
			}
		}
	})
}

func (g *Game) label(dst *ebiten.Image, face *text.GoTextFace, s string, x, y float64, c color.RGBA, center bool) {
	op := &text.DrawOptions{}
	op.SecondaryAlign = text.AlignCenter
	if center {
		op.PrimaryAlign = text.AlignCenter
	}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, face, op)
}

// outlined draws text with a dark edge so it reads over bright effects.
func (g *Game) outlined(dst *ebiten.Image, face *text.GoTextFace, s string, x, y float64, c color.RGBA, a float32) {
	d := float64(g.u(1.5))
	sh := fade(color.RGBA{0x05, 0x06, 0x0a, 0xff}, a*0.9)
	for _, o := range [][2]float64{{-d, 0}, {d, 0}, {0, -d}, {0, d}, {d, d}} {
		g.label(dst, face, s, x+o[0], y+o[1], sh, true)
	}
	g.label(dst, face, s, x, y, c, true)
}

func (g *Game) bar(dst *ebiten.Image, x, y, w, h float32, frac float32, c color.RGBA, label string) {
	p := g.u(2)
	frac = max(0, min(1, frac))
	vector.DrawFilledRect(dst, x-p, y-p, w+2*p, h+2*p, colPanel, false)
	vector.StrokeRect(dst, x-p, y-p, w+2*p, h+2*p, g.u(1), color.RGBA{0x3a, 0x44, 0x5e, 0xff}, false)
	vector.DrawFilledRect(dst, x, y, w, h, color.RGBA{0x16, 0x1a, 0x26, 0xff}, false)
	fw := w * frac
	vector.DrawFilledRect(dst, x, y, fw, h, fade(c, 0.85), false)
	// A lit strip along the top and a bright cap at the fill's end.
	vector.DrawFilledRect(dst, x, y, fw, h*0.3, fade(color.RGBA{0xff, 0xff, 0xff, 0xff}, 0.22), false)
	vector.DrawFilledRect(dst, x, y+h*0.8, fw, h*0.2, fade(color.RGBA{0, 0, 0, 0xff}, 0.25), false)
	if frac > 0.005 && frac < 0.995 {
		vector.DrawFilledRect(dst, x+fw-g.u(2), y, g.u(2), h, color.RGBA{0xff, 0xff, 0xff, 0xd0}, false)
	}
	for i := 1; i < 10; i++ {
		tx := x + w*float32(i)/10
		vector.DrawFilledRect(dst, tx, y+h*0.6, g.u(1), h*0.4, color.RGBA{0, 0, 0, 0x60}, false)
	}
	g.label(dst, g.small, label, float64(x+g.u(6))+float64(g.u(1)), float64(y+h/2)+float64(g.u(1)), color.RGBA{0, 0, 0, 0xc0}, false)
	g.label(dst, g.small, label, float64(x+g.u(6)), float64(y+h/2), colText, false)
}

// bankText is a hex's charge banks for its HUD card, like "A 3/5", and
// how full the first spend is (-1 when no bank has a spend gate).
func (g *Game) bankText(i int) (string, float64) {
	p := g.sim.Load.Progs[i]
	if p == nil || p.Banks == 0 {
		return "", -1
	}
	var b [skill.NumBanks]uint32
	copy(b[:], g.sim.Banks[i][:])
	copy(b[skill.MaxBanks:], g.sim.BoardBanks[:])
	var parts []string
	frac := -1.0
	for k := 0; k < skill.NumBanks; k++ {
		if !p.UsesBank(k) {
			continue
		}
		if n := p.Spend[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d/%d", skill.BankName(uint8(k)), b[k], n))
			if frac < 0 {
				frac = math.Min(1, float64(b[k])/float64(n))
			}
		} else {
			parts = append(parts, fmt.Sprintf("%s %d", skill.BankName(uint8(k)), b[k]))
		}
	}
	return strings.Join(parts, " "), frac
}

func (g *Game) drawHUD(screen *ebiten.Image) {
	sim := g.sim
	p := sim.P()
	rig := &sim.Rig
	W, H := float32(g.w), float32(g.h)
	u := g.u
	narrow := W < u(700)

	// Top left: depth and kills.
	g.label(screen, g.face, fmt.Sprintf("DEPTH %d", sim.Depth), float64(u(14)), float64(u(20)), colText, false)
	info := fmt.Sprintf("data %d  best %d  lv %d  pack %d/%d", sim.DataTotal(), sim.Best, p.U.Level, sim.Stash.Inv.Count(), loot.InventorySize)
	if sim.Unbanked > 0 {
		info = fmt.Sprintf("data %d (%d unbanked)  best %d  lv %d  pack %d/%d", sim.DataTotal(), sim.Unbanked, sim.Best, p.U.Level, sim.Stash.Inv.Count(), loot.InventorySize)
	}
	if n := sim.Unsecured(); n > 0 {
		info += fmt.Sprintf(" (%d new, safe at %d)", n, sim.NextCheckpoint())
	}
	if c := &sim.Cache; c.Active && c.Depth != sim.Depth {
		info += fmt.Sprintf("  cache on %d", c.Depth)
	}
	g.label(screen, g.small, info, float64(u(14)), float64(u(40)), colDim, false)

	// Bars: hull, power, heat. Centered on wide screens, under the depth
	// line on narrow ones.
	bw := min(W-u(28), u(360))
	bx, by := (W-bw)/2, u(12)
	if narrow {
		bx, by, bw = u(14), u(56), W-u(28)
	}
	hull := fmt.Sprintf("HULL %.0f / %.0f", p.U.HP, p.U.MaxHP)
	if p.U.Shield >= 0.5 {
		hull += fmt.Sprintf("  +%.0f shield", p.U.Shield)
	}
	g.bar(screen, bx, by, bw, u(14), p.U.HP/p.U.MaxHP, colHP, hull)
	if p.U.Shield > 0 {
		// The shield rides along the top of the hull bar, fading as its
		// last few seconds run out.
		sw := bw * min(1, p.U.Shield/p.U.MaxHP)
		sa := min(1, p.U.ShieldLeft/1.5)
		vector.DrawFilledRect(screen, bx, by, sw, u(4), fade(colShield, 0.9*sa), false)
	}
	g.bar(screen, bx, by+u(20), bw, u(12), rig.Power/rig.MaxPower, colPower, fmt.Sprintf("POWER %.0f / %.0f", rig.Power, rig.MaxPower))
	heatCol := colHeat
	heatLabel := fmt.Sprintf("HEAT %.0f / %.0f", rig.Heat, rig.MaxHeat)
	if rig.Locked() {
		heatCol = colHot
		if rig.Heat > 0 {
			heatLabel = fmt.Sprintf("OVERHEAT  locked %.1fs", rig.ShutdownLeft)
		} else {
			heatLabel = fmt.Sprintf("VENTING  %.1fs", rig.ShutdownLeft)
		}
		if g.frame/8%2 == 0 {
			heatCol = colHeat
		}
	} else if rig.Heat > rig.MaxHeat*0.75 {
		heatLabel += "  vent!"
	}
	g.bar(screen, bx, by+u(38), bw, u(12), rig.Heat/rig.MaxHeat, heatCol, heatLabel)

	// Hex slots: along the bottom on wide screens, a row under the bars on
	// narrow ones (the bottom belongs to the thumbs).
	cardH := u(46)
	gap := u(6)
	var cx0, cy float32
	var cardW float32
	if narrow {
		cardH = u(38)
		cx0, cy = u(14), by+u(60)
		cardW = (W - u(28) - 3*gap) / game.MaxSlots
	} else {
		room := float32(g.vent.x-g.vent.r) - u(30)
		cardW = min(u(190), (room-u(14))/game.MaxSlots-gap)
		cx0, cy = u(14), H-cardH-u(14)
	}
	for i := 0; i < sim.HexesOpen(); i++ {
		x := cx0 + float32(i)*(cardW+gap)
		g.newPulse(screen, rect{float64(x), float64(cy), float64(cardW), float64(cardH)}, g.hudPulse(i))
		vector.DrawFilledRect(screen, x, cy, cardW, cardH, colPanel, false)
		edge := fade(slotCol[i], 0.45)
		sub := g.slotKey(i)
		if fl := sim.SlotFlash[i]; fl > 0 {
			if sim.SlotLast[i] == skill.Fired {
				edge = slotCol[i]
			} else {
				edge = colHot
				sub = sim.SlotLast[i].String()
			}
		}
		if sim.StanceOn(i) && sim.SlotFlash[i] <= 0 {
			// A stance that's on holds the card lit until you tap it off.
			edge, sub = slotCol[i], "on"
			vector.DrawFilledRect(screen, x, cy, cardW, cardH, fade(slotCol[i], 0.12), false)
		}
		if fl := sim.SlotFlash[i]; fl > 0 {
			fc := slotCol[i]
			if sim.SlotLast[i] != skill.Fired {
				fc = colHot
			}
			vector.DrawFilledRect(screen, x, cy, cardW, cardH, fade(fc, 0.3*min(1, float32(fl)*3)), false)
		}
		vector.DrawFilledRect(screen, x, cy, cardW, cardH*0.45, fade(slotCol[i], 0.06), false)
		vector.DrawFilledRect(screen, x, cy, u(4), cardH, slotCol[i], false)
		vector.StrokeRect(screen, x, cy, cardW, cardH, u(2), edge, false)
		bank, frac := g.bankText(i)
		if frac >= 0 {
			// A charge strip along the bottom fills toward the spend.
			vector.DrawFilledRect(screen, x+u(2), cy+cardH-u(5), (cardW-u(4))*float32(frac), u(3), slotCol[i], false)
		}
		if bank != "" && sim.SlotFlash[i] <= 0 {
			if narrow {
				sub = bank
			} else {
				sub += "  ·  " + bank
			}
		}
		if narrow {
			g.label(screen, g.small, shortName(sim.Load.Names[i]), float64(x+cardW/2), float64(cy+cardH*0.32), slotCol[i], true)
			g.label(screen, g.small, sub, float64(x+cardW/2), float64(cy+cardH*0.72), colDim, true)
			continue
		}
		if pr := sim.Load.Progs[i]; pr != nil && sim.SlotFlash[i] <= 0 {
			sub += "  ·  " + costTag(pr.Power, pr.Heat)
		}
		g.label(screen, g.small, sim.Load.Names[i]+"  ·  "+sub, float64(x+u(7)), float64(cy+cardH*0.3), slotCol[i], false)
		g.label(screen, g.small, sim.Load.Blurb[i], float64(x+u(7)), float64(cy+cardH*0.7), colDim, false)
	}

	// Touch buttons (they work with the mouse too).
	g.drawCastButtons(screen)
	{
		b := g.vent
		c := color.RGBA{0xd8, 0xf4, 0xff, 0xff}
		if rig.Heat > rig.MaxHeat*0.75 && g.frame/10%2 == 0 {
			c = colHot
		}
		g.roundButton(screen, b, c)
		g.outlined(screen, g.face, b.label, b.x, b.y, colText, 1)
	}
	g.drawHexButton(screen)
	g.drawHint(screen)

	// Sound, menu and movement mode toggles, and the stick itself.
	g.drawSoundButton(screen)
	{
		m := g.menuBtn
		vector.DrawFilledRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), colPanel, false)
		vector.StrokeRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), u(1.5), colDim, false)
		g.label(screen, g.small, "MENU", m.x+m.w/2, m.y+m.h/2, colDim, true)
	}
	{
		m := g.modeBtn
		label, c := "MOVE: FOLLOW", colDim
		if g.stickMode {
			label, c = "MOVE: STICK", colPlayer
		}
		vector.DrawFilledRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), colPanel, false)
		vector.StrokeRect(screen, float32(m.x), float32(m.y), float32(m.w), float32(m.h), u(1.5), c, false)
		g.label(screen, g.small, label, m.x+m.w/2, m.y+m.h/2, c, true)
	}
	if g.stickMode {
		st := &g.stick
		bx, by := st.BaseX, st.BaseY
		a := float32(0.9)
		if !st.On {
			// Resting ghost where a left thumb usually lands.
			bx, by = st.Radius+float64(u(28)), float64(H)-st.Radius-float64(u(40))
			if !narrow { // the hex cards sit along the bottom
				by = float64(cy) - st.Radius - float64(u(16))
			}
			a = 0.35
		}
		vector.DrawFilledCircle(screen, float32(bx), float32(by), float32(st.Radius), fade(colPanel, a*0.6), true)
		vector.StrokeCircle(screen, float32(bx), float32(by), float32(st.Radius), u(2), fade(colPlayer, a*0.6), true)
		kx, ky := bx, by
		if st.On {
			kx, ky = st.Knob()
		}
		vector.DrawFilledCircle(screen, float32(kx), float32(ky), float32(st.Radius*0.42), fade(colPlayer, a*0.55), true)
	}

	// The lesson card sits under the bars (and the hex row on a phone).
	cardTop := float64(by + u(60))
	if narrow {
		cardTop = float64(cy + cardH + u(8))
	}
	g.drawCard(screen, cardTop)

	// First-seconds hint, once the first card has gone.
	if sim.Depth == 1 && sim.Time < 20 && sim.CardT <= 0 && sim.State == game.Playing {
		if narrow || g.touched {
			walk := "hold anywhere to walk"
			if g.stickMode {
				walk = "drag on the left half to steer"
			}
			g.label(screen, g.small, walk+" · strikes are automatic", float64(W/2), float64(cy+cardH+u(18)), colDim, true)
			g.label(screen, g.small, "1 casts · VENT when hot · walk over glowing pickups", float64(W/2), float64(cy+cardH+u(36)), colDim, true)
		} else {
			g.label(screen, g.small, "WASD / mouse: move    1 or SPACE: cast    R: vent    walk over glowing pickups to learn    ▼ stairs",
				float64(W/2), float64(cy-u(18)), colDim, true)
		}
	}

	if t := sim.CheckpointT; t > 0 && sim.State == game.Playing {
		a := min(1, t)
		y := float64(H) * 0.3
		g.label(screen, g.big, "CHECKPOINT", float64(W/2), y, fade(colSalvage, a), true)
		g.label(screen, g.face, "pack secured", float64(W/2), y+float64(u(28)), fade(colText, a), true)
	}

	switch sim.State {
	case game.Descending:
		a := min(1, sim.StateT/game.DescendTime)
		vector.DrawFilledRect(screen, 0, 0, W, H, fade(colBG, a), false)
		g.label(screen, g.big, fmt.Sprintf("DEPTH %d", sim.Depth+1), float64(W/2), float64(H/2), fade(colStairs, a), true)
	case game.Dead:
		vector.DrawFilledRect(screen, 0, 0, W, H, color.RGBA{0, 0, 0, 0xb0}, false)
		g.label(screen, g.big, "SIGNAL LOST", float64(W/2), float64(H/2)-float64(u(30)), colHot, true)
		d := sim.LastDeath
		y := float64(H/2) + float64(u(8))
		g.label(screen, g.face, fmt.Sprintf("falling back to depth %d", d.To), float64(W/2), y, colText, true)
		y += float64(u(24))
		if d.Mercy {
			g.label(screen, g.small, "a shorter fall: you went down recently", float64(W/2), y, colDim, true)
			y += float64(u(18))
		}
		if d.Lost > 0 {
			g.label(screen, g.small, fmt.Sprintf("%d new parts left in a salvage cache on depth %d", d.Lost, d.From), float64(W/2), y, colSalvage, true)
			y += float64(u(18))
		}
		if d.Data > 0 {
			g.label(screen, g.small, fmt.Sprintf("%d unbanked Data in the salvage crate on depth %d", d.Data, d.From), float64(W/2), y, colSalvage, true)
			y += float64(u(18))
		}
		if d.LetGo > 0 {
			g.label(screen, g.small, fmt.Sprintf("the grid let go of %d outer cells: buy them back at half price", d.LetGo), float64(W/2), y, colData, true)
			y += float64(u(18))
		}
		if d.Gone > 0 || d.DataGone > 0 {
			g.label(screen, g.small, fmt.Sprintf("your old cache is gone for good (%d parts, %d Data)", d.Gone, d.DataGone), float64(W/2), y, colHot, true)
			y += float64(u(18))
		}
		g.label(screen, g.small, "rig, hexes, locker and the rest of your pack are safe", float64(W/2), y, colDim, true)
		g.label(screen, g.small, "tap, Enter or Space to redeploy", float64(W/2), y+float64(u(24)), colDim, true)
	}
}

func shortName(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return s[i+1:]
		}
	}
	return s
}

func main() {
	ebiten.SetWindowSize(devWindow())
	ebiten.SetWindowTitle("Hexes")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(newGame()); err != nil {
		log.Fatal(err)
	}
}
