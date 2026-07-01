package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

const (
	screenWidth  = 600
	screenHeight = 750

	boardWidth  = 10
	boardHeight = 25

	blockSize    = 28
	fallInterval = time.Second
)

var (
	buttonSize = 50.0

	leftX     = 100.0
	leftY     = 550.0
	rightX    = leftX + buttonSize + 10
	rightY    = leftY
	downX     = leftX + buttonSize/2 + 5
	downY     = leftY + buttonSize - 20
	upX       = leftX + buttonSize/2 + 5
	upY       = leftY - buttonSize + 20
	lineWidth = 4.0
)

//go:embed assets/gameover.wav
var gameoverWAV []byte

var gameoverPlayer *audio.Player

//go:embed assets/tetris-piano.mp3
var tetrisByte []byte

var tetrisPlay *audio.Player

var audioContext *audio.Context

var sampleRate = 44100

//go:embed assets/striker-2.ttf
var strikerTTF []byte

//go:embed assets/PressStart2P-Regular.ttf
var pressStart2PTTF []byte

//go:embed assets/background.png
var backgroundPng []byte

var backgroundImage *ebiten.Image

var (
	emptyColor           = color.RGBA{0, 0, 0, 255}
	scoreBackgroundColor = color.RGBA{11, 11, 0, 255}
	borderColor          = color.RGBA{114, 32, 255, 255}
	borderColorIn        = color.RGBA{0, 0, 0, 255}
	gameOverBgColor      = color.RGBA{255, 0, 0, 255}
	gameOverTextColor    = color.RGBA{255, 255, 255, 255}

	colors = []color.RGBA{
		{0, 255, 255, 255},
		{0, 0, 255, 255},
		{255, 165, 0, 255},
		{255, 255, 0, 255},
		{0, 255, 0, 255},
		{255, 255, 255, 255},
		{255, 0, 0, 255},
	}
	arcadeFont         font.Face
	largeArcadeFont    font.Face
	twolargeArcadeFont font.Face
	minarcadeFont      font.Face

	score    int
	gameOver bool

	pieces = [][][]int{
		{ // I
			{0, 0, 1, 0},
			{0, 0, 1, 0},
			{0, 0, 1, 0},
			{0, 0, 1, 0},
		},
		{ // J
			{0, 0, 2},
			{0, 0, 2},
			{0, 2, 2},
		},
		{ // L
			{0, 3, 0},
			{0, 3, 0},
			{0, 3, 3},
		},
		{ // O
			{0, 4, 4},
			{0, 4, 4},
		},
		{ // S
			{0, 5, 5},
			{5, 5, 0},
		},
		{ // T
			{0, 6, 0},
			{6, 6, 6},
		},
		{ // Z
			{7, 7, 0},
			{0, 7, 7},
		},
	}
)

var pieceBag []int

func initPieceBag() {
	pieceBag = rand.Perm(len(pieces))
}

func drawPieceFromBag() int {
	if len(pieceBag) == 0 {
		initPieceBag()
	}
	shapeIndex := pieceBag[0]
	pieceBag = pieceBag[1:]
	return shapeIndex
}

func init() {
	img, _, err := image.Decode(bytes.NewReader(backgroundPng))
	if err != nil {
		log.Fatal(err)
	}
	backgroundImage = ebiten.NewImageFromImage(img)

	tt, err := opentype.Parse(pressStart2PTTF)
	if err != nil {
		log.Fatal(err)
	}

	tt2, err := opentype.Parse(strikerTTF)
	if err != nil {
		log.Fatal(err)
	}

	const dpi = 72
	minarcadeFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    16,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}

	arcadeFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    28,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}

	largeArcadeFont, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    30,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}

	twolargeArcadeFont, err = opentype.NewFace(tt2, &opentype.FaceOptions{
		Size:    56,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal(err)
	}
}

type Piece struct {
	x, y  int
	shape [][]int
	color int
}

type Game struct {
	board         [boardHeight][boardWidth]int
	piece         *Piece
	lastFall      time.Time
	lastMoveLeft  time.Time
	lastMoveRight time.Time
	moveInterval  time.Duration
	audioEnabled  bool
}

func newPiece() *Piece {
	shapeIndex := drawPieceFromBag()
	shape := pieces[shapeIndex]
	return &Piece{
		x:     boardWidth/2 - len(shape[0])/2,
		y:     0,
		shape: shape,
		color: shapeIndex + 1,
	}
}

