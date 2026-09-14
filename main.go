package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed seed.json
var embeddedFS embed.FS

const (
	WS_OVERLAPPEDWINDOW  = 0x00CF0000
	WS_VISIBLE           = 0x10000000
	CW_USEDEFAULT        = 0x80000000
	SW_SHOW              = 5
	COLOR_WINDOW         = 5
	IDC_ARROW            = 32512
	WM_DESTROY           = 0x0002
	WM_CLOSE             = 0x0010
	WM_PAINT             = 0x000F
	WM_ERASEBKGND        = 0x0014
	WM_LBUTTONDOWN       = 0x0201
	WM_MOUSEWHEEL        = 0x020A
	WM_KEYDOWN           = 0x0100
	WM_TIMER             = 0x0113
	WM_SIZE              = 0x0005
	WM_APP_SYNC_DONE     = 0x8001
	WM_APP_OAUTH_DONE    = 0x8002
	WM_APP_CLEAR_PRESSED = 0x8003
	WM_APP_ROUTE_DONE    = 0x8004
	WM_APP_UPDATE_DONE   = 0x8005
	WM_APP_TILE_DONE     = 0x8006
	WM_APP_MARKET_DONE   = 0x8007
	SRCCOPY              = 0x00CC0020
	DT_LEFT              = 0x0000
	DT_CENTER            = 0x0001
	DT_RIGHT             = 0x0002
	DT_VCENTER           = 0x0004
	DT_SINGLELINE        = 0x0020
	DT_END_ELLIPSIS      = 0x8000
	TRANSPARENT          = 1
	PS_SOLID             = 0
	FW_NORMAL            = 400
	FW_SEMIBOLD          = 600
	FW_BOLD              = 700
	MB_OK                = 0x00000000
	MB_ICONINFORMATION   = 0x00000040
	MB_ICONWARNING       = 0x00000030
	FLASHW_ALL           = 0x00000003
	FLASHW_TIMERNOFG     = 0x0000000C
	VK_UP                = 0x26
	VK_DOWN              = 0x28
	VK_PRIOR             = 0x21
	VK_NEXT              = 0x22
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdiplus  = syscall.NewLazyDLL("gdiplus.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pShowWindow       = user32.NewProc("ShowWindow")
	pUpdateWindow     = user32.NewProc("UpdateWindow")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pFillRect         = user32.NewProc("FillRect")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pDrawTextW        = user32.NewProc("DrawTextW")
	pSetTimer         = user32.NewProc("SetTimer")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pMessageBoxW      = user32.NewProc("MessageBoxW")
	pPostMessageW     = user32.NewProc("PostMessageW")
	pLoadCursorW      = user32.NewProc("LoadCursorW")
	pMessageBeep      = user32.NewProc("MessageBeep")
	pFlashWindowEx    = user32.NewProc("FlashWindowEx")
	pEllipse          = gdi32.NewProc("Ellipse")

	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pMoveToEx               = gdi32.NewProc("MoveToEx")
	pLineTo                 = gdi32.NewProc("LineTo")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")

	pShellExecuteW    = shell32.NewProc("ShellExecuteW")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	pGdiplusStartup        = gdiplus.NewProc("GdiplusStartup")
	pGdiplusShutdown       = gdiplus.NewProc("GdiplusShutdown")
	pGdipCreateFromHDC     = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics    = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipLoadImageFromFile = gdiplus.NewProc("GdipLoadImageFromFile")
	pGdipDisposeImage      = gdiplus.NewProc("GdipDisposeImage")
	pGdipDrawImageRectI    = gdiplus.NewProc("GdipDrawImageRectI")
	pGdipSetClipRectI      = gdiplus.NewProc("GdipSetClipRectI")
)

type WNDCLASSEX struct {
	CbSize, Style                                                                  uint32
	LpfnWndProc                                                                    uintptr
	CbClsExtra, CbWndExtra                                                         int32
	HInstance, HIcon, HCursor, HbrBackground, LpszMenuName, LpszClassName, HIconSm uintptr
}

type POINT struct{ X, Y int32 }

type FLASHWINFO struct {
	CbSize    uint32
	Hwnd      uintptr
	DwFlags   uint32
	UCount    uint32
	DwTimeout uint32
}

type GdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type UpdateManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes"`
}
type RECT struct{ Left, Top, Right, Bottom int32 }
type MSG struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             POINT
}
type PAINTSTRUCT struct {
	Hdc                uintptr
	Erase              int32
	RcPaint            RECT
	Restore, IncUpdate int32
	RgbReserved        [32]byte
}

type Meta struct {
	SnapshotDate      string `json:"snapshotDate"`
	MarketLastChecked string `json:"marketLastChecked"`
	Profession        string `json:"profession"`
	Canton            string `json:"canton"`
	Version           string `json:"version,omitempty"`
	GmailLastSyncUnix int64  `json:"gmailLastSyncUnix,omitempty"`
}
type Home struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
type Application struct {
	ID          string `json:"id"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	Address     string `json:"address,omitempty"`
	Email       string `json:"email"`
	AppliedDate string `json:"appliedDate"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	ThreadID    string `json:"threadId"`
	Note        string `json:"note"`
}
type Market struct {
	ID           string `json:"id"`
	Company      string `json:"company"`
	Location     string `json:"location"`
	Address      string `json:"address,omitempty"`
	Direction    string `json:"direction"`
	Year         int    `json:"year"`
	Availability string `json:"availability"`
	SourceURL    string `json:"sourceUrl"`
	Note         string `json:"note"`
}
type Data struct {
	Meta         Meta          `json:"meta"`
	Home         Home          `json:"home"`
	Applications []Application `json:"applications"`
	Market       []Market      `json:"market"`
}

type Coord struct{ Lat, Lon float64 }
type RouteInfo struct {
	Minutes   int
	KM        float64
	Estimated bool
	Err       string
}

type OAuthConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}
type oauthRoot struct {
	Installed    OAuthConfig `json:"installed"`
	Web          OAuthConfig `json:"web"`
	ClientID     string      `json:"client_id"`
	ClientSecret string      `json:"client_secret"`
}
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	ObtainedAt   int64  `json:"obtained_at"`
}

type GMessage struct {
	ID           string `json:"id"`
	ThreadID     string `json:"threadId"`
	Snippet      string `json:"snippet"`
	InternalDate string `json:"internalDate"`
	Payload      struct {
		Headers []struct{ Name, Value string } `json:"headers"`
	} `json:"payload"`
}

type RectHit struct {
	Name string
	R    RECT
	URL  string
}

type MapItem struct {
	Key, Company, Location, Address, Status string
	Applied                                 bool
}

var (
	hwndMain                                                                            uintptr
	appDataDir, dataPath, oauthPath, tokenPath, setupPath, routeCachePath, tileCacheDir string
	current                                                                             Data
	dataMu                                                                              sync.RWMutex
	activeTab                                                                           = 0
	offsets                                                                             = []int{0, 0, 0, 0, 0, 0}
	hits                                                                                []RectHit
	syncBusy                                                                            bool
	oauthBusy                                                                           bool
	stateMu                                                                             sync.Mutex
	pressedName                                                                         string
	lastSyncText                                                                        = "Henüz senkronize edilmedi"
	statusLine                                                                          = "Hazır"
	lastAlert                                                                           string
	alertUntil                                                                          time.Time
	client                                                                              = &http.Client{Timeout: 25 * time.Second}
	routeClient                                                                         = &http.Client{Timeout: 10 * time.Second}
	routeMu                                                                             sync.Mutex
	routeCache                                                                          = map[string]RouteInfo{}
	routePending                                                                        = map[string]bool{}
	routeSem                                                                            = make(chan struct{}, 2)
	homeOnce                                                                            sync.Once
	homeCoord                                                                           Coord
	homeCoordErr                                                                        error
	homeCoordMu                                                                         sync.RWMutex
	brushCache                                                                          = map[uintptr]uintptr{}
	penCache                                                                            = map[uintptr]uintptr{}
	fontTitle, fontSub, fontTab, fontBody, fontBold, fontSmall, fontBig                 uintptr
	backDC, backBmp, backOld                                                            uintptr
	backW, backH                                                                        int32
	gdipToken                                                                           uintptr
	tileMu                                                                              sync.Mutex
	tileImages                                                                          = map[string]uintptr{}
	tilePending                                                                         = map[string]bool{}
	tileSem                                                                             = make(chan struct{}, 2)
	updateBusy                                                                          bool
	updateStatus                                                                        = "Güncelleme kontrolü bekleniyor"
	marketBusy                                                                          bool
	marketStatus                                                                        = "Lehrstellen canlı kontrolü bekleniyor"
)

const appVersion = "1.3.0"

const updateManifestURL = "https://raw.githubusercontent.com/omerbuyukkaymaz-maker/lehr-radar/main/update.json"
const marketDataURL = "https://raw.githubusercontent.com/omerbuyukkaymaz-maker/lehr-radar/main/market.json"

func rgb(r, g, b byte) uintptr { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
func wstr(s string) *uint16    { p, _ := syscall.UTF16PtrFromString(s); return p }
func loWord(v uintptr) int32   { return int32(int16(v & 0xffff)) }
func hiWord(v uintptr) int32   { return int32(int16((v >> 16) & 0xffff)) }

func appDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "LehrRadar")
}

func initFiles() error {
	appDataDir = appDir()
	dataPath = filepath.Join(appDataDir, "data.json")
	oauthPath = filepath.Join(appDataDir, "gmail-oauth.json")
	tokenPath = filepath.Join(appDataDir, "gmail-token.json")
	setupPath = filepath.Join(appDataDir, "GMAIL_KURULUM.txt")
	routeCachePath = filepath.Join(appDataDir, "route-cache.json")
	tileCacheDir = filepath.Join(appDataDir, "mapcache")
	if err := os.MkdirAll(tileCacheDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		b, _ := embeddedFS.ReadFile("seed.json")
		_ = os.WriteFile(dataPath, b, 0644)
	}
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		_ = os.WriteFile(setupPath, []byte(gmailSetupText), 0644)
	}
	if err := loadData(); err != nil {
		return err
	}
	if migrateData() {
		if err := saveData(); err != nil {
			return err
		}
	}
	loadRouteCache()
	return nil
}

func loadData() error {
	b, err := os.ReadFile(dataPath)
	if err != nil {
		return err
	}
	var d Data
	if err = json.Unmarshal(b, &d); err != nil {
		return err
	}
	dataMu.Lock()
	current = d
	dataMu.Unlock()
	return nil
}

func migrateData() bool {
	changed := false
	dataMu.Lock()
	upgrading := current.Meta.Version != appVersion
	defer dataMu.Unlock()
	if current.Home.Address == "" {
		current.Home = Home{Name: "Ev", Address: ""}
		changed = true
	}
	if current.Meta.Version != appVersion {
		current.Meta.Version = appVersion
		changed = true
	}
	if current.Meta.GmailLastSyncUnix == 0 {
		current.Meta.GmailLastSyncUnix = time.Now().Add(-7 * 24 * time.Hour).Unix()
		changed = true
	}
	if current.Meta.Profession == "" {
		current.Meta.Profession = "Automobil-Fachmann EFZ"
		changed = true
	}
	if current.Meta.Canton == "" {
		current.Meta.Canton = "AG"
		changed = true
	}
	if upgrading {
		if b, err := embeddedFS.ReadFile("seed.json"); err == nil {
			var seed Data
			if json.Unmarshal(b, &seed) == nil && len(seed.Market) > 0 {
				idx := map[string]int{}
				for i := range current.Market {
					idx[current.Market[i].ID] = i
				}
				for _, m := range seed.Market {
					if i, ok := idx[m.ID]; ok {
						current.Market[i] = m
					} else {
						current.Market = append(current.Market, m)
					}
				}
				current.Meta.MarketLastChecked = seed.Meta.MarketLastChecked
				changed = true
			}
		}
	}
	addresses := map[string]string{
		"amag-schinznach":    "Aarauerstrasse 22, 5116 Schinznach-Bad, Schweiz",
		"emil-frey-safenwil": "Emil Frey Strasse, 5745 Safenwil, Schweiz",
		"kennys-wettingen":   "Landstrasse 189, 5430 Wettingen, Schweiz",
		"autohaus-kueng":     "Im Halt 2, 5412 Gebenstorf, Schweiz",
		"garage-kueng":       "Landstrasse 53, 5412 Gebenstorf, Schweiz",
		"autocenter-kueng":   "Landstrasse 148, 5430 Wettingen, Schweiz",
		"feldgarage-seengen": "Egliswilerstrasse 35, 5707 Seengen, Schweiz",
		"auto-meier":         "Hauptstrasse 253, 5314 Kleindöttingen, Schweiz",
		"garage-ruetter":     "Mettenfeldring 8, 5642 Mühlau, Schweiz",
		"rh-auto-service":    "Aarauerstrasse 35, 5600 Lenzburg, Schweiz",
		"haller-zofingen":    "Untere Brühlstrasse 33, 4800 Zofingen, Schweiz",
		"walter-hasler":      "Schützenweg 4, 5070 Frick, Schweiz",
		"garage-pinar":       "Wartburgstrasse 2, 4663 Aarburg, Schweiz",
		"auto-schneider":     "Kuhgässlistrasse 1, 5303 Würenlingen, Schweiz",
		"bestdrive":          "Neulandweg 6, 5502 Hunzenschwil, Schweiz",
		"tinner":             "Bruggerstrasse 152, 5400 Baden, Schweiz",
		"sepp-suter":         "Oberdorf 15, 5637 Beinwil (Freiamt), Schweiz",
		"hedin-wohlen":       "Schützenmattweg 20, 5610 Wohlen, Schweiz",
		"wyser-seon":         "Breitenweg 19, 5703 Seon, Schweiz",
		"carplanet-aarburg":  "Oltnerstrasse 101, 4663 Aarburg, Schweiz",
	}
	for i := range current.Applications {
		if current.Applications[i].Address == "" {
			if a := addresses[current.Applications[i].ID]; a != "" {
				current.Applications[i].Address = a
				changed = true
			}
		}
	}
	marketAddr := map[string]string{
		"gross-garage-wohlen": "Wohlen AG, Schweiz", "gross-garage-wettingen": "Wettingen, Schweiz",
		"auto-technik-aarau": "Aarau, Schweiz", "autohaus-tivoli": "Spreitenbach, Schweiz",
		"garage-rinau": "Kaiseraugst, Schweiz", "garage-gut": "Meisterschwanden, Schweiz",
		"gebr-knecht": "Windisch, Schweiz", "iveco-hendschiken": "Hendschiken, Schweiz",
		"scania-murgenthal": "Murgenthal, Schweiz", "merbag-wettingen-nf": "Wettingen, Schweiz",
		"merbag-aarau-rohr": "Aarau Rohr, Schweiz", "dreier-oberentfelden": "Oberentfelden, Schweiz",
	}
	for i := range current.Market {
		if current.Market[i].Address == "" {
			if a := marketAddr[current.Market[i].ID]; a != "" {
				current.Market[i].Address = a
				changed = true
			}
		}
	}
	found := false
	for _, m := range current.Market {
		if m.ID == "lancarauto-arni" {
			found = true
			break
		}
	}
	if !found {
		current.Market = append(current.Market, Market{ID: "lancarauto-arni", Company: "Lancarauto GmbH", Location: "Arni", Address: "Arni AG, Schweiz", Direction: "Automobil-Fachmann EFZ Personenwagen", Year: 2027, Availability: "unknown", SourceURL: "https://www.yousty.ch/de-CH/lehrstellen/AG/34302-automobil-fachmann-frau-efz-personenwagen", Note: "Lehrbetrieb; 2027 durumu henüz net değil."})
		changed = true
	}
	return changed
}

func saveData() error {
	dataMu.RLock()
	b, err := json.MarshalIndent(current, "", "  ")
	dataMu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(dataPath, b, 0644)
}

func loadRouteCache() {
	b, err := os.ReadFile(routeCachePath)
	if err != nil {
		return
	}
	var m map[string]RouteInfo
	if json.Unmarshal(b, &m) == nil {
		routeMu.Lock()
		for k, v := range m {
			routeCache[k] = v
		}
		routeMu.Unlock()
	}
}

func saveRouteCache() {
	routeMu.Lock()
	copyMap := make(map[string]RouteInfo, len(routeCache))
	for k, v := range routeCache {
		copyMap[k] = v
	}
	routeMu.Unlock()
	b, _ := json.MarshalIndent(copyMap, "", "  ")
	_ = os.WriteFile(routeCachePath, b, 0644)
}

func initGDIPlus() {
	in := GdiplusStartupInput{GdiplusVersion: 1}
	pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
}

func shutdownGDIPlus() {
	tileMu.Lock()
	for _, img := range tileImages {
		if img != 0 {
			pGdipDisposeImage.Call(img)
		}
	}
	tileImages = map[string]uintptr{}
	tileMu.Unlock()
	if gdipToken != 0 {
		pGdiplusShutdown.Call(gdipToken)
		gdipToken = 0
	}
}

func cleanupBackbuffer() {
	if backDC != 0 {
		if backOld != 0 {
			pSelectObject.Call(backDC, backOld)
		}
		if backBmp != 0 {
			pDeleteObject.Call(backBmp)
		}
		pDeleteDC.Call(backDC)
	}
	backDC, backBmp, backOld = 0, 0, 0
	backW, backH = 0, 0
}

func ensureBackbuffer(screenHdc uintptr, w, h int32) uintptr {
	if backDC != 0 && backW == w && backH == h {
		return backDC
	}
	cleanupBackbuffer()
	backDC, _, _ = pCreateCompatibleDC.Call(screenHdc)
	backBmp, _, _ = pCreateCompatibleBitmap.Call(screenHdc, uintptr(w), uintptr(h))
	backOld, _, _ = pSelectObject.Call(backDC, backBmp)
	backW, backH = w, h
	return backDC
}

func installedExePath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "LehrRadar", "Lehr_Radar.exe")
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func createDesktopShortcut(target string) {
	name := "Lehr Radar.lnk"
	cmd := fmt.Sprintf(`$ws=New-Object -ComObject WScript.Shell; $d=[Environment]::GetFolderPath('Desktop'); $s=$ws.CreateShortcut((Join-Path $d '%s')); $s.TargetPath='%s'; $s.WorkingDirectory='%s'; $s.Description='Lehr Radar - Lehrstellen Tracker'; $s.Save()`, psQuote(name), psQuote(target), psQuote(filepath.Dir(target)))
	_ = exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-Command", cmd).Run()
}

// ensureInstalled makes the app a one-time install: after the first launch, a stable
// copy lives under LocalAppData and future versions can replace that copy in place.
func ensureInstalled() bool {
	exe, err := os.Executable()
	if err != nil {
		return true
	}
	exe, _ = filepath.Abs(exe)
	dst := installedExePath()
	dstAbs, _ := filepath.Abs(dst)
	if strings.EqualFold(exe, dstAbs) {
		return true
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return true
	}
	in, err := os.Open(exe)
	if err != nil {
		return true
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return true
	}
	_, cpErr := io.Copy(out, in)
	closeErr := out.Close()
	if cpErr != nil || closeErr != nil {
		return true
	}
	createDesktopShortcut(dst)
	_ = exec.Command(dst).Start()
	return false
}

func versionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	ss := strings.Split(v, ".")
	out := make([]int, 3)
	for i := 0; i < len(out) && i < len(ss); i++ {
		n := strings.SplitN(ss[i], "-", 2)[0]
		out[i], _ = strconv.Atoi(n)
	}
	return out
}

func versionGreater(a, b string) bool {
	aa, bb := versionParts(a), versionParts(b)
	for i := 0; i < 3; i++ {
		if aa[i] != bb[i] {
			return aa[i] > bb[i]
		}
	}
	return false
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func downloadTo(u, path string) error {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "LehrRadar/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	cerr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return cerr
	}
	return os.Rename(tmp, path)
}

func launchUpdater(newExe, target string) error {
	pid := os.Getpid()
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("lehr-radar-update-%d.cmd", time.Now().UnixNano()))
	script := fmt.Sprintf(`@echo off
set "PID=%d"
:wait
tasklist /FI "PID eq %%PID%%" 2>NUL | find "%%PID%%" >NUL
if not errorlevel 1 (
  timeout /t 1 /nobreak >NUL
  goto wait
)
copy /Y "%s" "%s" >NUL
del /Q "%s" >NUL 2>NUL
start "" "%s"
del /Q "%%~f0" >NUL 2>NUL
`, pid, newExe, target, newExe, target)
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/C", scriptPath)
	return cmd.Start()
}