func pointInTriangle(px, py, x1, y1, x2, y2, x3, y3 float64) bool {
	areaOrig := area(x1, y1, x2, y2, x3, y3)
	area1 := area(px, py, x2, y2, x3, y3)
	area2 := area(x1, y1, px, py, x3, y3)
	area3 := area(x1, y1, x2, y2, px, py)
	return areaOrig == area1+area2+area3
}

func area(x1, y1, x2, y2, x3, y3 float64) float64 {
	return 0.5 * abs(x1*(y2-y3)+x2*(y3-y1)+x3*(y1-y2))
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (g *Game) Update() error {
	g.audioEnabled = audioContext != nil && audioContext.IsReady()

	if gameOver {
		if ebiten.IsKeyPressed(ebiten.KeyR) {
			resetGame(g)
		}
		return nil
	}

	if g.audioEnabled && tetrisPlay != nil && !tetrisPlay.IsPlaying() {
		tetrisPlay.Rewind()
		tetrisPlay.SetVolume(0.7)
		tetrisPlay.Play()
	}

	if ebiten.IsKeyPressed(ebiten.KeyLeft) && time.Since(g.lastMoveLeft) >= g.moveInterval {
		g.movePieceLeft()
		g.lastMoveLeft = time.Now()
	}

	if ebiten.IsKeyPressed(ebiten.KeyRight) && time.Since(g.lastMoveRight) >= g.moveInterval {
		g.movePieceRight()
		g.lastMoveRight = time.Now()
	}

	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.movePieceDown()
	}

	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.rotatePiece()
	}

	if time.Since(g.lastFall) >= fallInterval {
		g.lastFall = time.Now()
		g.movePieceDown()
	}

	touches := ebiten.TouchIDs()
	for _, t := range touches {
		x, y := ebiten.TouchPosition(t)
		if pointInTriangle(float64(x), float64(y), leftX, leftY-buttonSize/2, leftX, leftY+buttonSize/2, leftX-buttonSize, leftY) {
			g.movePieceLeft()
		} else if pointInTriangle(float64(x), float64(y), rightX, rightY-buttonSize/2, rightX, rightY+buttonSize/2, rightX+buttonSize, rightY) {
			g.movePieceRight()
		} else if pointInTriangle(float64(x), float64(y), downX-buttonSize/2, downY, downX+buttonSize/2, downY, downX, downY+buttonSize) {
			g.movePieceDown()
		} else if pointInTriangle(float64(x), float64(y), upX-buttonSize/2, upY, upX+buttonSize/2, upY, upX, upY-buttonSize) {
			g.rotatePiece()
		}
	}

	return nil
}

func (g *Game) movePieceLeft() {
	g.piece.x--
	if g.checkCollision() {
		g.piece.x++
	}
}

func (g *Game) movePieceRight() {
	g.piece.x++
	if g.checkCollision() {
		g.piece.x--
	}
}

type scoreS struct {
	Score int `json:"score"`
}

func (g *Game) movePieceDown() {
	g.piece.y++
	if g.checkCollision() {
		g.piece.y--
		g.lockPiece()
		g.clearLines()
		g.piece = newPiece()
		if g.checkCollision() {
			gameOver = true
			if tetrisPlay != nil {
				tetrisPlay.Pause()
			}

			var scor = scoreS{
				Score: score,
			}
			bb, _ := json.Marshal(scor)
			fmt.Printf("%s\n", string(bb))

			playSound(gameoverPlayer, 2)
		}
	}
}

func (g *Game) rotatePiece() {
	shape := g.piece.shape
	size := len(shape)
	newShape := make([][]int, size)
	for i := range newShape {
		newShape[i] = make([]int, size)
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			newShape[x][size-1-y] = shape[y][x]
		}
	}
	oldShape := g.piece.shape
	g.piece.shape = newShape
	if g.checkCollision() {
		g.piece.shape = oldShape
	}
}

func (g *Game) checkCollision() bool {
	for py := 0; py < len(g.piece.shape); py++ {
		for px := 0; px < len(g.piece.shape[py]); px++ {
			if g.piece.shape[py][px] != 0 {
				x := g.piece.x + px
				y := g.piece.y + py
				if x < 0 || x >= boardWidth || y >= boardHeight || g.board[y][x] != 0 {
					return true
				}
			}
		}
	}
	return false
}