func checkForUpdate(userTriggered bool) {
	stateMu.Lock()
	if updateBusy {
		stateMu.Unlock()
		return
	}
	updateBusy = true
	updateStatus = "Güncelleme kontrol ediliyor..."
	stateMu.Unlock()
	invalidate()
	go func() {
		defer func() {
			stateMu.Lock()
			updateBusy = false
			stateMu.Unlock()
			pPostMessageW.Call(hwndMain, WM_APP_UPDATE_DONE, 0, 0)
		}()
		req, _ := http.NewRequest("GET", updateManifestURL+"?t="+strconv.FormatInt(time.Now().Unix(), 10), nil)
		req.Header.Set("User-Agent", "LehrRadar/"+appVersion)
		resp, err := (&http.Client{Timeout: 7 * time.Second}).Do(req)
		if err != nil {
			stateMu.Lock()
			updateStatus = "Güncelleme sunucusuna ulaşılamadı"
			stateMu.Unlock()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			stateMu.Lock()
			updateStatus = "Otomatik güncelleme kanalı hazırlanıyor"
			stateMu.Unlock()
			return
		}
		if resp.StatusCode >= 300 {
			stateMu.Lock()
			updateStatus = fmt.Sprintf("Güncelleme kontrolü: HTTP %d", resp.StatusCode)
			stateMu.Unlock()
			return
		}
		var m UpdateManifest
		if json.NewDecoder(resp.Body).Decode(&m) != nil || m.Version == "" || m.URL == "" {
			stateMu.Lock()
			updateStatus = "Güncelleme manifesti geçersiz"
			stateMu.Unlock()
			return
		}
		if !versionGreater(m.Version, appVersion) {
			stateMu.Lock()
			updateStatus = "Güncel · v" + appVersion
			stateMu.Unlock()
			return
		}
		newPath := filepath.Join(os.TempDir(), "Lehr_Radar_"+strings.ReplaceAll(m.Version, ".", "_")+".exe")
		stateMu.Lock()
		updateStatus = "v" + m.Version + " indiriliyor..."
		stateMu.Unlock()
		invalidate()
		if err := downloadTo(m.URL, newPath); err != nil {
			stateMu.Lock()
			updateStatus = "Güncelleme indirilemedi: " + err.Error()
			stateMu.Unlock()
			return
		}
		if strings.TrimSpace(m.SHA256) != "" {
			h, err := fileSHA256(newPath)
			if err != nil || !strings.EqualFold(h, strings.TrimSpace(m.SHA256)) {
				_ = os.Remove(newPath)
				stateMu.Lock()
				updateStatus = "Güncelleme doğrulaması başarısız"
				stateMu.Unlock()
				return
			}
		}
		target := installedExePath()
		stateMu.Lock()
		updateStatus = "v" + m.Version + " hazır · yeniden başlatılıyor"
		lastAlert = "Yeni sürüm v" + m.Version + " kuruluyor"
		alertUntil = time.Now().Add(8 * time.Second)
		stateMu.Unlock()
		notifyUser()
		time.Sleep(900 * time.Millisecond)
		if err := launchUpdater(newPath, target); err != nil {
			stateMu.Lock()
			updateStatus = "Güncelleme kurulamadı: " + err.Error()
			stateMu.Unlock()
			return
		}
		pPostMessageW.Call(hwndMain, WM_CLOSE, 0, 0)
	}()
}

func refreshMarket(userTriggered bool) {
	stateMu.Lock()
	if marketBusy {
		stateMu.Unlock()
		return
	}
	marketBusy = true
	marketStatus = "Lehrstellen güncelleniyor..."
	stateMu.Unlock()
	invalidate()
	go func() {
		defer func() {
			stateMu.Lock()
			marketBusy = false
			stateMu.Unlock()
			pPostMessageW.Call(hwndMain, WM_APP_MARKET_DONE, 0, 0)
		}()
		req, _ := http.NewRequest("GET", marketDataURL+"?t="+strconv.FormatInt(time.Now().Unix(), 10), nil)
		req.Header.Set("User-Agent", "LehrRadar/"+appVersion)
		resp, err := (&http.Client{Timeout: 7 * time.Second}).Do(req)
		if err != nil {
			stateMu.Lock()
			marketStatus = "Canlı Lehrstellen kaynağına ulaşılamadı"
			stateMu.Unlock()
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			stateMu.Lock()
			marketStatus = "Canlı Lehrstellen kanalı hazırlanıyor"
			stateMu.Unlock()
			return
		}
		if resp.StatusCode >= 300 {
			stateMu.Lock()
			marketStatus = fmt.Sprintf("Lehrstellen kontrolü: HTTP %d", resp.StatusCode)
			stateMu.Unlock()
			return
		}
		var mk []Market
		if err := json.NewDecoder(resp.Body).Decode(&mk); err != nil || len(mk) == 0 {
			stateMu.Lock()
			marketStatus = "Canlı Lehrstellen verisi geçersiz"
			stateMu.Unlock()
			return
		}
		dataMu.Lock()
		current.Market = mk
		current.Meta.MarketLastChecked = time.Now().Format("2006-01-02 15:04")
		dataMu.Unlock()
		_ = saveData()
		stateMu.Lock()
		marketStatus = fmt.Sprintf("Canlı · %d Lehrbetrieb", len(mk))
		statusLine = "Lehrstellen listesi güncellendi"
		stateMu.Unlock()
		if userTriggered {
			notifyUser()
		}
	}()
}

func notifyUser() {
	pMessageBeep.Call(MB_ICONINFORMATION)
	if hwndMain != 0 {
		f := FLASHWINFO{CbSize: uint32(unsafe.Sizeof(FLASHWINFO{})), Hwnd: hwndMain, DwFlags: FLASHW_ALL | FLASHW_TIMERNOFG, UCount: 5}
		pFlashWindowEx.Call(uintptr(unsafe.Pointer(&f)))
	}
}

func messageBox(title, text string, icon uintptr) {
	pMessageBoxW.Call(hwndMain, uintptr(unsafe.Pointer(wstr(text))), uintptr(unsafe.Pointer(wstr(title))), MB_OK|icon)
}
func invalidate() { pInvalidateRect.Call(hwndMain, 0, 1) }
func openExternal(target string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(wstr("open"))), uintptr(unsafe.Pointer(wstr(target))), 0, 0, 1)
}
func openFolder(path string) { openExternal(path) }

func createFont(size int32, weight int32) uintptr {
	r, _, _ := pCreateFontW.Call(uintptr(-size), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wstr("Segoe UI"))))
	return r
}
func initFonts() {
	fontTitle = createFont(28, FW_BOLD)
	fontSub = createFont(14, FW_NORMAL)
	fontTab = createFont(14, FW_SEMIBOLD)
	fontBody = createFont(14, FW_NORMAL)
	fontBold = createFont(14, FW_SEMIBOLD)
	fontSmall = createFont(12, FW_NORMAL)
	fontBig = createFont(26, FW_BOLD)
}
func brush(c uintptr) uintptr {
	if b := brushCache[c]; b != 0 {
		return b
	}
	r, _, _ := pCreateSolidBrush.Call(c)
	brushCache[c] = r
	return r
}
func fill(hdc uintptr, r RECT, c uintptr) {
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush(c))
}
func text(hdc uintptr, r RECT, s string, c uintptr, font uintptr, flags uintptr) {
	old, _, _ := pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, c)
	pSetBkMode.Call(hdc, TRANSPARENT)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(wstr(s))), uintptr(0xffffffff), uintptr(unsafe.Pointer(&r)), flags)
	pSelectObject.Call(hdc, old)
}
func line(hdc uintptr, x1, y1, x2, y2 int32, c uintptr) {
	pen := penCache[c]
	if pen == 0 {
		pen, _, _ = pCreatePen.Call(PS_SOLID, 1, c)
		penCache[c] = pen
	}
	old, _, _ := pSelectObject.Call(hdc, pen)
	pMoveToEx.Call(hdc, uintptr(x1), uintptr(y1), 0)
	pLineTo.Call(hdc, uintptr(x2), uintptr(y2))
	pSelectObject.Call(hdc, old)
}

func countStatus(st string) int {
	dataMu.RLock()
	defer dataMu.RUnlock()
	n := 0
	for _, a := range current.Applications {
		if a.Status == st {
			n++
		}
	}
	return n
}
func unappliedMarkets() []Market {
	dataMu.RLock()
	defer dataMu.RUnlock()
	var out []Market
	for _, m := range current.Market {
		if !isMarketAppliedLocked(m) {
			out = append(out, m)
		}
	}
	return out
}

var normalizeRe = regexp.MustCompile(`[^a-z0-9äöüß]+`)
var rejectRe = regexp.MustCompile(`absage|abgelehnt|leider.{0,80}nicht|anderen bewerber|nicht in die engere|nicht weiter`)
var positiveRe = regexp.MustCompile(`schnupper|einladung|vorstellungsgespräch|gespräch|termin|kennenlernen|probearbeit`)
var reviewingRe = regexp.MustCompile(`erhalten|eingangsbestätigung|prüfen|geprüft|geduld|bearbeit`)

func normalize(s string) string {
	s = strings.ToLower(s)
	s = normalizeRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
func isMarketAppliedLocked(m Market) bool {
	nm := normalize(m.Company)
	for _, a := range current.Applications {
		na := normalize(a.Company)
		if nm == na || strings.Contains(nm, na) || strings.Contains(na, nm) {
			return true
		}
	}
	return false
}

func drawPill(hdc uintptr, x, y, w int32, label string, bg, fg uintptr, f uintptr) {
	r := RECT{x, y, x + w, y + 26}
	fill(hdc, r, bg)
	text(hdc, r, label, fg, f, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	case WM_PAINT:
		paint(hwnd)
		return 0
	case WM_ERASEBKGND:
		// We draw the full client area into a backbuffer; skipping background erase
		// avoids redundant work and visible stalls/flicker on resize.
		return 1
	case WM_SIZE:
		invalidate()
		return 0
	case WM_LBUTTONDOWN:
		handleClick(loWord(lParam), hiWord(lParam))
		return 0
	case WM_MOUSEWHEEL:
		delta := hiWord(wParam)
		if delta > 0 {
			offsets[activeTab] -= 3
		} else {
			offsets[activeTab] += 3
		}
		if offsets[activeTab] < 0 {
			offsets[activeTab] = 0
		}
		invalidate()
		return 0
	case WM_KEYDOWN:
		if wParam == VK_UP {
			offsets[activeTab]--
			if offsets[activeTab] < 0 {
				offsets[activeTab] = 0
			}
			invalidate()
		}
		if wParam == VK_DOWN {
			offsets[activeTab]++
			invalidate()
		}
		if wParam == VK_PRIOR {
			offsets[activeTab] -= 8
			if offsets[activeTab] < 0 {
				offsets[activeTab] = 0
			}
			invalidate()
		}
		if wParam == VK_NEXT {
			offsets[activeTab] += 8
			invalidate()
		}
		return 0
	case WM_TIMER:
		if wParam == 1 {
			startSync(false)
		}
		if wParam == 2 {
			invalidate()
		}
		if wParam == 3 {
			checkForUpdate(false)
		}
		if wParam == 4 {
			refreshMarket(false)
		}
		return 0
	case WM_APP_CLEAR_PRESSED:
		stateMu.Lock()
		pressedName = ""
		stateMu.Unlock()
		invalidate()
		return 0
	case WM_APP_ROUTE_DONE:
		invalidate()
		return 0
	case WM_APP_TILE_DONE:
		invalidate()
		return 0
	case WM_APP_UPDATE_DONE:
		invalidate()
		return 0
	case WM_APP_MARKET_DONE:
		invalidate()
		return 0
	case WM_APP_SYNC_DONE:
		stateMu.Lock()
		syncBusy = false
		stateMu.Unlock()
		invalidate()
		return 0
	case WM_APP_OAUTH_DONE:
		stateMu.Lock()
		oauthBusy = false
		stateMu.Unlock()
		invalidate()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func paint(hwnd uintptr) {
	var ps PAINTSTRUCT
	screenHdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc RECT
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	W, H := rc.Right, rc.Bottom
	if W <= 0 || H <= 0 {
		return
	}
	memDC := ensureBackbuffer(screenHdc, W, H)
	paintScene(memDC, W, H)
	pBitBlt.Call(screenHdc, 0, 0, uintptr(W), uintptr(H), memDC, 0, 0, SRCCOPY)
}

func paintScene(hdc uintptr, W, H int32) {
	fill(hdc, RECT{0, 0, W, H}, rgb(8, 13, 25))
	hits = nil
	fill(hdc, RECT{0, 0, W, 82}, rgb(12, 20, 38))
	text(hdc, RECT{28, 12, 360, 48}, "LEHR RADAR", rgb(241, 245, 249), fontTitle, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	text(hdc, RECT{29, 46, 720, 70}, "Automobil-Fachmann EFZ · Kanton Aargau · Lehrbeginn 2027", rgb(148, 163, 184), fontSub, DT_LEFT|DT_VCENTER|DT_SINGLELINE)

	dataMu.RLock()
	marketChecked := current.Meta.MarketLastChecked
	homeAddr := current.Home.Address
	dataMu.RUnlock()
	if marketChecked == "" {
		marketChecked = "—"
	}
	fresh := fmt.Sprintf("v%s  ·  %s  ·  Lehrstellen: %s", appVersion, time.Now().Format("02.01.2006 15:04"), marketChecked)
	text(hdc, RECT{W - 610, 58, W - 28, 78}, fresh, rgb(100, 116, 139), fontSmall, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)

	stateMu.Lock()
	sb := syncBusy
	ob := oauthBusy
	ls := lastSyncText
	sl := statusLine
	alert := lastAlert
	alertActive := alert != "" && time.Now().Before(alertUntil)
	stateMu.Unlock()
	bx := W - 568
	stateMu.Lock()
	ub := updateBusy
	stateMu.Unlock()
	updateLabel := "Güncelle"
	if ub {
		updateLabel = "Kontrol..."
	}
	addButton(hdc, RECT{bx, 18, bx + 128, 56}, updateLabel, "update", rgb(49, 46, 129), rgb(255, 255, 255), fontTab)
	addButton(hdc, RECT{bx + 140, 18, bx + 292, 56}, func() string {
		if sb {
			return "Senkronize..."
		}
		return "↻ Gmail Yenile"
	}(), "sync", rgb(37, 99, 235), rgb(255, 255, 255), fontTab)
	gmailLabel := "Gmail Bağla"
	if tokenExists() {
		gmailLabel = "Gmail Bağlı ✓"
	}
	if ob {
		gmailLabel = "Bağlanıyor..."
	}
	addButton(hdc, RECT{bx + 304, 18, bx + 540, 56}, gmailLabel, "gmail", rgb(30, 41, 59), rgb(226, 232, 240), fontTab)

	tabs := []string{"Özet", "Başvurular", "Redler", "Başvurmadıklar", "Harita / Mesafe", "Ayarlar"}
	widths := []int32{92, 112, 86, 150, 146, 92}
	x := int32(28)
	y := int32(94)
	for i, t := range tabs {
		tw := widths[i]
		r := RECT{x, y, x + tw, y + 36}
		if i == activeTab {
			fill(hdc, r, rgb(30, 64, 175))
			text(hdc, r, t, rgb(255, 255, 255), fontTab, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		} else {
			text(hdc, r, t, rgb(148, 163, 184), fontTab, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		}
		hits = append(hits, RectHit{fmt.Sprintf("tab:%d", i), r, ""})
		x += tw + 8
	}
	line(hdc, 28, 136, W-28, 136, rgb(30, 41, 59))

	if alertActive {
		r := RECT{W - 440, 92, W - 28, 132}
		fill(hdc, r, rgb(22, 78, 63))
		text(hdc, RECT{r.Left + 12, r.Top, r.Right - 12, r.Bottom}, alert, rgb(220, 252, 231), fontSmall, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}

	switch activeTab {
	case 0:
		drawDashboard(hdc, W, H, fontBold, fontBody, fontSmall)
	case 1:
		drawApplications(hdc, W, H, fontBold, fontBody, fontSmall, false)
	case 2:
		drawApplications(hdc, W, H, fontBold, fontBody, fontSmall, true)
	case 3:
		drawMarket(hdc, W, H, fontBold, fontBody, fontSmall)
	case 4:
		drawDistanceTab(hdc, W, H, fontBold, fontBody, fontSmall)
	case 5:
		drawSettings(hdc, W, H, fontBold, fontBody, fontSmall)
	}

	if strings.TrimSpace(homeAddr) == "" {
		homeAddr = "ayarlanmamış"
	}
	status := fmt.Sprintf("Durum: %s  ·  %s  ·  Ev: %s", sl, ls, homeAddr)
	text(hdc, RECT{28, H - 34, W - 28, H - 10}, status, rgb(100, 116, 139), fontSmall, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func darken(c uintptr, amount byte) uintptr {
	r := byte(c & 0xff)
	g := byte((c >> 8) & 0xff)
	b := byte((c >> 16) & 0xff)
	if r > amount {
		r -= amount
	} else {
		r = 0
	}
	if g > amount {
		g -= amount
	} else {
		g = 0
	}
	if b > amount {
		b -= amount
	} else {
		b = 0
	}
	return rgb(r, g, b)
}

func addButton(hdc uintptr, r RECT, label, name string, bg, fg uintptr, font uintptr) {
	stateMu.Lock()
	pressed := pressedName == name
	stateMu.Unlock()
	original := r
	if pressed {
		bg = darken(bg, 38)
		r.Top += 2
		r.Bottom += 2
	}
	fill(hdc, r, bg)
	border := rgb(71, 85, 105)
	if pressed {
		border = rgb(147, 197, 253)
	}
	line(hdc, r.Left, r.Top, r.Right, r.Top, border)
	line(hdc, r.Left, r.Bottom-1, r.Right, r.Bottom-1, border)
	line(hdc, r.Left, r.Top, r.Left, r.Bottom, border)
	line(hdc, r.Right-1, r.Top, r.Right-1, r.Bottom, border)
	text(hdc, r, label, fg, font, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	hits = append(hits, RectHit{name, original, ""})
}

func showPress(name string) {
	stateMu.Lock()
	pressedName = name
	stateMu.Unlock()
	invalidate()
	time.AfterFunc(160*time.Millisecond, func() { pPostMessageW.Call(hwndMain, WM_APP_CLEAR_PRESSED, 0, 0) })
}

func drawDashboard(hdc uintptr, W, H int32, boldF, bodyF, smallF uintptr) {
	applied := 0
	rejected := 0
	reviewing := 0
	positive := 0
	waiting := 0
	dataMu.RLock()
	for _, a := range current.Applications {
		applied++
		switch a.Status {
		case "rejected":
			rejected++
		case "reviewing":
			reviewing++
		case "positive":
			positive++
		default:
			waiting++
		}
	}
	dataMu.RUnlock()
	market := len(unappliedMarkets())
	cards := []struct {
		label string
		n     int
		bg    uintptr
	}{{"Toplam başvuru", applied, rgb(30, 41, 59)}, {"Bekleyen", waiting, rgb(30, 58, 82)}, {"İnceleniyor", reviewing, rgb(80, 60, 20)}, {"Red", rejected, rgb(90, 28, 38)}, {"Olumlu", positive, rgb(22, 75, 55)}, {"Başvurmadığın", market, rgb(49, 46, 129)}}
	x := int32(28)
	y := int32(154)
	gap := int32(12)
	cw := (W - 56 - gap*5) / 6
	if cw < 145 {
		cw = 145
	}
	bigF := fontBig
	for _, c := range cards {
		r := RECT{x, y, x + cw, y + 92}
		fill(hdc, r, c.bg)
		text(hdc, RECT{x + 14, y + 12, x + cw - 10, y + 38}, c.label, rgb(203, 213, 225), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		text(hdc, RECT{x + 14, y + 39, x + cw - 10, y + 78}, strconv.Itoa(c.n), rgb(255, 255, 255), bigF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		x += cw + gap
	}
	text(hdc, RECT{28, 267, W - 28, 296}, "Son başvurular", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	dataMu.RLock()
	apps := append([]Application(nil), current.Applications...)
	dataMu.RUnlock()
	sort.Slice(apps, func(i, j int) bool { return apps[i].AppliedDate > apps[j].AppliedDate })
	if len(apps) > 8 {
		apps = apps[:8]
	}
	drawAppRows(hdc, apps, 306, W, H, boldF, bodyF, smallF)
}

func routeDestination(address, location string) string {
	if strings.TrimSpace(address) != "" {
		return address
	}
	if strings.TrimSpace(location) != "" {
		return location + ", Schweiz"
	}
	return "Kanton Aargau, Schweiz"
}

func googleRouteURL(dest string) string {
	dataMu.RLock()
	origin := current.Home.Address
	dataMu.RUnlock()
	q := url.Values{}
	q.Set("api", "1")
	q.Set("origin", origin)
	q.Set("destination", dest)
	q.Set("travelmode", "driving")
	return "https://www.google.com/maps/dir/?" + q.Encode()
}

func haversineKM(a, b Coord) float64 {
	const R = 6371.0
	lat1, lat2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dlat := (b.Lat - a.Lat) * math.Pi / 180
	dlon := (b.Lon - a.Lon) * math.Pi / 180
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}

func cachedHomeCoord() (Coord, bool) {
	homeCoordMu.RLock()
	defer homeCoordMu.RUnlock()
	return homeCoord, homeCoord.Lat != 0 || homeCoord.Lon != 0
}

func fallbackRoute(location string) (RouteInfo, bool) {
	target, ok := coordForLocation(location)
	if !ok {
		return RouteInfo{}, false
	}
	home, homeOK := cachedHomeCoord()
	if !homeOK {
		// Generic temporary center until the user's own home address is geocoded.
		home = Coord{47.390, 8.140}
	}
	km := haversineKM(home, target)
	// Conservative road factor + city/entry overhead. Exact OSRM value replaces this in background.
	roadKM := km * 1.28
	minutes := int(math.Round(roadKM/52.0*60.0 + 5.0))
	if minutes < 4 {
		minutes = 4
	}
	return RouteInfo{Minutes: minutes, KM: roadKM, Estimated: true}, true
}

func geocodeSwiss(q string) (Coord, error) {
	u := "https://api3.geo.admin.ch/rest/services/api/SearchServer?type=locations&limit=1&searchText=" + url.QueryEscape(q)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "LehrRadar/"+appVersion)
	resp, err := routeClient.Do(req)
	if err != nil {
		return Coord{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Coord{}, fmt.Errorf("geocoding HTTP %d", resp.StatusCode)
	}
	var r struct {
		Results []struct {
			Attrs struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"attrs"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Coord{}, err
	}
	if len(r.Results) == 0 || (r.Results[0].Attrs.Lat == 0 && r.Results[0].Attrs.Lon == 0) {
		return Coord{}, fmt.Errorf("konum bulunamadı")
	}
	return Coord{Lat: r.Results[0].Attrs.Lat, Lon: r.Results[0].Attrs.Lon}, nil
}

func getHomeCoord() (Coord, error) {
	homeOnce.Do(func() {
		dataMu.RLock()
		addr := strings.TrimSpace(current.Home.Address)
		dataMu.RUnlock()
		if addr == "" {
			homeCoordMu.Lock()
			homeCoordErr = fmt.Errorf("ev adresi ayarlanmamış")
			homeCoordMu.Unlock()
			return
		}
		c, err := geocodeSwiss(addr)
		homeCoordMu.Lock()
		homeCoord, homeCoordErr = c, err
		homeCoordMu.Unlock()
	})
	homeCoordMu.RLock()
	defer homeCoordMu.RUnlock()
	return homeCoord, homeCoordErr
}

func ensureHomeCoordAsync() {
	if _, ok := cachedHomeCoord(); ok {
		return
	}
	go func() {
		_, _ = getHomeCoord()
		pPostMessageW.Call(hwndMain, WM_APP_ROUTE_DONE, 0, 0)
	}()
}

func routeOSRM(a, b Coord) (RouteInfo, error) {
	u := fmt.Sprintf("https://router.project-osrm.org/route/v1/driving/%.6f,%.6f;%.6f,%.6f?overview=false", a.Lon, a.Lat, b.Lon, b.Lat)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "LehrRadar/"+appVersion)
	resp, err := routeClient.Do(req)
	if err != nil {
		return RouteInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return RouteInfo{}, fmt.Errorf("route HTTP %d", resp.StatusCode)
	}
	var r struct {
		Routes []struct {
			Duration float64 `json:"duration"`
			Distance float64 `json:"distance"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return RouteInfo{}, err
	}
	if len(r.Routes) == 0 {
		return RouteInfo{}, fmt.Errorf("rota bulunamadı")
	}
	return RouteInfo{Minutes: int(math.Round(r.Routes[0].Duration / 60)), KM: r.Routes[0].Distance / 1000}, nil
}

func routeFor(key, dest, location string) RouteInfo {
	routeMu.Lock()
	if r, ok := routeCache[key]; ok {
		routeMu.Unlock()
		return r
	}
	if !routePending[key] {
		routePending[key] = true
		go func() {
			routeSem <- struct{}{}
			defer func() { <-routeSem }()
			home, err := getHomeCoord()
			var out RouteInfo
			if err == nil {
				var target Coord
				target, err = geocodeSwiss(dest)
				if err == nil {
					out, err = routeOSRM(home, target)
				}
			}
			if err != nil {
				if f, ok := fallbackRoute(location); ok {
					out = f
					out.Err = err.Error()
				} else {
					out = RouteInfo{Err: err.Error(), Estimated: true}
				}
			}
			routeMu.Lock()
			routeCache[key] = out
			delete(routePending, key)
			routeMu.Unlock()
			go saveRouteCache()
			pPostMessageW.Call(hwndMain, WM_APP_ROUTE_DONE, 0, 0)
		}()
	}
	routeMu.Unlock()
	if f, ok := fallbackRoute(location); ok {
		return f
	}
	return RouteInfo{Estimated: true}
}

func routeLabel(r RouteInfo) string {
	if r.Minutes <= 0 {
		return "… dk"
	}
	if r.Estimated {
		return fmt.Sprintf("~%d dk", r.Minutes)
	}
	return fmt.Sprintf("%d dk", r.Minutes)
}

func statusLabel(st string) (string, uintptr, uintptr) {
	switch st {
	case "rejected":
		return "RED", rgb(127, 29, 29), rgb(254, 226, 226)
	case "reviewing":
		return "İNCELENİYOR", rgb(113, 63, 18), rgb(254, 243, 199)
	case "positive":
		return "OLUMLU", rgb(20, 83, 45), rgb(220, 252, 231)
	default:
		return "BEKLENİYOR", rgb(30, 64, 175), rgb(219, 234, 254)
	}
}

func drawApplications(hdc uintptr, W, H int32, boldF, bodyF, smallF uintptr, rejectedOnly bool) {
	dataMu.RLock()
	var apps []Application
	for _, a := range current.Applications {
		if !rejectedOnly || a.Status == "rejected" {
			apps = append(apps, a)
		}
	}
	dataMu.RUnlock()
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].AppliedDate > apps[j].AppliedDate })
	title := "Tüm Lehr başvuruları"
	if rejectedOnly {
		title = "Red gelen garajlar"
	}
	text(hdc, RECT{28, 150, W - 28, 182}, title, rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	off := offsets[activeTab]
	if off > len(apps)-1 && len(apps) > 0 {
		off = len(apps) - 1
		offsets[activeTab] = off
	}
	if off < 0 {
		off = 0
	}
	if off < len(apps) {
		apps = apps[off:]
	} else {
		apps = nil
	}
	drawAppRows(hdc, apps, 190, W, H, boldF, bodyF, smallF)
}

func drawAppRows(hdc uintptr, apps []Application, startY, W, H int32, boldF, bodyF, smallF uintptr) {
	y := startY
	rh := int32(62)
	for _, a := range apps {
		if y+rh > H-48 {
			break
		}
		r := RECT{28, y, W - 28, y + rh - 5}
		fill(hdc, r, rgb(15, 23, 42))
		text(hdc, RECT{42, y + 7, W - 460, y + 29}, a.Company, rgb(241, 245, 249), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		text(hdc, RECT{42, y + 30, W - 460, y + 52}, fmt.Sprintf("%s · %s · %s", a.Location, a.AppliedDate, a.Email), rgb(148, 163, 184), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		dest := routeDestination(a.Address, a.Location)
		ri := routeFor("app:"+a.ID, dest, a.Location)
		routeR := RECT{W - 302, y + 14, W - 214, y + 48}
		addButton(hdc, routeR, routeLabel(ri), "route:app:"+a.ID, rgb(22, 45, 66), rgb(186, 230, 253), smallF)
		hits[len(hits)-1].URL = googleRouteURL(dest)
		lab, bg, fg := statusLabel(a.Status)
		pw := int32(116)
		if lab == "İNCELENİYOR" {
			pw = 132
		}
		drawPill(hdc, W-190, y+18, pw, lab, bg, fg, smallF)
		y += rh
	}
}

func drawMarket(hdc uintptr, W, H int32, boldF, bodyF, smallF uintptr) {
	mk := unappliedMarkets()
	sort.SliceStable(mk, func(i, j int) bool {
		if mk[i].Availability != mk[j].Availability {
			return mk[i].Availability == "available"
		}
		return mk[i].Location < mk[j].Location
	})
	text(hdc, RECT{28, 150, W - 28, 182}, "Kanton AG · Henüz başvurmadığın Lehrbetriebe", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	dataMu.RLock()
	checked := current.Meta.MarketLastChecked
	dataMu.RUnlock()
	text(hdc, RECT{28, 180, W - 28, 204}, "Son piyasa kontrolü: "+checked+" · Mavi = 2027 açık · Gri = 2027 durumu net değil", rgb(100, 116, 139), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	off := offsets[activeTab]
	if off > len(mk)-1 && len(mk) > 0 {
		off = len(mk) - 1
	}
	if off < len(mk) {
		mk = mk[off:]
	} else {
		mk = nil
	}
	y := int32(212)
	rh := int32(72)
	for _, m := range mk {
		if y+rh > H-48 {
			break
		}
		r := RECT{28, y, W - 28, y + rh - 5}
		fill(hdc, r, rgb(15, 23, 42))
		text(hdc, RECT{42, y + 7, W - 450, y + 28}, m.Company, rgb(241, 245, 249), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		text(hdc, RECT{42, y + 29, W - 450, y + 50}, fmt.Sprintf("%s · %s · %d", m.Location, m.Direction, m.Year), rgb(148, 163, 184), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		av := "DURUM BELİRSİZ"
		bg := rgb(51, 65, 85)
		fg := rgb(226, 232, 240)
		if m.Availability == "available" {
			av = "2027 AÇIK"
			bg = rgb(30, 64, 175)
			fg = rgb(219, 234, 254)
		}
		drawPill(hdc, W-420, y+21, 120, av, bg, fg, smallF)
		dest := routeDestination(m.Address, m.Location)
		ri := routeFor("market:"+m.ID, dest, m.Location)
		rr := RECT{W - 288, y + 17, W - 202, y + 54}
		addButton(hdc, rr, routeLabel(ri), "route:market:"+m.ID, rgb(22, 45, 66), rgb(186, 230, 253), smallF)
		hits[len(hits)-1].URL = googleRouteURL(dest)
		br := RECT{W - 188, y + 17, W - 42, y + 54}
		addButton(hdc, br, "İlanı Aç", "url:"+m.ID, rgb(30, 41, 59), rgb(191, 219, 254), smallF)
		hits[len(hits)-1].URL = m.SourceURL
		y += rh
	}
}

var townCoords = map[string]Coord{
	"waltenschwil": {47.333, 8.298}, "wohlen": {47.350, 8.277}, "muri": {47.274, 8.338},
	"seon": {47.349, 8.160}, "lenzburg": {47.387, 8.176}, "seengen": {47.326, 8.204},
	"mellingen": {47.420, 8.273}, "gebenstorf": {47.481, 8.239}, "wettingen": {47.465, 8.327},
	"würenlingen": {47.532, 8.256}, "koblenz": {47.609, 8.238}, "frick": {47.508, 8.019},
	"zofingen": {47.288, 7.945}, "aarburg": {47.321, 7.900}, "safenwil": {47.321, 7.983},
	"hunzenschwil": {47.386, 8.123}, "unterentfelden": {47.367, 8.045}, "aarau": {47.391, 8.044},
	"mühlau": {47.230, 8.389}, "beinwil": {47.231, 8.342}, "hendschiken": {47.386, 8.216},
	"windisch": {47.478, 8.219}, "murgenthal": {47.272, 7.831}, "aarau rohr": {47.414, 8.076},
	"oberentfelden": {47.357, 8.044}, "kaiseraugst": {47.539, 7.728}, "spreitenbach": {47.423, 8.365},
	"meisterschwanden": {47.294, 8.228}, "arni": {47.318, 8.420}, "baden": {47.473, 8.308},
	"rothrist": {47.306, 7.890}, "untersiggenthal": {47.503, 8.254}, "schinznach-bad": {47.449, 8.165},
	"schinznach": {47.449, 8.165}, "kleindöttingen": {47.571, 8.248},
	"brugg": {47.481, 8.208}, "othmarsingen": {47.402, 8.214}, "rudolfstetten": {47.371, 8.381},
}

func coordForLocation(location string) (Coord, bool) {
	l := strings.ToLower(strings.TrimSpace(location))
	if l == "aargau" || l == "ag" {
		return Coord{47.390, 8.140}, true
	}
	keys := make([]string, 0, len(townCoords))
	for k := range townCoords {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.Contains(l, k) || strings.Contains(k, l) {
			return townCoords[k], true
		}
	}
	return Coord{}, false
}

func worldPixel(c Coord, zoom int) (float64, float64) {
	n := math.Exp2(float64(zoom))
	x := (c.Lon + 180.0) / 360.0 * n * 256.0
	lat := c.Lat * math.Pi / 180.0
	y := (1.0 - math.Log(math.Tan(lat)+1.0/math.Cos(lat))/math.Pi) / 2.0 * n * 256.0
	return x, y
}

func tileFilename(z, x, y int) string {
	return filepath.Join(tileCacheDir, fmt.Sprintf("%d-%d-%d.png", z, x, y))
}

func scheduleTile(z, x, y int, path string) {
	key := fmt.Sprintf("%d/%d/%d", z, x, y)
	tileMu.Lock()
	if tilePending[key] {
		tileMu.Unlock()
		return
	}
	tilePending[key] = true
	tileMu.Unlock()
	go func() {
		tileSem <- struct{}{}
		defer func() { <-tileSem }()
		u := fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", z, x, y)
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "LehrRadar/"+appVersion+" (personal desktop app)")
		resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
		if err == nil && resp.StatusCode < 300 {
			tmp := path + ".part"
			if f, e := os.Create(tmp); e == nil {
				_, e = io.Copy(f, resp.Body)
				_ = f.Close()
				if e == nil {
					_ = os.Rename(tmp, path)
				} else {
					_ = os.Remove(tmp)
				}
			}
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		tileMu.Lock()
		delete(tilePending, key)
		tileMu.Unlock()
		pPostMessageW.Call(hwndMain, WM_APP_TILE_DONE, 0, 0)
	}()
}

func tileImage(z, x, y int) uintptr {
	path := tileFilename(z, x, y)
	tileMu.Lock()
	if img := tileImages[path]; img != 0 {
		tileMu.Unlock()
		return img
	}
	tileMu.Unlock()
	if st, err := os.Stat(path); err == nil && st.Size() > 100 {
		var img uintptr
		status, _, _ := pGdipLoadImageFromFile.Call(uintptr(unsafe.Pointer(wstr(path))), uintptr(unsafe.Pointer(&img)))
		if status == 0 && img != 0 {
			tileMu.Lock()
			tileImages[path] = img
			tileMu.Unlock()
			return img
		}
		_ = os.Remove(path)
	}
	scheduleTile(z, x, y, path)
	return 0
}

func drawMarker(hdc uintptr, x, y, radius int32, bg, border uintptr) {
	b := brush(bg)
	p := penCache[border]
	if p == 0 {
		p, _, _ = pCreatePen.Call(PS_SOLID, 2, border)
		penCache[border] = p
	}
	oldB, _, _ := pSelectObject.Call(hdc, b)
	oldP, _, _ := pSelectObject.Call(hdc, p)
	pEllipse.Call(hdc, uintptr(x-radius), uintptr(y-radius), uintptr(x+radius), uintptr(y+radius))
	pSelectObject.Call(hdc, oldP)
	pSelectObject.Call(hdc, oldB)
}

func drawOSMMap(hdc uintptr, r RECT, items []MapItem, smallF, boldF uintptr) {
	// Stability-first native map. Earlier builds decoded OSM PNG tiles inside WM_PAINT;
	// on some Windows systems GDI+/disk/network contention could make the window appear
	// "Not responding". This version keeps all paint work local and deterministic while
	// still plotting real town coordinates and opening Google Maps for navigation.
	fill(hdc, r, rgb(17, 30, 48))
	// Aargau-ish grid/background for geographic orientation.
	for x := r.Left + 70; x < r.Right; x += 110 {
		line(hdc, x, r.Top, x, r.Bottom, rgb(35, 53, 73))
	}
	for y := r.Top + 52; y < r.Bottom; y += 78 {
		line(hdc, r.Left, y, r.Right, y, rgb(35, 53, 73))
	}
	line(hdc, r.Left, r.Top, r.Right, r.Top, rgb(71, 85, 105))
	line(hdc, r.Left, r.Bottom-1, r.Right, r.Bottom-1, rgb(71, 85, 105))
	line(hdc, r.Left, r.Top, r.Left, r.Bottom, rgb(71, 85, 105))
	line(hdc, r.Right-1, r.Top, r.Right-1, r.Bottom, rgb(71, 85, 105))

	// Bounds cover Aargau and nearby Zofingen/Murgenthal area.
	const minLon, maxLon = 7.70, 8.45
	const minLat, maxLat = 47.20, 47.63
	project := func(c Coord) (int32, int32) {
		x := float64(r.Left+24) + (c.Lon-minLon)/(maxLon-minLon)*float64((r.Right-r.Left)-48)
		y := float64(r.Bottom-24) - (c.Lat-minLat)/(maxLat-minLat)*float64((r.Bottom-r.Top)-48)
		return int32(x), int32(y)
	}

	// Home marker if we know the exact locally geocoded position, otherwise Waltenschwil.
	home, ok := cachedHomeCoord()
	if !ok {
		home = townCoords["waltenschwil"]
	}
	hx, hy := project(home)
	drawMarker(hdc, hx, hy, 8, rgb(22, 163, 74), rgb(220, 252, 231))
	text(hdc, RECT{hx + 10, hy - 12, hx + 90, hy + 12}, "EV", rgb(220, 252, 231), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)

	for _, it := range items {
		c, ok := coordForLocation(it.Location)
		if !ok {
			continue
		}
		x, y := project(c)
		if x < r.Left+8 || x > r.Right-8 || y < r.Top+8 || y > r.Bottom-8 {
			continue
		}
		bg := rgb(37, 99, 235)
		if !it.Applied {
			bg = rgb(124, 58, 237)
		}
		if it.Status == "rejected" {
			bg = rgb(185, 28, 28)
		}
		if it.Status == "positive" {
			bg = rgb(5, 150, 105)
		}
		drawMarker(hdc, x, y, 5, bg, rgb(226, 232, 240))
	}

	text(hdc, RECT{r.Left + 14, r.Top + 8, r.Right - 14, r.Top + 30}, "Aargau haritası · hafif mod", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	text(hdc, RECT{r.Left + 14, r.Bottom - 32, r.Right - 14, r.Bottom - 8}, "Yeşil: ev/olumlu · Mavi: başvuru · Mor: başvurulmadı · Kırmızı: red", rgb(148, 163, 184), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

func buildMapItems() []MapItem {
	dataMu.RLock()
	apps := append([]Application(nil), current.Applications...)
	markets := append([]Market(nil), current.Market...)
	dataMu.RUnlock()
	items := make([]MapItem, 0, len(apps)+len(markets))
	for _, a := range apps {
		items = append(items, MapItem{Key: "app:" + a.ID, Company: a.Company, Location: a.Location, Address: a.Address, Status: a.Status, Applied: true})
	}
	for _, m := range markets {
		applied := false
		for _, a := range apps {
			na, nm := normalize(a.Company), normalize(m.Company)
			if na == nm || strings.Contains(na, nm) || strings.Contains(nm, na) {
				applied = true
				break
			}
		}
		if !applied {
			items = append(items, MapItem{Key: "market:" + m.ID, Company: m.Company, Location: m.Location, Address: m.Address, Status: "not_applied", Applied: false})
		}
	}
	return items
}

func drawDistanceTab(hdc uintptr, W, H int32, boldF, bodyF, smallF uintptr) {
	dataMu.RLock()
	home := current.Home
	dataMu.RUnlock()
	text(hdc, RECT{28, 150, W - 28, 182}, "Harita / Mesafe", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	homeR := RECT{28, 188, W - 28, 240}
	fill(hdc, homeR, rgb(17, 31, 50))
	text(hdc, RECT{44, 194, 88, 218}, "EV", rgb(125, 211, 252), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	text(hdc, RECT{88, 194, W - 420, 218}, home.Address, rgb(226, 232, 240), bodyF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	text(hdc, RECT{88, 216, W - 420, 237}, "Dakikalar ev adresinden tahmini araba süresi. Harita canlı OpenStreetMap katmanı kullanır.", rgb(100, 116, 139), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	addButton(hdc, RECT{W - 392, 196, W - 236, 232}, "Evi haritada aç", "home-route", rgb(22, 78, 63), rgb(255, 255, 255), smallF)
	addButton(hdc, RECT{W - 224, 196, W - 42, 232}, "Mesafeleri yenile", "route-refresh", rgb(30, 64, 175), rgb(255, 255, 255), smallF)

	items := buildMapItems()
	// Sort with instant local estimates; exact routes are only requested for visible rows.
	sort.SliceStable(items, func(i, j int) bool {
		fi, _ := fallbackRoute(items[i].Location)
		fj, _ := fallbackRoute(items[j].Location)
		mi, mj := fi.Minutes, fj.Minutes
		if mi == 0 {
			mi = 9999
		}
		if mj == 0 {
			mj = 9999
		}
		if mi != mj {
			return mi < mj
		}
		return items[i].Company < items[j].Company
	})

	listW := int32(390)
	mapR := RECT{28, 252, W - listW - 46, H - 48}
	if mapR.Right-mapR.Left < 420 {
		mapR.Right = mapR.Left + 420
	}
	drawOSMMap(hdc, mapR, items, smallF, boldF)

	listX := mapR.Right + 16
	text(hdc, RECT{listX, 252, W - 28, 280}, "En yakın yerler", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	off := offsets[activeTab]
	if off < 0 {
		off = 0
	}
	if off > len(items)-1 && len(items) > 0 {
		off = len(items) - 1
		offsets[activeTab] = off
	}
	if off < len(items) {
		items = items[off:]
	} else {
		items = nil
	}
	y := int32(286)
	rh := int32(66)
	for _, it := range items {
		if y+rh > H-48 {
			break
		}
		r := RECT{listX, y, W - 28, y + rh - 6}
		fill(hdc, r, rgb(15, 23, 42))
		text(hdc, RECT{listX + 12, y + 5, W - 165, y + 27}, it.Company, rgb(241, 245, 249), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		label := "Başvuruldu"
		if !it.Applied {
			label = "Başvurulmadı"
		}
		text(hdc, RECT{listX + 12, y + 28, W - 165, y + 49}, it.Location+" · "+label, rgb(148, 163, 184), smallF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		dest := routeDestination(it.Address, it.Location)
		ri := routeFor(it.Key, dest, it.Location)
		detail := routeLabel(ri)
		if ri.KM > 0 {
			detail = fmt.Sprintf("%s · %.1f km", detail, ri.KM)
		}
		addButton(hdc, RECT{W - 154, y + 12, W - 42, y + 48}, detail, "route:list:"+it.Key, rgb(22, 45, 66), rgb(186, 230, 253), smallF)
		hits[len(hits)-1].URL = googleRouteURL(dest)
		y += rh
	}
}

func drawSettings(hdc uintptr, W, H int32, boldF, bodyF, smallF uintptr) {
	text(hdc, RECT{28, 150, W - 28, 182}, "Ayarlar & Gmail bağlantısı", rgb(226, 232, 240), boldF, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	connected := "Bağlı değil"
	if tokenExists() {
		connected = "Bağlı ✓"
	}
	configured := "Yok"
	if _, err := os.Stat(oauthPath); err == nil {
		configured = "Hazır ✓"
	}
	dataMu.RLock()
	home := current.Home.Address
	checked := current.Meta.MarketLastChecked
	dataMu.RUnlock()
	stateMu.Lock()
	us := updateStatus
	ms := marketStatus
	stateMu.Unlock()
	labels := []string{
		fmt.Sprintf("Sürüm: %s", appVersion),
		fmt.Sprintf("Otomatik güncelleme: %s", us),
		fmt.Sprintf("Canlı Lehrstellen: %s", ms),
		fmt.Sprintf("Gmail durumu: %s", connected),
		fmt.Sprintf("OAuth dosyası: %s", configured),
		fmt.Sprintf("Ev: %s", home),
		fmt.Sprintf("Lehrstellen son kontrol: %s", checked),
		"Otomatik Gmail kontrolü: uygulama açıkken her 10 dakikada bir",
		"Performans modu: native Win32 + çift tamponlu çizim + arka plan ağ işlemleri",
	}
	y := int32(200)
	for _, s := range labels {
		text(hdc, RECT{42, y, W - 42, y + 28}, s, rgb(203, 213, 225), bodyF, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		y += 31
	}
	addButton(hdc, RECT{42, y + 12, 230, y + 50}, "Veri klasörünü aç", "folder", rgb(30, 41, 59), rgb(226, 232, 240), smallF)
	addButton(hdc, RECT{244, y + 12, 468, y + 50}, "Gmail kurulum dosyasını aç", "setup", rgb(30, 41, 59), rgb(226, 232, 240), smallF)
	addButton(hdc, RECT{482, y + 12, 670, y + 50}, "Gmail Bağla", "gmail", rgb(37, 99, 235), rgb(255, 255, 255), smallF)
	addButton(hdc, RECT{684, y + 12, 884, y + 50}, "Ev rotasını aç", "home-route", rgb(22, 78, 63), rgb(255, 255, 255), smallF)
	addButton(hdc, RECT{898, y + 12, 1098, y + 50}, "Lehrstellen yenile", "market-refresh", rgb(49, 46, 129), rgb(255, 255, 255), smallF)
	text(hdc, RECT{42, y + 76, W - 42, y + 132}, "Mesafeler internet varsa arka planda İsviçre adres araması + yol rotası ile hesaplanır; servis erişilemezse yaklaşık süre gösterilir. Gmail işlemleri de arka planda çalışır, arayüzü kilitlemez.", rgb(148, 163, 184), smallF, DT_LEFT|DT_END_ELLIPSIS)
}

func pointIn(r RECT, x, y int32) bool {
	return x >= r.Left && x <= r.Right && y >= r.Top && y <= r.Bottom
}
func handleClick(x, y int32) {
	for _, h := range hits {
		if !pointIn(h.R, x, y) {
			continue
		}
		showPress(h.Name)
		switch {
		case strings.HasPrefix(h.Name, "tab:"):
			i, _ := strconv.Atoi(strings.TrimPrefix(h.Name, "tab:"))
			activeTab = i
			invalidate()
			return
		case h.Name == "update":
			stateMu.Lock()
			statusLine = "Güncelleme kontrol ediliyor..."
			stateMu.Unlock()
			checkForUpdate(true)
			return
		case h.Name == "market-refresh":
			stateMu.Lock()
			statusLine = "Lehrstellen canlı kontrol ediliyor..."
			stateMu.Unlock()
			refreshMarket(true)
			return
		case h.Name == "sync":
			stateMu.Lock()
			statusLine = "Gmail Yenile tıklandı"
			stateMu.Unlock()
			startSync(true)
			return
		case h.Name == "gmail":
			stateMu.Lock()
			statusLine = "Gmail bağlantısı açılıyor..."
			stateMu.Unlock()
			startOAuth()
			return
		case h.Name == "folder":
			openFolder(appDataDir)
			return
		case h.Name == "setup":
			openExternal(setupPath)
			return
		case h.Name == "home-route":
			dataMu.RLock()
			home := strings.TrimSpace(current.Home.Address)
			dataMu.RUnlock()
			if home != "" {
				openExternal("https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(home))
			}
			return
		case h.Name == "route-refresh":
			routeMu.Lock()
			routeCache = map[string]RouteInfo{}
			routePending = map[string]bool{}
			routeMu.Unlock()
			_ = os.Remove(routeCachePath)
			homeOnce = sync.Once{}
			homeCoordMu.Lock()
			homeCoord, homeCoordErr = Coord{}, nil
			homeCoordMu.Unlock()
			ensureHomeCoordAsync()
			stateMu.Lock()
			statusLine = "Mesafe önbelleği temizlendi · yeniden hesaplanıyor"
			stateMu.Unlock()
			invalidate()
			return
		case strings.HasPrefix(h.Name, "url:") || strings.HasPrefix(h.Name, "route:") || strings.HasPrefix(h.Name, "maproute:"):
			if h.URL != "" {
				openExternal(h.URL)
			}
			return
		}
	}
}

func tokenExists() bool { _, err := os.Stat(tokenPath); return err == nil }
func oauthConfig() (OAuthConfig, error) {
	b, err := os.ReadFile(oauthPath)
	if err != nil {
		return OAuthConfig{}, err
	}
	var r oauthRoot
	if err = json.Unmarshal(b, &r); err != nil {
		return OAuthConfig{}, err
	}
	if r.Installed.ClientID != "" {
		return r.Installed, nil
	}
	if r.Web.ClientID != "" {
		return r.Web, nil
	}
	if r.ClientID != "" {
		return OAuthConfig{r.ClientID, r.ClientSecret}, nil
	}
	return OAuthConfig{}, fmt.Errorf("client_id bulunamadı")
}

func startOAuth() {
	stateMu.Lock()
	if oauthBusy {
		stateMu.Unlock()
		return
	}
	oauthBusy = true
	stateMu.Unlock()
	invalidate()
	if _, err := os.Stat(oauthPath); err != nil {
		stateMu.Lock()
		oauthBusy = false
		statusLine = "OAuth dosyası eksik"
		stateMu.Unlock()
		messageBox("Gmail kurulumu", "Gmail'i bağlamak için önce Google Desktop OAuth JSON dosyanı şu klasöre gmail-oauth.json adıyla koy:\n\n"+appDataDir+"\n\n'Gmail kurulum dosyasını aç' butonunda adımlar var.", MB_ICONWARNING)
		openFolder(appDataDir)
		invalidate()
		return
	}
	go func() {
		err := oauthFlow()
		if err != nil {
			stateMu.Lock()
			statusLine = "Gmail bağlantı hatası: " + err.Error()
			stateMu.Unlock()
			messageBox("Gmail bağlanamadı", err.Error(), MB_ICONWARNING)
		} else {
			stateMu.Lock()
			statusLine = "Gmail başarıyla bağlandı"
			stateMu.Unlock()
			messageBox("Gmail bağlandı", "Gmail bağlantısı tamamlandı. Bundan sonra uygulama açıkken her 10 dakikada bir Lehr başvurularını kontrol edecek.", MB_ICONINFORMATION)
			startSync(false)
		}
		pPostMessageW.Call(hwndMain, WM_APP_OAUTH_DONE, 0, 0)
	}()
}

func oauthFlow() error {
	cfg, err := oauthConfig()
	if err != nil {
		return err
	}
	verifierBytes := make([]byte, 48)
	rand.Read(verifierBytes)
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)
	auth := url.URL{Scheme: "https", Host: "accounts.google.com", Path: "/o/oauth2/v2/auth"}
	q := auth.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", "https://www.googleapis.com/auth/gmail.readonly")
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	auth.RawQuery = q.Encode()
	openExternal(auth.String())
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if e := r.URL.Query().Get("error"); e != "" {
			fmt.Fprintln(w, "Bağlantı iptal edildi. Bu pencereyi kapatabilirsiniz.")
			errCh <- fmt.Errorf(e)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "<h2>Lehr Radar</h2><p>Gmail bağlantısı tamamlandı. Bu pencereyi kapatabilirsiniz.</p>")
		codeCh <- code
	})
	go srv.Serve(ln)
	var code string
	select {
	case code = <-codeCh:
	case e := <-errCh:
		return e
	case <-time.After(3 * time.Minute):
		return fmt.Errorf("Google giriş süresi doldu")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	form := url.Values{"code": {code}, "client_id": {cfg.ClientID}, "redirect_uri": {redirect}, "grant_type": {"authorization_code"}, "code_verifier": {verifier}}
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	resp, err := client.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token alınamadı: %s", string(b))
	}
	var t Token
	if err = json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return err
	}
	t.ObtainedAt = time.Now().UnixMilli()
	b, _ := json.MarshalIndent(t, "", "  ")
	return os.WriteFile(tokenPath, b, 0600)
}

func getAccessToken() (string, error) {
	b, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("Gmail bağlı değil")
	}
	var t Token
	if err = json.Unmarshal(b, &t); err != nil {
		return "", err
	}
	if t.AccessToken != "" && time.Now().UnixMilli() < t.ObtainedAt+(t.ExpiresIn*1000)-60000 {
		return t.AccessToken, nil
	}
	if t.RefreshToken == "" {
		return "", fmt.Errorf("refresh token yok")
	}
	cfg, err := oauthConfig()
	if err != nil {
		return "", err
	}
	form := url.Values{"client_id": {cfg.ClientID}, "refresh_token": {t.RefreshToken}, "grant_type": {"refresh_token"}}
	if cfg.ClientSecret != "" {
		form.Set("client_secret", cfg.ClientSecret)
	}
	resp, err := client.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token yenilenemedi")
	}
	var f Token
	if err = json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return "", err
	}
	if f.RefreshToken == "" {
		f.RefreshToken = t.RefreshToken
	}
	f.ObtainedAt = time.Now().UnixMilli()
	bb, _ := json.MarshalIndent(f, "", "  ")
	os.WriteFile(tokenPath, bb, 0600)
	return f.AccessToken, nil
}

func gmailGET(token, endpoint string, out any) error {
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gmail HTTP %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
func gmailList(token, q string) ([]string, error) {
	u := "https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=150&q=" + url.QueryEscape(q)
	var r struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := gmailGET(token, u, &r); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.Messages))
	for _, m := range r.Messages {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
func gmailGet(token, id string) (GMessage, error) {
	u := "https://gmail.googleapis.com/gmail/v1/users/me/messages/" + id + "?format=metadata&metadataHeaders=Subject&metadataHeaders=From&metadataHeaders=To&metadataHeaders=Date"
	var m GMessage
	err := gmailGET(token, u, &m)
	return m, err
}
func h(m GMessage, name string) string {
	for _, x := range m.Payload.Headers {
		if strings.EqualFold(x.Name, name) {
			return x.Value
		}
	}
	return ""
}

var emailRe = regexp.MustCompile(`[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)

func emailAddr(s string) string {
	v := emailRe.FindString(strings.ToUpper(s))
	return strings.ToLower(v)
}
func statusFromText(s string) string {
	t := strings.ToLower(s)
	if rejectRe.MatchString(t) {
		return "rejected"
	}
	if positiveRe.MatchString(t) {
		return "positive"
	}
	if reviewingRe.MatchString(t) {
		return "reviewing"
	}
	return "waiting"
}

var known = map[string][2]string{
	"info.retail-schinznach@amag.ch": {"AMAG Schinznach", "Schinznach"}, "autocenter@emilfrey.ch": {"Emil Frey Safenwil", "Safenwil"}, "lernende@kennys.ch": {"Kenny's Auto-Center AG", "Wettingen"}, "hr@kueng-automobile.ch": {"Küng Automobile", "Aargau"}, "martina.bruder@feldgarage-seengen.ch": {"Feldgarage Seengen AG", "Seengen"}, "f.erne@automeierag.ch": {"Auto Meier AG", "Kleindöttingen"}, "info@garage-ruetter.ch": {"Garage Rütter AG", "Mühlau"}, "s.kloetzli@rhauto.ch": {"RH Auto-Service Lenzburg AG", "Lenzburg"}, "garage.haller@bluewin.ch": {"Haller Automobile AG", "Zofingen"}, "info@hasler-garage.ch": {"Walter Hasler AG", "Frick"}, "pinar@gmx.ch": {"Garage Pinar", "Aarburg"}, "ausbildung@baechli-auto.ch": {"Bächli Automobile AG", "Würenlingen"}, "garage@nusser.ch": {"K. Nusser AG", "Koblenz"}, "info@autoschlatterag.ch": {"Auto Schlatter AG", "Aargau"}, "hunzenschwil@bestdrive.ch": {"BestDrive Switzerland AG", "Hunzenschwil"}, "info@garage-frey.ch": {"Garage Frey Unterentfelden AG", "Unterentfelden"}, "info@rigacker.ch": {"Garage / Auto Rigacker", "Aargau"}, "info@garage-weibel.ch": {"Garage Weibel", "Untersiggenthal"}, "info@tinnerag.ch": {"Ruedi Tinner AG", "Aargau"}, "m.hosang@autoschneider.ch": {"Auto Schneider AG", "Würenlingen"}, "info@huber-automobile.ch": {"Huber Automobile AG", "Mellingen"}, "info@citygaragegmbh.ch": {"City-Garage GmbH", "Aarau"}, "garage@sutersepp.ch": {"Garage Sepp Suter", "Beinwil (Freiamt)"}, "samoa.huegli@huegli.swiss": {"Hügli", "Aargau"}, "info-wohlen@hedinautomotive.ch": {"Hedin Automotive Schweiz AG", "Wohlen"}}

func startSync(userTriggered bool) {
	if !tokenExists() {
		if userTriggered {
			messageBox("Gmail bağlı değil", "Önce 'Gmail Bağla' butonunu kullan. İlk kurulum için Google Desktop OAuth JSON gerekir.", MB_ICONWARNING)
		}
		return
	}
	stateMu.Lock()
	if syncBusy {
		stateMu.Unlock()
		return
	}
	syncBusy = true
	statusLine = "Gmail kontrol ediliyor..."
	stateMu.Unlock()
	invalidate()
	go func() {
		changes, err := syncGmail()
		if err != nil {
			stateMu.Lock()
			statusLine = "Senkron hatası: " + err.Error()
			lastSyncText = "Son deneme: " + time.Now().Format("15:04")
			stateMu.Unlock()
			if userTriggered {
				messageBox("Senkron hatası", err.Error(), MB_ICONWARNING)
			}
		} else {
			stateMu.Lock()
			statusLine = fmt.Sprintf("Gmail güncel · %d değişiklik", len(changes))
			lastSyncText = "Son Gmail kontrolü: " + time.Now().Format("02.01.2006 15:04")
			stateMu.Unlock()
			if len(changes) > 0 {
				stateMu.Lock()
				lastAlert = strings.Join(changes, " · ")
				alertUntil = time.Now().Add(12 * time.Second)
				stateMu.Unlock()
				notifyUser()
			}
		}
		pPostMessageW.Call(hwndMain, WM_APP_SYNC_DONE, 0, 0)
	}()
}

func syncGmail() ([]string, error) {
	token, err := getAccessToken()
	if err != nil {
		return nil, err
	}
	dataMu.RLock()
	cursor := current.Meta.GmailLastSyncUnix
	dataMu.RUnlock()
	if cursor == 0 {
		cursor = time.Now().Add(-7 * 24 * time.Hour).Unix()
	}
	// One-day overlap avoids missing messages around midnight/time-zone boundaries.
	since := time.Unix(cursor, 0).Add(-24 * time.Hour).Format("2006/01/02")
	sentIDs, err := gmailList(token, fmt.Sprintf(`in:sent after:%s ("Automobil-Fachmann" OR "Automobilfachmann" OR "Lehrstelle")`, since))
	if err != nil {
		return nil, err
	}
	incIDs, err := gmailList(token, fmt.Sprintf(`after:%s ("Automobil-Fachmann" OR "Automobilfachmann" OR "Lehrstelle" OR "Absage" OR "abgelehnt" OR "Schnupper") -from:me`, since))
	if err != nil {
		return nil, err
	}
	fetchAll := func(ids []string) []GMessage {
		out := make([]GMessage, 0, len(ids))
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, id := range ids {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				m, e := gmailGet(token, id)
				<-sem
				if e == nil {
					mu.Lock()
					out = append(out, m)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		return out
	}
	sent := fetchAll(sentIDs)
	incoming := fetchAll(incIDs)
	dataMu.Lock()
	defer dataMu.Unlock()
	byThread := map[string]int{}
	for i, a := range current.Applications {
		if a.ThreadID != "" {
			byThread[a.ThreadID] = i
		}
	}
	for _, m := range sent {
		if _, ok := byThread[m.ThreadID]; ok {
			continue
		}
		to := emailAddr(h(m, "To"))
		k, ok := known[to]
		company := to
		loc := "Aargau"
		if ok {
			company = k[0]
			loc = k[1]
		}
		sub := h(m, "Subject")
		if strings.Contains(strings.ToLower(sub), "agvs-eignungstest") {
			continue
		}
		date := time.Now().Format("2006-01-02")
		if ms, er := strconv.ParseInt(m.InternalDate, 10, 64); er == nil {
			date = time.UnixMilli(ms).Format("2006-01-02")
		}
		id := "gmail-" + m.ID
		current.Applications = append(current.Applications, Application{ID: id, Company: company, Location: loc, Email: to, AppliedDate: date, Status: "waiting", Source: "gmail", ThreadID: m.ThreadID, Note: "Gmail üzerinden otomatik eklendi."})
		byThread[m.ThreadID] = len(current.Applications) - 1
	}
	var changes []string
	for _, m := range incoming {
		idx, ok := byThread[m.ThreadID]
		if !ok {
			continue
		}
		txt := h(m, "Subject") + " " + m.Snippet
		ns := statusFromText(txt)
		old := current.Applications[idx].Status
		if ns != "waiting" && ns != old {
			current.Applications[idx].Status = ns
			current.Applications[idx].Note = "Gmail cevabı: " + m.Snippet
			if ns == "rejected" || ns == "positive" {
				lab := "Olumlu"
				if ns == "rejected" {
					lab = "Red"
				}
				changes = append(changes, current.Applications[idx].Company+": "+lab)
			}
		}
	}
	current.Meta.GmailLastSyncUnix = time.Now().Unix()
	b, _ := json.MarshalIndent(current, "", "  ")
	if err = os.WriteFile(dataPath, b, 0644); err != nil {
		return changes, err
	}
	return changes, nil
}

const gmailSetupText = `LEHR RADAR - GMAIL CANLI SENKRON KURULUMU

Bu EXE Node.js istemez ve direkt çalışır.
Gmail'i otomatik okuyabilmesi için Google'ın güvenlik kuralı nedeniyle bir kere kendi OAuth Desktop Client dosyanı vermen gerekir.

1) Google Cloud Console'a gir: https://console.cloud.google.com/
2) Bir proje oluştur (ör. Lehr Radar).
3) APIs & Services > Library > Gmail API > Enable.
4) OAuth consent screen bölümünde uygulamayı External olarak ayarla ve kendi Gmail adresini Test User olarak ekle.
5) Credentials > Create Credentials > OAuth client ID.
6) Application type: Desktop app.
7) JSON'u indir.
8) İndirdiğin JSON'u bu klasöre kopyala ve adını TAM olarak gmail-oauth.json yap:

%APPDATA%\LehrRadar\gmail-oauth.json

9) Lehr Radar'ı açıp Gmail Bağla butonuna bas.
10) Tarayıcıda kendi Google hesabınla izin ver.

Uygulama sadece Gmail read-only izni ister; mail gönderemez/silemez.
`

func main() {
	if !ensureInstalled() {
		return
	}
	if err := initFiles(); err != nil {
		fmt.Println(err)
		return
	}
	initGDIPlus()
	defer func() {
		cleanupBackbuffer()
		shutdownGDIPlus()
	}()
	initFonts()
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cls := wstr("LehrRadarNativeClass")
	cur, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), LpfnWndProc: syscall.NewCallback(wndProc), HInstance: hInst, HCursor: cur, LpszClassName: uintptr(unsafe.Pointer(cls))}
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		messageBox("Hata", "Pencere sınıfı oluşturulamadı.", MB_ICONWARNING)
		return
	}
	hwnd, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(wstr("Lehr Radar - Automobil-Fachmann EFZ"))), WS_OVERLAPPEDWINDOW|WS_VISIBLE, 120, 80, 1380, 860, 0, 0, hInst, 0)
	hwndMain = hwnd
	pShowWindow.Call(hwnd, SW_SHOW)
	pUpdateWindow.Call(hwnd)
	pSetTimer.Call(hwnd, 1, 10*60*1000, 0)
	pSetTimer.Call(hwnd, 2, 5*60*1000, 0)
	pSetTimer.Call(hwnd, 3, 30*60*1000, 0)
	pSetTimer.Call(hwnd, 4, 60*60*1000, 0)
	go func() { time.Sleep(500 * time.Millisecond); ensureHomeCoordAsync() }()
	go func() { time.Sleep(1200 * time.Millisecond); checkForUpdate(false) }()
	go func() { time.Sleep(1800 * time.Millisecond); refreshMarket(false) }()
	if tokenExists() {
		go func() { time.Sleep(2400 * time.Millisecond); startSync(false) }()
	}
	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