func (g *Game) lockPiece() {
	for py := 0; py < len(g.piece.shape); py++ {
		for px := 0; px < len(g.piece.shape[py]); px++ {
			if g.piece.shape[py][px] != 0 {
				g.board[g.piece.y+py][g.piece.x+px] = g.piece.color
				score += 10
			}
		}
	}
}

func (g *Game) clearLines() {
	linesCleared := 0
	for y := boardHeight - 1; y >= 0; y-- {
		full := true
		for x := 0; x < boardWidth; x++ {
			if g.board[y][x] == 0 {
				full = false
				break
			}
		}
		if full {
			linesCleared++
			// Move every upper row down by one.
			for ny := y; ny > 0; ny-- {
				for nx := 0; nx < boardWidth; nx++ {
					g.board[ny][nx] = g.board[ny-1][nx]
				}
			}
			// Clear the top row.
			for nx := 0; nx < boardWidth; nx++ {
				g.board[0][nx] = 0
			}
			// Re-check this row after shifting.
			y++
		}
	}
	score += linesCleared * 100
}

func drawTriangle(screen *ebiten.Image, x1, y1, x2, y2, x3, y3 float64, clr color.Color) {
	ebitenutil.DrawLine(screen, x1, y1, x2, y2, clr)
	ebitenutil.DrawLine(screen, x2, y2, x3, y3, clr)
	ebitenutil.DrawLine(screen, x3, y3, x1, y1, clr)
}

func (g *Game) Draw(screen *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(backgroundImage, op)

	for y := 0; y < boardHeight; y++ {
		for x := 0; x < boardWidth; x++ {
			drawBlock(screen, x, y, g.board[y][x])
		}
	}
	if g.piece != nil {
		for py := 0; py < len(g.piece.shape); py++ {
			for px := 0; px < len(g.piece.shape[py]); px++ {
				if g.piece.shape[py][px] != 0 {
					drawBlock(screen, g.piece.x+px, g.piece.y+py, g.piece.color)
				}
			}
		}
	}

	borderThickness := 18
	xOffset := 200 + (480-boardWidth*blockSize)/2
	yOffset := (screenHeight - boardHeight*blockSize) / 2

	ebitenutil.DrawRect(screen, float64(xOffset-borderThickness), float64(yOffset-borderThickness),
		float64(boardWidth*blockSize+2*borderThickness), float64(borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(xOffset-borderThickness), float64(yOffset+boardHeight*blockSize),
		float64(boardWidth*blockSize+2*borderThickness), float64(borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(xOffset-borderThickness), float64(yOffset), float64(borderThickness), float64(boardHeight*blockSize), borderColor)

	ebitenutil.DrawRect(screen, float64(xOffset+boardWidth*blockSize), float64(yOffset), float64(borderThickness), float64(boardHeight*blockSize), borderColor)

	scoreXOffset := 48
	scoreRect := image.Rect(scoreXOffset, (screenHeight-200)/2, 200+scoreXOffset, (screenHeight+200)/2)

	borderThickness = 16

	ebitenutil.DrawRect(screen, float64(scoreRect.Min.X-borderThickness), float64(scoreRect.Min.Y-borderThickness),
		float64(scoreRect.Dx()+2*borderThickness), float64(scoreRect.Dy()+borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(scoreRect.Min.X-borderThickness), float64(scoreRect.Min.Y-borderThickness),
		float64(borderThickness), float64(scoreRect.Dy()+2*borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(scoreRect.Min.X+scoreRect.Dx()), float64(scoreRect.Min.Y-borderThickness),
		float64(borderThickness), float64(scoreRect.Dy()+2*borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(scoreRect.Min.X-borderThickness), float64(scoreRect.Min.Y+scoreRect.Dy()),
		float64(scoreRect.Dx()+2*borderThickness), float64(borderThickness), borderColor)

	ebitenutil.DrawRect(screen, float64(scoreRect.Min.X), float64(scoreRect.Min.Y),
		float64(scoreRect.Dx()), float64(scoreRect.Dy()), scoreBackgroundColor)

	text.Draw(screen, "TETRIS", twolargeArcadeFont, scoreXOffset+40, (screenHeight/2)-200, color.White)

	text.Draw(screen, "SCORE", arcadeFont, scoreXOffset+20, (screenHeight/2)-30, color.White)
	text.Draw(screen, fmt.Sprintf("%d", score), arcadeFont, scoreXOffset+20, (screenHeight/2)+10, color.White)

	if gameOver {
		gameOverText := "GAME OVER"
		pressRText := "Press 'R' to Play"

		x, y := screenWidth/2-100, screenHeight/2-50
		w, h := 300, 100
		borderThickness := 7

		ebitenutil.DrawRect(screen, float64(x-borderThickness), float64(y-borderThickness), float64(w+2*borderThickness), float64(h+2*borderThickness), color.White)
		ebitenutil.DrawRect(screen, float64(x), float64(y), float64(w), float64(h), gameOverBgColor)

		text.Draw(screen, gameOverText, arcadeFont, x+10, y+50, gameOverTextColor)
		text.Draw(screen, pressRText, minarcadeFont, x+10, y+70, gameOverTextColor)
	}

	drawTriangle(screen, leftX, leftY-buttonSize/2, leftX, leftY+buttonSize/2, leftX-buttonSize, leftY, color.RGBA{255, 0, 0, 255})
	drawTriangle(screen, rightX, rightY-buttonSize/2, rightX, rightY+buttonSize/2, rightX+buttonSize, rightY, color.RGBA{0, 255, 0, 255})
	drawTriangle(screen, downX-buttonSize/2, downY, downX+buttonSize/2, downY, downX, downY+buttonSize, color.RGBA{0, 0, 255, 255})
	drawTriangle(screen, upX-buttonSize/2, upY, upX+buttonSize/2, upY, upX, upY-buttonSize, color.RGBA{255, 255, 0, 255})

	if audioContext != nil && !g.audioEnabled {
		text.Draw(screen, "Audio locked by browser: click the page and press any key.", minarcadeFont, 20, screenHeight-20, color.White)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func drawBlock(screen *ebiten.Image, x, y, colorIndex int) {
	color := emptyColor
	if colorIndex != 0 {
		color = colors[colorIndex-1]
	}
	xOffset := (screenWidth-boardWidth*blockSize)/2 + 140
	yOffset := (screenHeight - boardHeight*blockSize) / 2
	cellSize := blockSize - 2

	ebitenutil.DrawRect(screen, float64(x*blockSize+xOffset), float64(y*blockSize+yOffset), float64(blockSize), float64(blockSize), borderColorIn)
	ebitenutil.DrawRect(screen, float64(x*blockSize+xOffset+1), float64(y*blockSize+yOffset+1), float64(cellSize), float64(cellSize), color)
}

func resetGame(g *Game) {
	for y := 0; y < boardHeight; y++ {
		for x := 0; x < boardWidth; x++ {
			g.board[y][x] = 0
		}
	}
	score = 0
	gameOver = false
	g.piece = newPiece()

	g.lastMoveLeft = time.Now()
	g.lastMoveRight = time.Now()
	g.moveInterval = time.Millisecond * 100
}

func playSound(player *audio.Player, volume float64) {
	if player == nil || audioContext == nil || !audioContext.IsReady() {
		return
	}
	player.Rewind()
	player.SetVolume(volume)
	player.Play()
}

func loadSound() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("audio init panic: %v", r)
		}
	}()

	audioContext = audio.NewContext(sampleRate)

	hitSoundDecoded, err := mp3.DecodeWithSampleRate(sampleRate, bytes.NewReader(tetrisByte))
	if err != nil {
		return err
	}

	tetrisPlay, err = audio.NewPlayer(audioContext, hitSoundDecoded)
	if err != nil {
		return err
	}

	gameoverSoundDecoded, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(gameoverWAV))
	if err != nil {
		return err
	}

	gameoverPlayer, err = audio.NewPlayer(audioContext, gameoverSoundDecoded)
	if err != nil {
		return err
	}

	return nil
}

func shouldEnableAudio() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TETRIS_AUDIO"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	// Disable by default on macOS to avoid known CoreAudio initialization failures.
	return runtime.GOOS != "darwin"
}

func newGame() *Game {
	return &Game{
		piece:         newPiece(),
		lastFall:      time.Now(),
		lastMoveLeft:  time.Now(),
		lastMoveRight: time.Now(),
		moveInterval:  time.Millisecond * 100,
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	ebiten.SetWindowSize(screenWidth, screenHeight)
	if shouldEnableAudio() {
		if err := loadSound(); err != nil {
			log.Printf("audio disabled: %v", err)
			audioContext = nil
			tetrisPlay = nil
			gameoverPlayer = nil
		}
	} else {
		log.Printf("audio disabled (set TETRIS_AUDIO=1 to force-enable)")
	}
	ebiten.SetWindowTitle("Tetris in Go")
	game := newGame()
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
